package guildformation

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
// COMPONENT - GuildFormation as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Manages guild creation, membership, and promotions.
// State is maintained in-memory using sync.Map projections.
// =============================================================================

// Component implements the GuildFormation processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// Guild state - in-memory projection
	guilds sync.Map // map[domain.GuildID]*Guild

	// Agent to guild mapping - in-memory projection
	agentGuilds sync.Map // map[domain.AgentID][]domain.GuildID

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	guildsCreated   atomic.Uint64
	membersJoined   atomic.Uint64
	promotionsCount atomic.Uint64
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
		Description: "Guild formation and membership management",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-xp",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Agent XP events for auto-formation triggers",
			Config: &component.NATSPort{
				Subject: domain.PredicateAgentXP,
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "guild-events",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Guild lifecycle and membership events",
			Config: &component.NATSPort{
				Subject: domain.PredicateGuildCreated,
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
			"min_members_for_formation": {
				Type:        "int",
				Description: "Minimum members to form a guild",
				Default:     3,
				Category:    "guild",
			},
			"max_guild_size": {
				Type:        "int",
				Description: "Maximum members per guild",
				Default:     20,
				Category:    "guild",
			},
			"enable_auto_formation": {
				Type:        "bool",
				Description: "Enable automatic guild formation from skill clusters",
				Default:     true,
				Category:    "guild",
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

	operations := c.guildsCreated.Load() + c.membersJoined.Load() + c.promotionsCount.Load()
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

	c.logger.Info("guildformation component started",
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

	close(c.stopChan)

	c.running.Store(false)
	c.logger.Info("guildformation component stopped")

	return nil
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}
