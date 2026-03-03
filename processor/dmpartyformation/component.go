package dmpartyformation

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
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// COMPONENT - DM Party Formation as native semstreams processor
// =============================================================================

// Component implements the DM Party Formation processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	boardConfig *domain.BoardConfig
	graph       *semdragons.GraphClient
	logger      *slog.Logger

	// Party formation infrastructure
	boids  *boidengine.DefaultBoidEngine
	engine *PartyFormationEngine

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	partiesFormed atomic.Uint64
	errorsCount   atomic.Int64
	lastActivity  atomic.Value // time.Time
	startTime     time.Time
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
		Description: "Boids-based party formation for quests",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "party-requests",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Party formation requests",
			Config: &component.NATSPort{
				Subject: "party.formation.request.>",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "party-events",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Party formed events",
			Config: &component.NATSPort{
				Subject: domain.PredicatePartyFormed,
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
			"default_strategy": {
				Type:        "string",
				Description: "Default party formation strategy",
				Default:     "balanced",
				Category:    "formation",
			},
			"max_party_size": {
				Type:        "int",
				Description: "Maximum party size",
				Default:     5,
				Category:    "formation",
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

	parties := c.partiesFormed.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(parties) / uptime
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

	// Create boid engine
	c.boids = boidengine.NewDefaultBoidEngine()

	// Create formation engine
	c.engine = NewPartyFormationEngine(c.boids, c.graph, c.boardConfig)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("dm_partyformation component started",
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
	c.logger.Info("dm_partyformation component stopped")

	return nil
}

// =============================================================================
// PUBLIC API
// =============================================================================

// FormParty assembles a party for a quest using the specified strategy.
func (c *Component) FormParty(
	ctx context.Context,
	quest *domain.Quest,
	strategy domain.PartyStrategy,
	availableAgents []agentprogression.Agent,
) (*partycoord.Party, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	party, err := c.engine.FormParty(ctx, quest, strategy, availableAgents)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "DMPartyFormation", "FormParty", "form party")
	}

	c.partiesFormed.Add(1)
	c.lastActivity.Store(time.Now())

	return party, nil
}

// RankAgentsForQuest returns agents ranked by their suitability for a quest.
func (c *Component) RankAgentsForQuest(
	agents []agentprogression.Agent,
	quest *domain.Quest,
) []boidengine.SuggestedClaim {
	if !c.running.Load() {
		return nil
	}

	return c.engine.RankAgentsForQuest(agents, quest)
}

// SuggestPartyMembers returns suggested party members with rankings.
func (c *Component) SuggestPartyMembers(
	agents []agentprogression.Agent,
	quest *domain.Quest,
	strategy domain.PartyStrategy,
) ([]PartyMemberSuggestion, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	return c.engine.SuggestPartyMembers(agents, quest, strategy)
}

// GetEngine returns the underlying party formation engine.
func (c *Component) GetEngine() *PartyFormationEngine {
	return c.engine
}

// GetBoidEngine returns the boid engine for direct access.
func (c *Component) GetBoidEngine() *boidengine.DefaultBoidEngine {
	return c.boids
}
