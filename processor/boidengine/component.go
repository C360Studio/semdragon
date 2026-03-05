// Package boidengine provides a native semstreams component for emergent agent
// behavior coordination. It implements boid flocking rules to suggest quest claims
// based on agent skills, preferences, and current state.
package boidengine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/boardcontrol"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// COMPONENT - BoidEngine as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Watches agent and quest state via KV, computes attractions periodically,
// and publishes suggestion events.
// =============================================================================

// Component implements the BoidEngine as a semstreams processor.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	boidEngine  BoidEngine
	logger      *slog.Logger
	boardConfig *domain.BoardConfig
	rules       BoidRules

	// KV watches for real-time state updates
	agentWatch jetstream.KeyWatcher
	questWatch jetstream.KeyWatcher
	guildWatch jetstream.KeyWatcher

	// Cached state
	agents   map[string]*agentprogression.Agent
	quests   map[string]*domain.Quest
	guilds   map[string]*domain.Guild
	agentsMu sync.RWMutex
	questsMu sync.RWMutex
	guildsMu sync.RWMutex

	// KV bucket for persisting boid suggestions (nil when disabled)
	suggestionsBucket jetstream.KeyValue

	// Board pause integration
	pauseChecker boardcontrol.PauseChecker // Optional: nil means always-running

	// Internal state
	running     atomic.Bool
	mu          sync.RWMutex
	stopChan    chan struct{}
	doneChan    chan struct{}
	watchDoneCh chan struct{} // Signals watch goroutines are done
	stopOnce    sync.Once

	// Metrics
	suggestionsComputed atomic.Uint64
	errorsCount         atomic.Int64
	lastActivity        atomic.Value // time.Time
	startTime           time.Time
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
		Name:        "boidengine",
		Type:        "processor",
		Description: "Emergent agent behavior coordination using boid flocking rules",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-state",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Agent state from KV (polled)",
			Config: &component.KVWatchPort{
				Bucket: "", // Set dynamically from config
			},
		},
		{
			Name:        "quest-state",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest state from KV (polled)",
			Config: &component.KVWatchPort{
				Bucket: "", // Set dynamically from config
			},
		},
		{
			Name:        "guild-state",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Guild state from KV for rank/reputation in affinity scoring",
			Config: &component.KVWatchPort{
				Bucket: "", // Set dynamically from config
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "boid-suggestions",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Quest claim suggestions",
			Config: &component.NATSPort{
				Subject: "boid.suggestions.*",
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
			"separation_weight": {
				Type:        "float",
				Description: "Avoid quest overlap weight (default 1.0)",
				Default:     1.0,
				Category:    "rules",
			},
			"alignment_weight": {
				Type:        "float",
				Description: "Align with peers weight (default 0.8)",
				Default:     0.8,
				Category:    "rules",
			},
			"cohesion_weight": {
				Type:        "float",
				Description: "Move toward skill clusters weight (default 0.6)",
				Default:     0.6,
				Category:    "rules",
			},
			"hunger_weight": {
				Type:        "float",
				Description: "Idle time urgency weight (default 1.2)",
				Default:     1.2,
				Category:    "rules",
			},
			"affinity_weight": {
				Type:        "float",
				Description: "Skill/guild match weight (default 1.5)",
				Default:     1.5,
				Category:    "rules",
			},
			"caution_weight": {
				Type:        "float",
				Description: "Avoid over-leveled quests weight (default 0.9)",
				Default:     0.9,
				Category:    "rules",
			},
			"update_interval_ms": {
				Type:        "int",
				Description: "How often to recompute suggestions (default 1000)",
				Default:     1000,
				Category:    "timing",
			},
			"neighbor_radius": {
				Type:        "int",
				Description: "How many nearby agents to consider (default 5)",
				Default:     5,
				Category:    "timing",
			},
			"boid_suggestions_bucket": {
				Type:        "string",
				Description: "KV bucket for persisting boid suggestions per agent (empty disables)",
				Default:     "BOID_SUGGESTIONS",
				Category:    "observability",
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
		status.LastError = "errors encountered during boid computation"
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

	computed := c.suggestionsComputed.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(computed) / uptime
	}

	if computed > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(computed)
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

	// Validate configuration
	if err := c.config.Validate(); err != nil {
		return errs.Wrap(err, "BoidEngine", "Initialize", "invalid config")
	}

	c.boardConfig = c.config.ToBoardConfig()
	c.rules = c.config.ToBoidRules()
	c.boidEngine = NewDefaultBoidEngine()
	c.agents = make(map[string]*agentprogression.Agent)
	c.quests = make(map[string]*domain.Quest)
	c.guilds = make(map[string]*domain.Guild)
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})
	c.watchDoneCh = make(chan struct{})

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client
	if err := c.createGraphClient(ctx); err != nil {
		return err
	}

	// Create BOID_SUGGESTIONS KV bucket when configured. Suggestions are ephemeral
	// (TTL 5 minutes) and observable via the message-logger SSE infrastructure.
	if c.config.BoidSuggestionsBucket != "" {
		bucket, err := c.deps.NATSClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
			Bucket:      c.config.BoidSuggestionsBucket,
			Description: "Ephemeral boid suggestions per agent",
			History:     5,
			TTL:         5 * time.Minute,
		})
		if err != nil {
			return fmt.Errorf("create boid suggestions bucket: %w", err)
		}
		c.suggestionsBucket = bucket
	}

	// Load initial state
	if err := c.loadInitialState(ctx); err != nil {
		return errs.Wrap(err, "BoidEngine", "Start", "load initial state")
	}

	// Set up KV watchers for real-time state updates
	if err := c.startKVWatchers(ctx); err != nil {
		return errs.Wrap(err, "BoidEngine", "Start", "start KV watchers")
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Start periodic computation
	go c.runComputeLoop()

	c.logger.Info("boidengine component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"update_interval_ms", c.config.UpdateIntervalMs)

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	// Signal stop using sync.Once to prevent double-close panic
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})

	// Wait for compute loop to finish with timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Wait for compute loop
	select {
	case <-c.doneChan:
		// Clean shutdown
	case <-time.After(timeout):
		c.logger.Warn("boidengine stop timed out waiting for compute loop")
	}

	// Stop KV watchers
	c.stopKVWatchers()

	// Wait for watch goroutines
	select {
	case <-c.watchDoneCh:
		// Watch goroutines stopped
	case <-time.After(timeout):
		c.logger.Warn("boidengine stop timed out waiting for KV watchers")
	}

	c.running.Store(false)
	c.logger.Info("boidengine component stopped")

	return nil
}

// SetPauseChecker injects the board pause checker. When paused, the compute
// loop skips suggestion computation. SetPauseChecker is ignored once running.
func (c *Component) SetPauseChecker(pc boardcontrol.PauseChecker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		c.logger.Warn("SetPauseChecker called while running; ignored")
		return
	}
	c.pauseChecker = pc
}
