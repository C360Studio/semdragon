// Package questboard provides a native semstreams component for quest lifecycle management.
// This processor handles quest posting, claiming, execution, and completion as events
// flow through the system via JetStream, with state persisted in NATS KV.
package questboard

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/model"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/internal/util"
	"github.com/c360studio/semdragons/processor/dmapproval"
)

// =============================================================================
// COMPONENT - QuestBoard as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Manages quest lifecycle: post → claim → start → submit → complete/fail.
// State stored in KV, events emitted via JetStream subjects.
//
// File organization:
// - component.go: Core Component struct, interfaces, lifecycle
// - config.go: Config struct, defaults, validation
// - handler.go: Quest operation methods (PostQuest, ClaimQuest, etc.)
// - register.go: Factory and registry registration
// =============================================================================

// Component implements the QuestBoard as a semstreams processor.
type Component struct {
	config *Config
	deps   component.Dependencies
	graph  *semdragons.GraphClient
	traces *semdragons.TraceManager
	logger *slog.Logger

	// Internal state
	boardConfig *domain.BoardConfig
	running     atomic.Bool
	mu          sync.RWMutex

	// Triage watcher (auto-triage for pending_triage quests)
	triageWatch   jetstream.KeyWatcher
	triageDoneCh  chan struct{}
	triageStopCh  chan struct{}
	triageCache   sync.Map // entityKey → domain.QuestStatus
	registry      model.RegistryReader
	approval      dmapproval.ApprovalRouter

	// Metrics
	messagesProcessed atomic.Uint64
	errorsCount       atomic.Int64
	lastActivity      atomic.Value // time.Time
	startTime         time.Time
}

// ensure Component implements the required interfaces.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// =============================================================================
// DISCOVERABLE INTERFACE
// =============================================================================

// Meta returns basic component information.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "questboard",
		Type:        "processor",
		Description: "Quest lifecycle management - posting, claiming, execution, completion",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
// QuestBoard is primarily API-driven, but can also react to events.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-commands",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Command messages for quest operations (post, claim, start, submit, complete, fail)",
			Config: &component.NATSRequestPort{
				Subject: "questboard.command.>",
				Timeout: "30s",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-lifecycle",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Quest lifecycle events (posted, claimed, started, submitted, completed, failed, escalated, abandoned)",
			Config: &component.JetStreamPort{
				StreamName:      "QUEST_EVENTS",
				Subjects:        []string{"quest.lifecycle.>"},
				Storage:         "file",
				RetentionPolicy: "limits",
				RetentionDays:   30,
				Replicas:        1,
			},
		},
		{
			Name:        "battle-events",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Boss battle review events (started, verdict)",
			Config: &component.JetStreamPort{
				StreamName:      "BATTLE_EVENTS",
				Subjects:        []string{"battle.review.>"},
				Storage:         "file",
				RetentionPolicy: "limits",
				RetentionDays:   30,
				Replicas:        1,
			},
		},
		{
			Name:        "quest-state",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Quest state persisted in KV",
			Config: &component.KVWritePort{
				Bucket: "", // Set dynamically from config
			},
		},
	}
}

// ConfigSchema returns the configuration schema for this component.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"org": {
				Type:        "string",
				Description: "Organization namespace (e.g., 'c360')",
				Default:     "default",
				Category:    "basic",
			},
			"platform": {
				Type:        "string",
				Description: "Platform/environment name (e.g., 'prod', 'dev')",
				Default:     "local",
				Category:    "basic",
			},
			"board": {
				Type:        "string",
				Description: "Quest board name",
				Default:     "main",
				Category:    "basic",
			},
			"default_max_attempts": {
				Type:        "int",
				Description: "Default maximum attempts for quests",
				Default:     3,
				Minimum:     util.IntPtr(1),
				Category:    "basic",
			},
			"enable_evaluation": {
				Type:        "bool",
				Description: "Enable automatic boss battle evaluation",
				Default:     true,
				Category:    "advanced",
			},
		},
		Required: []string{"org", "platform", "board"},
	}
}

// Health returns current health status.
func (c *Component) Health() component.HealthStatus {
	status := component.HealthStatus{
		Healthy:    c.running.Load(),
		LastCheck:  time.Now(),
		ErrorCount: int(c.errorsCount.Load()),
		Uptime:     time.Since(c.startTime),
	}

	if c.running.Load() {
		status.Status = "running"
	} else {
		status.Status = "stopped"
	}

	if c.errorsCount.Load() > 0 {
		status.LastError = "errors encountered during processing"
	}

	return status
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         0,
	}

	if lastTime, ok := c.lastActivity.Load().(time.Time); ok {
		metrics.LastActivity = lastTime
	}

	processed := c.messagesProcessed.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(processed) / uptime
	}

	if processed > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(processed)
	}

	return metrics
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
	c.traces = semdragons.NewTraceManager()

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client for graph system access
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Resolve model registry from deps (optional — triage degrades gracefully without it)
	if c.deps.ModelRegistry != nil {
		c.registry = c.deps.ModelRegistry
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Start triage watcher if triage is enabled
	if c.config.Triage.Enabled {
		watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeQuest)
		if err != nil {
			c.running.Store(false)
			return errors.New("start triage watcher: " + err.Error())
		}
		c.triageWatch = watcher
		c.triageDoneCh = make(chan struct{})
		c.triageStopCh = make(chan struct{})
		go c.processTriageWatchUpdates()

		c.logger.Info("triage watcher started",
			"dm_mode", c.config.Triage.DMMode,
			"min_difficulty", c.config.Triage.MinDifficultyForTriage,
			"timeout_mins", c.config.Triage.TriageTimeoutMins)
	}

	c.logger.Info("questboard component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"bucket", c.boardConfig.BucketName(),
		"triage_enabled", c.config.Triage.Enabled)

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

	// Signal triage watcher to stop
	if c.triageStopCh != nil {
		close(c.triageStopCh)
	}
	if c.triageWatch != nil {
		c.triageWatch.Stop()
	}

	// Wait for triage watcher goroutine with timeout
	if c.triageDoneCh != nil {
		select {
		case <-c.triageDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("stop timed out waiting for triage watcher")
		}
	}

	c.running.Store(false)
	c.logger.Info("questboard component stopped")

	return nil
}

// SetApproval injects the approval router for assisted/supervised triage modes.
// Must be called before Start.
func (c *Component) SetApproval(approval dmapproval.ApprovalRouter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		c.logger.Warn("SetApproval called while running; ignored")
		return
	}
	c.approval = approval
}
