// Package guildformation provides a native semstreams component for guild
// management and auto-formation suggestions. It wraps the GuildFormationEngine
// and exposes it as a semstreams component.
package guildformation

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// COMPONENT - GuildFormation as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Wraps the GuildFormationEngine to provide guild management as a component.
// =============================================================================

// Component implements GuildFormation as a semstreams processor.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	events      *semdragons.EventPublisher
	engine      *semdragons.DefaultGuildFormationEngine
	logger      *slog.Logger
	boardConfig *semdragons.BoardConfig

	// Internal state
	running atomic.Bool
	mu      sync.RWMutex

	// Metrics
	guildsCreated   atomic.Uint64
	membersAdded    atomic.Uint64
	suggestionsEmit atomic.Uint64
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
		Name:        "guildformation",
		Type:        "processor",
		Description: "Guild management and auto-formation suggestions",
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
			Description: "Agent state for cluster detection",
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
			Name:        "guild-suggested",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Guild formation suggestions",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateGuildSuggested,
			},
		},
		{
			Name:        "guild-joined",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Guild membership events",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateGuildAutoJoined,
			},
		},
		{
			Name:        "guild-state",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Guild state updates in KV",
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
			"min_founder_level": {
				Type:        "int",
				Description: "Minimum level to found guild (default 11)",
				Default:     11,
				Category:    "founding",
			},
			"founding_xp_cost": {
				Type:        "int",
				Description: "XP cost to found guild (default 500)",
				Default:     500,
				Category:    "founding",
			},
			"default_max_members": {
				Type:        "int",
				Description: "Default max members per guild (default 20)",
				Default:     20,
				Category:    "membership",
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
		status.LastError = "errors encountered during guild operations"
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

	operations := c.guildsCreated.Load() + c.membersAdded.Load()
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
		return errs.Wrap(err, "GuildFormation", "Start", "create graph client")
	}

	// Create event publisher
	c.events = semdragons.NewEventPublisher(c.deps.NATSClient)

	// Create formation engine
	formationConfig := c.config.ToFormationConfig()
	c.engine = semdragons.NewGuildFormationEngine(c.graph, c.events, formationConfig)
	c.engine.WithLogger(c.logger)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("guildformation component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

	return nil
}

// Stop gracefully shuts down the component.
// The timeout parameter is part of the LifecycleComponent interface but is not
// used as this component has no background goroutines requiring coordination.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	c.running.Store(false)
	c.logger.Info("guildformation component stopped")

	return nil
}

// createGraphClient creates the graph client for the component.
func (c *Component) createGraphClient(_ context.Context) error {
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	return nil
}
