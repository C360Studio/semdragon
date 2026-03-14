// Package questdagexec implements reactive DAG execution for party quests.
// When a lead agent decomposes a party quest, questbridge writes DAG state as
// quest.dag.* predicates on the parent quest entity. This component watches
// quest entity KV transitions to detect new DAGs and drive the state machine:
// node assignment, lead review dispatch, rollup, and failure escalation.
//
// Architecture: two producer goroutines (quest KV watcher, AGENT stream review
// consumer) feed a unified chan dagEvent. A single event loop goroutine
// processes events sequentially, eliminating data races by construction.
// dagCache, dagBySubQuest, and questCache are plain maps — no mutexes needed
// because only the event loop accesses them.
package questdagexec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semstreams/component"
)

// =============================================================================
// NARROW INTERFACES — sibling component dependencies
// =============================================================================
// Using interfaces instead of direct component references avoids import cycles
// and allows test mocking. Methods mirror the concrete implementations exactly.
// =============================================================================

// QuestBoardRef is the narrow interface questdagexec needs from questboard.
type QuestBoardRef interface {
	// SubmitResult transitions a quest to in_review with the given output.
	SubmitResult(ctx context.Context, questID domain.QuestID, result any) error
	// FailQuest transitions a quest to failed with a reason string.
	FailQuest(ctx context.Context, questID domain.QuestID, reason string) error
	// EscalateQuest transitions a quest to escalated.
	EscalateQuest(ctx context.Context, questID domain.QuestID, reason string) error
	// ClaimAndStartForParty claims a sub-quest for a specific agent within a
	// party and transitions it directly to in_progress in a single KV write,
	// eliminating the window where a crash could leave a sub-quest stuck in
	// claimed state. The assignedTo agent is recorded as ClaimedBy.
	ClaimAndStartForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID, assignedTo domain.AgentID) error
	// RepostForRetry resets a sub-quest back to posted status for DAG retry,
	// preserving the PartyID so it stays within the party's closed system.
	RepostForRetry(ctx context.Context, questID domain.QuestID) error
}

// PartyCoordRef is the narrow interface questdagexec needs from partycoord.
type PartyCoordRef interface {
	// JoinParty adds an agent to a party with the given role.
	JoinParty(ctx context.Context, partyID domain.PartyID, agentID domain.AgentID, role domain.PartyRole) error
	// AssignTask records a sub-quest assignment within the party.
	AssignTask(ctx context.Context, partyID domain.PartyID, subQuestID domain.QuestID, assignedTo domain.AgentID, rationale string) error
	// GetParty returns the current party state.
	GetParty(partyID domain.PartyID) (*partycoord.Party, bool)
	// DisbandParty dissolves the party.
	DisbandParty(ctx context.Context, partyID domain.PartyID, reason string) error
}

// =============================================================================
// COMPONENT
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
//
// Concurrency model:
//   - dagCache, dagBySubQuest, questCache: plain maps; ONLY accessed from the
//     event loop goroutine (runEventLoop). No mutexes required.
//   - questBoardRef, partyCoord: resolved lazily at call time via
//     resolveQuestBoard / resolvePartyCoord using deps.ComponentRegistry.
//   - Metric atomics (nodesCompleted etc.): safe from any goroutine.
// =============================================================================

// Component implements the questdagexec processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// events is the unified channel all producers write to. The event loop
	// is the sole reader. Buffered to absorb bursts without blocking producers.
	events chan dagEvent

	// In-memory DAG state — ONLY accessed by the event loop goroutine.
	// Plain maps; no mutexes needed.
	dagCache      map[string]*DAGExecutionState // executionID → state
	dagBySubQuest map[string]*DAGExecutionState // sub-quest entity key → state
	questCache    map[string]string             // quest entity key → last known status
	reviewRetries map[string]bool               // tracks one-time review retry per node

	// completedDAGKeys tracks entity keys of parent quests whose DAG has completed
	// (rollup triggered). Written by the event loop, read by produceQuestEvents
	// to prune seenDAGParents and allow future DAGs on the same entity key.
	// sync.Map because event loop writes and producer goroutine reads.
	completedDAGKeys sync.Map // string → bool (entity key → true)

	// dagStartTimes tracks when each DAG was first indexed (executionID → time.Time).
	// Written by event loop (onDAGEntry), read by sweep goroutine (sweepStaleDags).
	// sync.Map because event loop writes and sweep goroutine reads.
	dagStartTimes sync.Map // string → time.Time (executionID → start time)

	// Lifecycle.
	running  atomic.Bool
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Metrics — atomics are safe from any goroutine.
	nodesCompleted   atomic.Uint64
	nodesFailed      atomic.Uint64
	rollupsTriggered atomic.Uint64
	errorsCount      atomic.Int64
	lastActivity     atomic.Value // stores time.Time
	startTime        time.Time
}

// Ensure Component implements the required interfaces at compile time.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// =============================================================================
// SIBLING COMPONENT RESOLVERS
// =============================================================================

// resolveQuestBoard resolves questboard from the ComponentRegistry at call time.
// Returns nil when the registry is unavailable or questboard is not registered.
func (c *Component) resolveQuestBoard() QuestBoardRef {
	if c.deps.ComponentRegistry == nil {
		return nil
	}
	comp := c.deps.ComponentRegistry.Component("questboard")
	if comp == nil {
		return nil
	}
	ref, ok := comp.(QuestBoardRef)
	if !ok {
		c.logger.Warn("questboard component does not implement QuestBoardRef",
			"type", comp.Meta().Type)
		return nil
	}
	return ref
}

// resolvePartyCoord resolves partycoord from the ComponentRegistry at call time.
// Returns nil when the registry is unavailable or partycoord is not registered.
func (c *Component) resolvePartyCoord() PartyCoordRef {
	if c.deps.ComponentRegistry == nil {
		return nil
	}
	comp := c.deps.ComponentRegistry.Component("partycoord")
	if comp == nil {
		return nil
	}
	ref, ok := comp.(PartyCoordRef)
	if !ok {
		c.logger.Warn("partycoord component does not implement PartyCoordRef",
			"type", comp.Meta().Type)
		return nil
	}
	return ref
}

// =============================================================================
// DISCOVERABLE INTERFACE
// =============================================================================

// Meta returns basic component information.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        ComponentName,
		Type:        "processor",
		Description: "Reactive DAG execution for party quest decompositions",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-entities",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest entity state changes via KV watch (detects DAG init and sub-quest status transitions)",
			Config: &component.KVWatchPort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically via graph client
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "lead-review-tasks",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Lead review TaskMessages published to AGENT stream",
			Config: &component.JetStreamPort{
				StreamName: "AGENT",
				Subjects:   []string{"agent.task.*"},
			},
		},
	}
}

// ConfigSchema returns the configuration schema for this component.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"org":                  {Type: "string", Description: "Organization namespace", Default: "default", Category: "basic"},
			"platform":             {Type: "string", Description: "Platform/environment name", Default: "local", Category: "basic"},
			"board":                {Type: "string", Description: "Quest board name", Default: "main", Category: "basic"},
			"dag_timeout":          {Type: "duration", Description: "Maximum wall-clock time for a DAG to complete", Default: "30m", Category: "advanced"},
			"recruitment_timeout":  {Type: "duration", Description: "Maximum time to wait for recruitment", Default: "5m", Category: "advanced"},
			"recruitment_interval": {Type: "duration", Description: "Retry interval for recruitment", Default: "30s", Category: "advanced"},
			"max_retries_per_node": {Type: "int", Description: "Maximum retries before NodeFailed", Default: 2, Category: "advanced"},
			"stream_name":          {Type: "string", Description: "AGENT stream name for review task publishing", Default: "AGENT", Category: "basic"},
		},
		Required: []string{"org", "platform", "board"},
	}
}

// Health returns current health status.
func (c *Component) Health() component.HealthStatus {
	running := c.running.Load()
	status := component.HealthStatus{
		Healthy:    running,
		LastCheck:  time.Now(),
		ErrorCount: int(c.errorsCount.Load()),
		Uptime:     c.uptime(),
	}
	if running {
		status.Status = "running"
	} else {
		status.Status = "stopped"
	}
	if c.errorsCount.Load() > 0 {
		status.LastError = "errors encountered during DAG processing"
	}
	return status
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{}
	if lastTime, ok := c.lastActivity.Load().(time.Time); ok {
		metrics.LastActivity = lastTime
	}
	completed := c.nodesCompleted.Load()
	uptime := c.uptime().Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(completed) / uptime
	}
	if completed > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(completed)
	}
	return metrics
}

// uptime returns time since start, or zero if not yet started.
func (c *Component) uptime() time.Duration {
	if c.startTime.IsZero() {
		return 0
	}
	return time.Since(c.startTime)
}

// =============================================================================
// LIFECYCLE INTERFACE
// =============================================================================

// Initialize performs one-time setup. No I/O operations here.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}
	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}
	c.boardConfig = c.config.ToBoardConfig()
	c.stopChan = make(chan struct{})
	return nil
}

// Start begins component operation with the given context. It:
//  1. Ensures the graph bucket (ENTITY_STATES) is open
//  2. Launches 3 goroutines: 2 producers (quest KV watcher, AGENT stream review
//     consumer) + 1 event loop. The quest watcher handles both bootstrap replay
//     (priming questCache and dagCache from existing entities) and live events.
func (c *Component) Start(ctx context.Context) error {
	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client (synchronous, no I/O).
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	if err := c.graph.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("ensure board bucket: %w", err)
	}

	// Initialize in-memory state maps and the event channel.
	// These are owned exclusively by the event loop goroutine after Start returns.
	c.events = make(chan dagEvent, 256)
	c.dagCache = make(map[string]*DAGExecutionState)
	c.dagBySubQuest = make(map[string]*DAGExecutionState)
	c.questCache = make(map[string]string)
	c.reviewRetries = make(map[string]bool)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Launch producers, event loop, and sweep goroutine. produceQuestEvents
	// handles both bootstrap and live events — DAG detection and sub-quest
	// status transitions in one pass.
	c.wg.Add(4)
	go c.runEventLoop(ctx)
	go c.produceQuestEvents(ctx, c.events)
	go c.produceReviewEvents(ctx, c.events)
	go c.sweepStaleDags(ctx)

	c.logger.Info("questdagexec component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"stream", c.config.StreamName)

	return nil
}

// CancelDAGForQuest sends a cancel event for the DAG associated with the given
// parent quest entity key. The event is processed asynchronously by the event
// loop, which cancels sub-quest loops, escalates the parent quest, and disbands
// the party.
//
// The parentQuestEntityKey may be either the raw quest ID or the full entity key —
// onDAGTimedOut handles both via the dagCache lookup.
//
// Safe to call from any goroutine; does not touch event-loop-owned maps directly.
func (c *Component) CancelDAGForQuest(ctx context.Context, parentQuestEntityKey string) {
	evt := dagEvent{
		Type:        dagEventDAGTimedOut,
		EntityKey:   parentQuestEntityKey,
		ErrorReason: "cancelled via API",
	}

	select {
	case c.events <- evt:
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		c.logger.Warn("CancelDAGForQuest: timed out sending cancel event",
			"parent_quest_entity_key", parentQuestEntityKey)
	}
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(timeout time.Duration) error {
	if !c.running.Load() {
		return nil
	}

	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	close(c.stopChan)
	c.running.Store(false)

	done := make(chan struct{})
	go func() { c.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		c.logger.Warn("graceful shutdown timed out", "timeout", timeout)
	}

	c.logger.Info("questdagexec component stopped",
		"nodes_completed", c.nodesCompleted.Load(),
		"nodes_failed", c.nodesFailed.Load(),
		"rollups_triggered", c.rollupsTriggered.Load())

	return nil
}
