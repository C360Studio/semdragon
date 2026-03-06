// Package questdagexec implements reactive DAG execution for party quests.
// When a lead agent's questbridge handler posts sub-quests and writes a
// DAGExecutionState to QUEST_DAGS, this component watches sub-quest KV entity
// transitions and drives the state machine: node assignment, lead review
// dispatch, rollup, and failure escalation.
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
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
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
	// ClaimQuestForParty claims a sub-quest on behalf of a party.
	ClaimQuestForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID) error
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
// COMPONENT - QuestDAGExec as a native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Watches quest entity KV for sub-quest status transitions, drives DAG state,
// dispatches lead review TaskMessages, and triggers rollup on completion.
// Lock ordering: mu must be held before any per-DAG map operation when both
// mu and dagBySubQuest/dagCache are needed simultaneously.
// =============================================================================

// Component implements the questdagexec processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// QUEST_DAGS KV bucket — shared with questbridge (written there, watched here).
	questDagsBucket jetstream.KeyValue

	// Sibling component references injected before Start.
	questBoardRef QuestBoardRef // see SetQuestBoard
	partyCoord    PartyCoordRef // see SetPartyCoord

	// questCache stores last-known quest status (entity key → status string).
	// Populated during bootstrap; consulted on every live update to detect transitions.
	questCache sync.Map // map[string]string

	// dagBySubQuest maps sub-quest entity ID → *DAGExecutionState.
	// Built on startup from QUEST_DAGS bucket; updated on each DAG mutation.
	// Allows O(1) lookup when a sub-quest KV event arrives.
	dagBySubQuest sync.Map // map[string]*DAGExecutionState

	// dagCache is the in-memory store of all known DAGExecutionState values
	// keyed by ExecutionID. Updated on every mutation before KV persist.
	dagCache sync.Map // map[string]*DAGExecutionState

	// Internal lifecycle state.
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Metrics.
	nodesCompleted atomic.Uint64
	nodesFailed    atomic.Uint64
	rollupsTriggered atomic.Uint64
	errorsCount    atomic.Int64
	lastActivity   atomic.Value // time.Time
	startTime      time.Time
}

// Ensure Component implements the required interfaces at compile time.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// =============================================================================
// INJECTION METHODS
// =============================================================================

// SetQuestBoard injects the quest board reference for quest state transitions.
// Safe to call before or after Start — the reference is checked lazily when needed.
func (c *Component) SetQuestBoard(qb QuestBoardRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.questBoardRef = qb
}

// SetPartyCoord injects the party coordination reference.
// Safe to call before or after Start — the reference is checked lazily when needed.
func (c *Component) SetPartyCoord(pc PartyCoordRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.partyCoord = pc
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
			Description: "Sub-quest entity state changes via KV watch (detects status transitions)",
			Config: &component.KVWatchPort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically via graph client
			},
		},
		{
			Name:        "dag-states",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "DAGExecutionState entries from QUEST_DAGS bucket (written by questbridge)",
			Config: &component.KVWritePort{
				Bucket: "", // Set dynamically from config
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
			"org":                   {Type: "string", Description: "Organization namespace", Default: "default", Category: "basic"},
			"platform":              {Type: "string", Description: "Platform/environment name", Default: "local", Category: "basic"},
			"board":                 {Type: "string", Description: "Quest board name", Default: "main", Category: "basic"},
			"dag_timeout":           {Type: "duration", Description: "Maximum wall-clock time for a DAG to complete", Default: "30m", Category: "advanced"},
			"recruitment_timeout":   {Type: "duration", Description: "Maximum time to wait for recruitment", Default: "5m", Category: "advanced"},
			"recruitment_interval":  {Type: "duration", Description: "Retry interval for recruitment", Default: "30s", Category: "advanced"},
			"max_retries_per_node":  {Type: "int", Description: "Maximum retries before NodeFailed", Default: 2, Category: "advanced"},
			"quest_dags_bucket":     {Type: "string", Description: "QUEST_DAGS KV bucket name", Default: "QUEST_DAGS", Category: "basic"},
			"stream_name":           {Type: "string", Description: "AGENT stream name for review task publishing", Default: "AGENT", Category: "basic"},
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

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client (synchronous, no I/O).
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	if err := c.graph.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("ensure board bucket: %w", err)
	}

	// Get the QUEST_DAGS bucket. Questbridge creates it; we just open it.
	// Use CreateKeyValueBucket (idempotent get-or-create) so startup order
	// between questbridge and questdagexec doesn't matter.
	dagsBucket, err := c.deps.NATSClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      c.config.QuestDagsBucket,
		Description: "DAG execution state for party quest decompositions",
		History:     10,
	})
	if err != nil {
		return errs.Wrap(err, ComponentName, "Start", "open QUEST_DAGS bucket")
	}
	c.questDagsBucket = dagsBucket

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Bootstrap DAG index from existing QUEST_DAGS entries, then watch for
	// sub-quest entity transitions.
	c.wg.Add(1)
	go func() { defer c.wg.Done(); c.watchLoop(ctx) }()

	c.logger.Info("questdagexec component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"bucket", c.config.QuestDagsBucket,
		"stream", c.config.StreamName)

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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
		c.logger.Warn("graceful shutdown timed out waiting for DAG watcher", "timeout", timeout)
	}

	c.logger.Info("questdagexec component stopped",
		"nodes_completed", c.nodesCompleted.Load(),
		"nodes_failed", c.nodesFailed.Load(),
		"rollups_triggered", c.rollupsTriggered.Load())

	return nil
}
