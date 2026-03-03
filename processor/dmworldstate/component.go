package dmworldstate

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// COMPONENT - DM WorldState as native semstreams processor
// =============================================================================

// Component implements the DM WorldState processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	boardConfig *domain.BoardConfig
	graph       *semdragons.GraphClient
	logger      *slog.Logger

	// World state infrastructure
	aggregator *WorldStateAggregator

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	queriesExecuted atomic.Uint64
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
		Description: "World state aggregation and queries",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{}
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
			"max_entities_per_query": {
				Type:        "int",
				Description: "Maximum entities per query",
				Default:     1000,
				Category:    "query",
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

	queries := c.queriesExecuted.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(queries) / uptime
	}

	return metrics
}

// =============================================================================
// LIFECYCLE INTERFACE
// =============================================================================

// Initialize performs one-time setup.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}

	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}

	c.boardConfig = &domain.BoardConfig{
		Org:      c.config.Org,
		Platform: c.config.Platform,
		Board:    c.config.Board,
	}
	c.stopChan = make(chan struct{})

	return nil
}

// Start begins component operation.
// Context is unused: initialization is synchronous and in-memory.
func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Create aggregator
	c.aggregator = NewWorldStateAggregator(c.graph, c.config.MaxEntitiesPerQuery, c.logger)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("dm_worldstate component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

	return nil
}

// Stop gracefully shuts down the component.
// Timeout is unused: shutdown is non-blocking with no background goroutines to wait for.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	close(c.stopChan)
	c.running.Store(false)
	c.logger.Info("dm_worldstate component stopped")

	return nil
}

// =============================================================================
// PUBLIC API
// =============================================================================

// WorldState returns the complete state of the game world.
func (c *Component) WorldState(ctx context.Context) (*domain.WorldState, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	state, err := c.aggregator.WorldState(ctx)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "DMWorldState", "WorldState", "aggregate state")
	}

	c.queriesExecuted.Add(1)
	c.lastActivity.Store(time.Now())

	return state, nil
}

// GetIdleAgents returns all agents available to claim quests.
func (c *Component) GetIdleAgents(ctx context.Context) ([]agentprogression.Agent, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	agents, err := c.aggregator.GetIdleAgents(ctx)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "DMWorldState", "GetIdleAgents", "query agents")
	}

	c.queriesExecuted.Add(1)
	c.lastActivity.Store(time.Now())

	return agents, nil
}

// GetEscalatedQuests returns all quests needing DM attention.
func (c *Component) GetEscalatedQuests(ctx context.Context) ([]domain.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	quests, err := c.aggregator.GetEscalatedQuests(ctx)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "DMWorldState", "GetEscalatedQuests", "query quests")
	}

	c.queriesExecuted.Add(1)
	c.lastActivity.Store(time.Now())

	return quests, nil
}

// GetPendingBattles returns boss battles awaiting verdict.
func (c *Component) GetPendingBattles(ctx context.Context) ([]bossbattle.BossBattle, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	battles, err := c.aggregator.GetPendingBattles(ctx)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "DMWorldState", "GetPendingBattles", "query battles")
	}

	c.queriesExecuted.Add(1)
	c.lastActivity.Store(time.Now())

	return battles, nil
}

// GetAggregator returns the underlying world state aggregator.
func (c *Component) GetAggregator() *WorldStateAggregator {
	return c.aggregator
}
