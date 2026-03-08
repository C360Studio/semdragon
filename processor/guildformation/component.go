package guildformation

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/component"
	"github.com/nats-io/nats.go/jetstream"
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
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// Guild state - in-memory projection
	guilds sync.Map // map[domain.GuildID]*semdragons.Guild

	// Agent to guild mapping - in-memory projection
	agentGuilds sync.Map // map[domain.AgentID][]domain.GuildID

	// KV watcher for agent entity state changes (entity-centric architecture)
	agentWatch  jetstream.KeyWatcher
	watchDoneCh chan struct{}

	// Timeout loop for pending guild dissolution
	timeoutDoneCh chan struct{}

	// Agent state cache for detecting level/XP transitions that trigger clustering
	agents   map[string]*agentprogression.Agent
	agentsMu sync.RWMutex

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}
	stopOnce sync.Once

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
// Entity-centric: watches ENTITY_STATES KV for agent level/XP changes that trigger clustering.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-state-watch",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Agent entity state changes via KV watch (detects level/XP transitions for auto-formation)",
			Config: &component.KVWatchPort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically
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
			"enable_quorum_formation": {
				Type:        "bool",
				Description: "When true, new guilds start as pending and require founding quorum",
				Default:     false,
				Category:    "guild",
			},
			"min_founding_members": {
				Type:        "int",
				Description: "Number of members (including founder) required to activate a pending guild",
				Default:     3,
				Category:    "guild",
			},
			"formation_timeout_sec": {
				Type:        "int",
				Description: "Seconds before a pending guild dissolves if quorum is not met",
				Default:     300,
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
	c.agents = make(map[string]*agentprogression.Agent)
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

	// Create graph client for entity state reads and watches
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Watch agent entity type for level/XP changes that trigger auto-formation clustering
	if c.config.EnableAutoFormation {
		watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeAgent)
		if err != nil {
			return err
		}
		c.agentWatch = watcher
		c.watchDoneCh = make(chan struct{})
		go c.processAgentWatchUpdates()
	}

	// Start timeout loop for pending guild dissolution
	if c.config.EnableQuorumFormation {
		c.timeoutDoneCh = make(chan struct{})
		go c.runFormationTimeoutLoop()
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("guildformation component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"auto_formation", c.config.EnableAutoFormation,
		"quorum_formation", c.config.EnableQuorumFormation)

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

	// Stop KV watcher and wait for watch goroutine to exit
	if c.agentWatch != nil {
		c.agentWatch.Stop()
	}
	if c.watchDoneCh != nil {
		select {
		case <-c.watchDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("guildformation stop timed out waiting for KV watcher")
		}
	}
	// Wait for timeout loop to exit
	if c.timeoutDoneCh != nil {
		select {
		case <-c.timeoutDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("guildformation stop timed out waiting for timeout loop")
		}
	}

	c.running.Store(false)
	c.logger.Info("guildformation component stopped")

	return nil
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}
