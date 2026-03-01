// Package agent_progression provides a native semstreams component for XP calculation and
// agent progression management. It watches quest entity state via KV for completion and
// failure transitions, reads agent state directly from KV, calculates XP awards/penalties,
// and emits progression updates via entity state writes.
package agent_progression

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
	graphclient "github.com/c360studio/semdragons/graph"
	"github.com/c360studio/semdragons/internal/util"
)

// =============================================================================
// COMPONENT - Agent progression as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Watches quest entity state via KV for completed/failed transitions.
// Reads agent state from KV, calculates XP awards/penalties, and
// emits agent progression updates via entity state writes.
//
// File organization:
// - component.go: Core Component struct, interfaces, lifecycle
// - config.go: Config struct, defaults, validation
// - handler.go: KV watch handlers and entity state helpers
// - register.go: Factory and registry registration
// - xp.go: XP calculation logic
// - agent.go: Agent type definition
// - payloads.go: XP payload types (Graphable entities)
// =============================================================================

// Component implements the agent progression processor as a semstreams component.
type Component struct {
	config   *Config
	deps     component.Dependencies
	graph    *graphclient.GraphClient
	xpEngine XPEngine
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// KV watcher for quest entity state changes (entity-centric architecture)
	questWatch  jetstream.KeyWatcher
	watchDoneCh chan struct{}

	// Quest state cache for detecting transitions
	questCache sync.Map // map[entityID]domain.QuestStatus

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

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
		Name:        ComponentName,
		Type:        "processor",
		Description: "XP calculation and agent progression management",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
// Entity-centric: watches ENTITY_STATES KV for quest status transitions to completed/failed.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-state-watch",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest entity state changes via KV watch (detects completed/failed transitions)",
			Config: &component.KVWritePort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-xp",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "XP change events",
			Config: &component.JetStreamPort{
				StreamName:      "AGENT_EVENTS",
				Subjects:        []string{"agent.progression.>"},
				Storage:         "file",
				RetentionPolicy: "limits",
				RetentionDays:   30,
				Replicas:        1,
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
				Description: "Organization namespace",
				Default:     "default",
				Category:    "basic",
			},
			"platform": {
				Type:        "string",
				Description: "Platform/environment name",
				Default:     "local",
				Category:    "basic",
			},
			"board": {
				Type:        "string",
				Description: "Quest board name",
				Default:     "main",
				Category:    "basic",
			},
			"quality_multiplier": {
				Type:        "float",
				Description: "Quality bonus multiplier (default 2.0)",
				Default:     2.0,
				Category:    "advanced",
			},
			"speed_multiplier": {
				Type:        "float",
				Description: "Speed bonus multiplier (default 0.5)",
				Default:     0.5,
				Category:    "advanced",
			},
			"streak_multiplier": {
				Type:        "float",
				Description: "Streak bonus multiplier (default 0.1)",
				Default:     0.1,
				Category:    "advanced",
			},
			"retry_penalty_rate": {
				Type:        "float",
				Description: "XP penalty rate per retry attempt",
				Default:     0.25,
				Category:    "advanced",
			},
			"failure_penalty_rate": {
				Type:        "float",
				Description: "XP penalty rate for failures",
				Default:     0.5,
				Category:    "advanced",
			},
			"level_down_threshold": {
				Type:        "int",
				Description: "Consecutive failures before level demotion",
				Default:     3,
				Minimum:     util.IntPtr(1),
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
	c.xpEngine = c.config.ToXPEngine()
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

	// Create graph client for entity state reads
	c.graph = graphclient.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Watch quest entity type for status transitions to completed/failed (entity-centric)
	watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeQuest)
	if err != nil {
		c.running.Store(false)
		return errs.Wrap(err, "agent_progression", "Start", "watch quest entity type")
	}
	c.questWatch = watcher
	c.watchDoneCh = make(chan struct{})
	go c.processQuestWatchUpdates()

	c.logger.Info("agent_progression component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	// Close stop channel
	close(c.stopChan)

	// Stop KV watcher
	if c.questWatch != nil {
		c.questWatch.Stop()
	}

	// Wait for watch goroutine to finish
	if c.watchDoneCh != nil {
		<-c.watchDoneCh
	}

	c.running.Store(false)
	c.logger.Info("agent_progression component stopped")

	return nil
}
