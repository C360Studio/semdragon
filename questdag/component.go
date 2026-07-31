package questdag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	gateddagexec "github.com/c360studio/semstreams/processor/gated-dag"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
)

// =============================================================================
// NARROW INTERFACES — sibling component dependencies
// =============================================================================

// QuestBoardRef is the narrow interface questdag needs from questboard to
// dispatch a sub-quest to a party member.
type QuestBoardRef interface {
	// ClaimAndStartForParty atomically claims a party sub-quest for a specific
	// agent and transitions it to in_progress in one write.
	ClaimAndStartForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID, assignedTo domain.AgentID) error
}

// PartyCoordRef is the narrow interface questdag needs from partycoord.
type PartyCoordRef interface {
	// GetParty returns the current party state (members, lead).
	GetParty(partyID domain.PartyID) (*partycoord.Party, bool)
	// AssignTask records a sub-quest assignment within the party (advisory).
	AssignTask(ctx context.Context, partyID domain.PartyID, subQuestID domain.QuestID, assignedTo domain.AgentID, rationale string) error
}

// =============================================================================
// COMPONENT
// =============================================================================

// Component is the questdag processor. It owns one long-lived gated-DAG executor
// over the shared dagunit prefix and a durable consumer on the dispatch subject;
// on each dispatched unit it claim-starts the unit's sub-quest on the best-skill
// party member (lead excluded). It is the reactive half of the questdag layer,
// superseding the questdagexec event loop.
//
// The member reservation table serializes callbacks within this process. The
// durable consumer provides at-least-once delivery and redelivery, but it does
// not provide a distributed reservation across component replicas. Until agent
// assignment has a cross-process CAS seam, deploy one questdag replica per board.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// executor is the owned gated-DAG executor (created + started in Start).
	executor component.LifecycleComponent
	// decodeUnit pulls a unit entity ID out of a dispatch envelope.
	decodeUnit func([]byte) (string, bool)

	// Test seams: when set, these override registry resolution.
	questBoardOverride QuestBoardRef
	partyOverride      PartyCoordRef

	running       atomic.Bool
	consumeCancel context.CancelFunc

	// reservationMu makes member selection + reservation atomic across concurrent
	// durable-consumer callbacks in this component instance.
	reservationMu sync.Mutex
	reservations  map[domain.AgentID]memberReservation

	// Metrics.
	dispatchesHandled atomic.Uint64
	claimStarts       atomic.Uint64
	errorsCount       atomic.Int64
	startTime         time.Time
}

var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// =============================================================================
// DEP RESOLUTION
// =============================================================================

func (c *Component) resolveQuestBoard() QuestBoardRef {
	if c.questBoardOverride != nil {
		return c.questBoardOverride
	}
	if c.deps.ComponentRegistry == nil {
		return nil
	}
	comp := c.deps.ComponentRegistry.Component("questboard")
	if comp == nil {
		return nil
	}
	ref, ok := comp.(QuestBoardRef)
	if !ok {
		c.logger.Warn("questboard does not implement QuestBoardRef", "type", comp.Meta().Type)
		return nil
	}
	return ref
}

func (c *Component) resolveParty() PartyCoordRef {
	if c.partyOverride != nil {
		return c.partyOverride
	}
	if c.deps.ComponentRegistry == nil {
		return nil
	}
	comp := c.deps.ComponentRegistry.Component("partycoord")
	if comp == nil {
		return nil
	}
	ref, ok := comp.(PartyCoordRef)
	if !ok {
		c.logger.Warn("partycoord does not implement PartyCoordRef", "type", comp.Meta().Type)
		return nil
	}
	return ref
}

// =============================================================================
// DISCOVERABLE
// =============================================================================

// Meta returns basic component information.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        ComponentName,
		Type:        "processor",
		Description: "Party-DAG execution on the gated-DAG executor (dispatch → claim-start on best-skill member)",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component consumes on: the gated-DAG
// dispatch subject on its durable stream.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{{
		Name:        "unit-dispatch",
		Direction:   component.DirectionInput,
		Required:    true,
		Description: "Dispatchable gated-DAG unit references from the executor",
		Config:      &component.JetStreamPort{StreamName: c.config.DispatchStream, Subjects: []string{c.config.DispatchSubject}},
	}}
}

// OutputPorts returns the ports this component produces on (none: it drives
// sub-quest claim-start via the questboard reference, not a stream).
func (c *Component) OutputPorts() []component.Port { return nil }

// ConfigSchema returns the configuration schema for this component.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"org":              {Type: "string", Description: "Organization namespace", Default: "c360", Category: "basic"},
			"platform":         {Type: "string", Description: "Platform/environment", Default: "local", Category: "basic"},
			"board":            {Type: "string", Description: "Quest board name", Default: "board1", Category: "basic"},
			"dispatch_subject": {Type: "string", Description: "Gated-DAG dispatch subject", Default: "gateddag.dispatch.quest", Category: "basic"},
			"dispatch_stream":  {Type: "string", Description: "JetStream stream backing durable dispatch", Default: "GATEDDAG_DISPATCH", Category: "basic"},
		},
		Required: []string{"org", "platform", "board"},
	}
}

// Health returns the current health status.
func (c *Component) Health() component.HealthStatus {
	running := c.running.Load()
	status := component.HealthStatus{Healthy: running, LastCheck: time.Now(), ErrorCount: int(c.errorsCount.Load()), Uptime: c.uptime()}
	if running {
		status.Status = "running"
	} else {
		status.Status = "stopped"
	}
	return status
}

// DataFlow returns current data-flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{}
	handled := c.dispatchesHandled.Load()
	if up := c.uptime().Seconds(); up > 0 {
		metrics.MessagesPerSecond = float64(handled) / up
	}
	return metrics
}

func (c *Component) uptime() time.Duration {
	if c.startTime.IsZero() {
		return 0
	}
	return time.Since(c.startTime)
}

// =============================================================================
// LIFECYCLE
// =============================================================================

// Initialize performs no-I/O setup: board config + the dispatch decoder.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("questdag: config not set")
	}
	if c.deps.NATSClient == nil {
		return errors.New("questdag: NATS client required")
	}
	if c.deps.LifecycleManager == nil {
		return errors.New("questdag: LifecycleManager required (the gated-DAG executor's Watch substrate)")
	}
	c.boardConfig = c.config.BoardConfig()

	reg := payloadregistry.New()
	if err := gateddagexec.RegisterPayloads(reg); err != nil {
		return fmt.Errorf("questdag: register gated-dag payloads: %w", err)
	}
	dec := message.NewDecoder(reg)
	c.decodeUnit = func(data []byte) (string, bool) {
		base, err := dec.Decode(data)
		if err != nil {
			return "", false
		}
		dm, ok := base.Payload().(*gateddagexec.DispatchMessage)
		if !ok {
			return "", false
		}
		return dm.UnitEntityID, true
	}
	return nil
}

// Start creates and starts the owned gated-DAG executor, then subscribes the
// durable dispatch consumer. The executor creates the dispatch stream, so it
// must start before the consumer subscribes.
func (c *Component) Start(ctx context.Context) error {
	if c.running.Load() {
		return errors.New("questdag: already running")
	}

	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Own the gated-DAG executor over the shared dagunit prefix, markers-only
	// (no FanOutInstanceID): per-DAG isolation comes from the DAG-local edge set.
	execCfg := c.gatedDAGConfig()
	execJSON, err := json.Marshal(execCfg)
	if err != nil {
		return fmt.Errorf("questdag: marshal executor config: %w", err)
	}
	exec, err := gateddagexec.NewComponent(execJSON, c.deps)
	if err != nil {
		return fmt.Errorf("questdag: create gated-dag executor: %w", err)
	}
	if err := exec.Initialize(); err != nil {
		return fmt.Errorf("questdag: executor Initialize: %w", err)
	}
	if err := exec.Start(ctx); err != nil {
		return fmt.Errorf("questdag: executor Start: %w", err)
	}
	c.executor = exec

	// Durable at-least-once dispatch consumer (ADR-070). A lost/late dispatch is
	// redelivered when the consumer resubscribes; a no-available-member handler
	// error naks so the dispatch is retried rather than dropped.
	consumeCtx, cancel := context.WithCancel(context.Background())
	c.consumeCancel = cancel
	consumeErr := c.deps.NATSClient.ConsumeDurable(consumeCtx, natsclient.StreamConsumerConfig{
		StreamName:    c.config.DispatchStream,
		ConsumerName:  c.config.ConsumerName,
		FilterSubject: c.config.DispatchSubject,
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
		AckWait:       c.config.AckWait(),
	}, c.config.Heartbeat(), c.handleDispatch)
	if consumeErr != nil {
		cancel()
		_ = exec.Stop(5 * time.Second)
		return fmt.Errorf("questdag: subscribe dispatch consumer: %w", consumeErr)
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.logger.Info("questdag started",
		"unit_prefix", UnitPrefix(c.boardConfig),
		"dispatch_subject", c.config.DispatchSubject)
	return nil
}

// gatedDAGConfig declares only Semdragon's required scope and tuning. Marker
// predicates deliberately remain unset so the framework owns their defaults.
func (c *Component) gatedDAGConfig() gateddagexec.Config {
	return gateddagexec.Config{
		UnitEntityPrefix:     UnitPrefix(c.boardConfig),
		DispatchSubject:      c.config.DispatchSubject,
		DispatchStream:       c.config.DispatchStream,
		BackstopInterval:     c.config.BackstopInterval,
		DispatchDedupeWindow: c.config.DispatchDedupeWindow,
		StrandedAfter:        c.config.StrandedAfter,
	}
}

// Stop cancels the dispatch consumer and stops the owned executor.
func (c *Component) Stop(timeout time.Duration) error {
	if !c.running.Load() {
		return nil
	}
	c.running.Store(false)
	if c.consumeCancel != nil {
		c.consumeCancel()
	}
	if c.executor != nil {
		if err := c.executor.Stop(timeout); err != nil {
			c.logger.Warn("questdag: executor stop error", "error", err)
			c.logger.Info("questdag stopped",
				"dispatches_handled", c.dispatchesHandled.Load(), "claim_starts", c.claimStarts.Load())
			return fmt.Errorf("questdag: stop executor: %w", err)
		}
	}
	c.logger.Info("questdag stopped",
		"dispatches_handled", c.dispatchesHandled.Load(), "claim_starts", c.claimStarts.Load())
	return nil
}
