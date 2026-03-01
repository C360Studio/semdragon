// Package agent_store provides a native semstreams component for the agent
// marketplace. Agents spend XP to purchase tool access and consumables,
// creating strategic trade-offs between leveling up and acquiring capabilities.
package agent_store

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"

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
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

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
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-xp",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Agent XP events for purchase validation",
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

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Load default consumables into catalog
	for _, item := range DefaultConsumables() {
		c.catalog.Store(item.ID, &item)
	}

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
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	close(c.stopChan)

	c.running.Store(false)
	c.logger.Info("agent_store component stopped")

	return nil
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}
