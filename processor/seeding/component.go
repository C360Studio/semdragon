package seeding

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/component"
)

// =============================================================================
// COMPONENT - Seeding as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Provides environment bootstrapping for agents and guilds.
// Supports two modes: Training Arena and Tiered Roster.
// =============================================================================

// Component implements the Seeding processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	seedingSessions atomic.Uint64
	agentsSeeded    atomic.Uint64
	guildsSeeded    atomic.Uint64
	errorsCount     atomic.Int64
	lastActivity    atomic.Value // time.Time
	startTime       time.Time
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
		Description: "Environment seeding for agents and guilds",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "seed-request",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Seeding requests from CLI or API",
			Config: &component.NATSPort{
				Subject: "seeding.request",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "seeding-events",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Seeding lifecycle and entity creation events",
			Config: &component.NATSPort{
				Subject: "seeding.>",
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
			"mode": {
				Type:        "string",
				Description: "Seeding mode: training_arena or tiered_roster",
				Default:     "tiered_roster",
				Category:    "seeding",
			},
			"dry_run": {
				Type:        "bool",
				Description: "Log actions without executing",
				Default:     false,
				Category:    "seeding",
			},
			"idempotent": {
				Type:        "bool",
				Description: "Skip existing agents by name",
				Default:     true,
				Category:    "seeding",
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
		status.LastError = "errors encountered during seeding"
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

	operations := c.agentsSeeded.Load() + c.guildsSeeded.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(operations) / uptime
	}

	if operations > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(operations)
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

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("seeding component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"mode", c.config.Mode)

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	close(c.stopChan)

	c.running.Store(false)
	c.logger.Info("seeding component stopped")

	return nil
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}
