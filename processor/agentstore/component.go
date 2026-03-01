// Package agent_store provides a native semstreams component for the agent
// marketplace. Agents spend XP to purchase tool access and consumables,
// creating strategic trade-offs between leveling up and acquiring capabilities.
package agentstore

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

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// COMPONENT - AgentStore as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Manages store catalog, agent inventories, and consumable effects.
// State is maintained in-memory using sync.Map projections.
// =============================================================================

// Component implements the AgentStore processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// KV watcher for agent entity state changes (entity-centric architecture)
	agentWatch  jetstream.KeyWatcher
	watchDoneCh chan struct{}

	// Agent XP cache for affordability queries without caller-supplied state.
	// Keys are entity ID strings; values are int64 XP totals.
	agentXPCache sync.Map

	// Store catalog - in-memory projection
	catalog sync.Map // map[string]*StoreItem

	// Agent inventories - in-memory projection
	inventories sync.Map // map[AgentID]*AgentInventory

	// Active effects - in-memory projection
	activeEffects sync.Map // map[AgentID][]ActiveEffect

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}
	stopOnce sync.Once

	// Metrics
	itemsListed       atomic.Uint64
	purchasesComplete atomic.Uint64
	consumablesUsed   atomic.Uint64
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
		Description: "Agent marketplace for tools and consumables",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
// Entity-centric: watches ENTITY_STATES KV for agent XP changes.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-state-watch",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Agent entity state changes via KV watch (tracks XP for affordability)",
			Config: &component.KVWatchPort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically via graph client
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "store-events",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Store operation events",
			Config: &component.NATSPort{
				Subject: domain.PredicateStoreItemPurchased,
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
			"enable_guild_discounts": {
				Type:        "bool",
				Description: "Enable guild member discounts",
				Default:     true,
				Category:    "store",
			},
			"default_rental_uses": {
				Type:        "int",
				Description: "Default uses for rental items",
				Default:     10,
				Category:    "store",
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
		status.LastError = "errors encountered during store operations"
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

	operations := c.purchasesComplete.Load() + c.consumablesUsed.Load()
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

	// Load default consumables into catalog
	for _, item := range DefaultConsumables() {
		c.catalog.Store(item.ID, &item)
	}

	// Create graph client for entity state reads
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Start KV watcher for agent entity state (entity-centric: XP changes are facts)
	watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeAgent)
	if err != nil {
		return errs.Wrap(err, "agent_store", "Start", "watch agent entity type")
	}
	c.agentWatch = watcher
	go c.processAgentWatchUpdates()

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("agent_store component started",
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

	// Stop KV watcher to unblock the watch goroutine
	if c.agentWatch != nil {
		c.agentWatch.Stop()
	}

	// Wait for watch goroutine to finish with timeout
	if c.watchDoneCh != nil {
		select {
		case <-c.watchDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("agent_store stop timed out waiting for KV watcher")
		}
	}

	c.running.Store(false)
	c.logger.Info("agent_store component stopped")

	return nil
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}
