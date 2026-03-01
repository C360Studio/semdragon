// Package autonomy provides a native semstreams component for agent autonomy.
// It watches agent state and boid suggestions, manages per-agent heartbeat timers,
// evaluates possible actions on each heartbeat, and detects cooldown expiry.
package autonomy

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// COMPONENT - Autonomy processor as native semstreams component
// =============================================================================
// Watches agent state via KV and boid suggestions via NATS pub/sub.
// Manages per-agent heartbeat timers with state-dependent cadence.
// Evaluates what actions each agent can take on each heartbeat.
// Detects cooldown expiry and transitions agents back to idle.
// =============================================================================

// Component implements the autonomy processor as a semstreams component.
//
// Lock ordering: mu must be acquired before trackersMu when both are needed.
// mu guards lifecycle operations (Start/Stop); trackersMu guards the per-agent
// tracker map. Never hold trackersMu across I/O operations.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// KV watcher for agent entity state changes
	agentWatch  jetstream.KeyWatcher
	watchDoneCh chan struct{}

	// NATS subscription for boid suggestions
	boidSub *natsclient.Subscription

	// Per-agent heartbeat trackers
	trackers   map[string]*agentTracker
	trackersMu sync.RWMutex

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}
	stopOnce sync.Once

	// Metrics
	evaluationsRun   atomic.Uint64
	cooldownsExpired atomic.Uint64
	errorsCount      atomic.Int64
	lastActivity     atomic.Value // time.Time
	startTime        time.Time
}

// Ensure Component implements the required interfaces.
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
		Description: "Agent autonomy heartbeat and action evaluation",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-state-watch",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Agent entity state changes via KV watch",
			Config: &component.KVWatchPort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically via graph client
			},
		},
		{
			Name:        "boid-suggestions",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Boid engine quest suggestions via NATS pub/sub",
			Config: &component.NATSPort{
				Subject: "boid.suggestions.*",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "autonomy-evaluated",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Heartbeat evaluation events",
			Config: &component.NATSPort{
				Subject: domain.PredicateAutonomyEvaluated,
			},
		},
		{
			Name:        "autonomy-idle",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Agent idle with nothing actionable",
			Config: &component.NATSPort{
				Subject: domain.PredicateAutonomyIdle,
			},
		},
		{
			Name:        "claim-state",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Quest and agent entity state updates from autonomous claims",
			Config: &component.KVWritePort{
				Bucket: "",
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
			"initial_delay_ms": {
				Type:        "int",
				Description: "Delay before first heartbeat (ms)",
				Default:     2000,
				Category:    "heartbeat",
			},
			"idle_interval_ms": {
				Type:        "int",
				Description: "Heartbeat interval when idle (ms)",
				Default:     5000,
				Category:    "heartbeat",
			},
			"on_quest_interval_ms": {
				Type:        "int",
				Description: "Heartbeat interval when on quest (ms)",
				Default:     30000,
				Category:    "heartbeat",
			},
			"in_battle_interval_ms": {
				Type:        "int",
				Description: "Heartbeat interval when in battle (ms)",
				Default:     60000,
				Category:    "heartbeat",
			},
			"cooldown_interval_ms": {
				Type:        "int",
				Description: "Heartbeat interval when on cooldown (ms)",
				Default:     15000,
				Category:    "heartbeat",
			},
			"max_interval_ms": {
				Type:        "int",
				Description: "Maximum heartbeat interval (ms)",
				Default:     60000,
				Category:    "heartbeat",
			},
			"backoff_factor": {
				Type:        "float",
				Description: "Backoff multiplier for idle agents",
				Default:     1.5,
				Category:    "heartbeat",
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
		status.LastError = "errors encountered during autonomy evaluation"
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

	evaluations := c.evaluationsRun.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(evaluations) / uptime
	}

	if evaluations > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(evaluations)
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
	c.stopChan = make(chan struct{})
	c.watchDoneCh = make(chan struct{})
	c.trackers = make(map[string]*agentTracker)

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client for entity state reads/writes
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Start KV watcher for agent entity state
	watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeAgent)
	if err != nil {
		return errs.Wrap(err, "autonomy", "Start", "watch agent entity type")
	}
	c.agentWatch = watcher

	// Subscribe to boid suggestions (NATS pub/sub, not KV)
	boidSub, err := c.deps.NATSClient.Subscribe(ctx, "boid.suggestions.>", c.handleBoidSuggestion)
	if err != nil {
		c.agentWatch.Stop()
		return errs.Wrap(err, "autonomy", "Start", "subscribe boid suggestions")
	}
	c.boidSub = boidSub

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Start watch goroutine
	go c.processAgentWatchUpdates()

	c.logger.Info("autonomy component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

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

	// Signal stop using sync.Once to prevent double-close panic
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})

	// Unsubscribe from boid suggestions
	if c.boidSub != nil {
		c.boidSub.Unsubscribe()
	}

	// Stop KV watcher to unblock the watch goroutine
	if c.agentWatch != nil {
		c.agentWatch.Stop()
	}

	// Wait for watch goroutine to finish with timeout
	if c.watchDoneCh != nil {
		select {
		case <-c.watchDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("autonomy stop timed out waiting for KV watcher")
		}
	}

	// Set running=false BEFORE cancelling timers so that any timer callback
	// that fires between Stop() and cancelAllHeartbeats() sees the flag and
	// returns immediately without calling evaluateAutonomy.
	c.running.Store(false)
	c.cancelAllHeartbeats()

	c.logger.Info("autonomy component stopped")

	return nil
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}
