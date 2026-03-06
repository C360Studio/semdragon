// Package partycoord provides a native semstreams component for party
// coordination and quest decomposition. It manages party lifecycle and
// coordinates communication between party leads and members.
package partycoord

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
	"github.com/c360studio/semdragons/internal/util"
)

// =============================================================================
// COMPONENT - PartyCoord as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Manages party formation, coordination, and rollup for quest decomposition.
// =============================================================================

// QuestBoardRef is the narrow interface partycoord needs from questboard
// for claiming party-required quests on behalf of a lead agent.
type QuestBoardRef interface {
	ClaimQuestForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID) error
	StartQuest(ctx context.Context, questID domain.QuestID) error
}

// Component implements the PartyCoord processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// Sibling component reference injected before Start.
	questBoardRef QuestBoardRef // see SetQuestBoard

	// KV watch for quest state changes (facts about the world)
	questWatch jetstream.KeyWatcher

	// Cached quest state for reactive auto-formation decisions
	quests   map[string]*domain.Quest
	questsMu sync.RWMutex

	// Party tracking
	activeParties sync.Map // map[PartyID]*Party

	// Internal state
	running     atomic.Bool
	mu          sync.RWMutex
	stopChan    chan struct{}
	watchDoneCh chan struct{} // Signals watch goroutine is done
	stopOnce    sync.Once

	// Metrics
	partiesFormed    atomic.Uint64
	partiesDisbanded atomic.Uint64
	rollupsCompleted atomic.Uint64
	errorsCount      atomic.Int64
	lastActivity     atomic.Value // time.Time
	startTime        time.Time
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
		Name:        "partycoord",
		Type:        "processor",
		Description: "Party coordination and quest decomposition management",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-state",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest state from KV — facts about quests (claimed, completed) trigger auto-formation",
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
			Name:        "party-formed",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Party formation events",
			Config: &component.NATSPort{
				Subject: domain.PredicatePartyFormed,
			},
		},
		{
			Name:        "party-disbanded",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Party disbanding events",
			Config: &component.NATSPort{
				Subject: domain.PredicatePartyDisbanded,
			},
		},
		{
			Name:        "party-state",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Party state updates in KV",
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
			"default_max_party_size": {
				Type:        "int",
				Description: "Default max party members (default 5)",
				Default:     5,
				Minimum:     util.IntPtr(2),
				Category:    "parties",
			},
			"formation_timeout": {
				Type:        "duration",
				Description: "Timeout for party formation (default 10m)",
				Default:     "10m",
				Category:    "parties",
			},
			"auto_form_parties": {
				Type:        "bool",
				Description: "Auto-form parties for party-required quests",
				Default:     true,
				Category:    "parties",
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
		status.LastError = "errors encountered during party coordination"
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

	operations := c.partiesFormed.Load() + c.partiesDisbanded.Load()
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
	c.quests = make(map[string]*domain.Quest)
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

	// Create graph client
	if err := c.createGraphClient(ctx); err != nil {
		return errs.Wrap(err, "PartyCoord", "Start", "create graph client")
	}

	// Load initial quest state so the cache is populated before watching
	if err := c.loadInitialQuestState(ctx); err != nil {
		return errs.Wrap(err, "PartyCoord", "Start", "load initial quest state")
	}

	// Set up KV watcher to react to quest state changes (facts about the world)
	if err := c.startQuestWatcher(ctx); err != nil {
		return errs.Wrap(err, "PartyCoord", "Start", "start quest KV watcher")
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("partycoord component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"auto_form", c.config.AutoFormParties)

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

	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Stop the KV watcher, which unblocks the watch goroutine
	c.stopQuestWatcher()

	// Wait for watch goroutine to finish
	select {
	case <-c.watchDoneCh:
		// Clean shutdown
	case <-time.After(timeout):
		c.logger.Warn("partycoord stop timed out waiting for KV watcher goroutine")
	}

	c.running.Store(false)
	c.logger.Info("partycoord component stopped")

	return nil
}

// =============================================================================
// INJECTION METHODS
// =============================================================================

// SetQuestBoard injects the quest board reference for claiming party-required quests.
// Safe to call before or after Start — the reference is checked lazily when needed.
func (c *Component) SetQuestBoard(qb QuestBoardRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.questBoardRef = qb
}

// createGraphClient creates the graph client for the component.
// Context is unused: NewGraphClient is a synchronous in-memory constructor.
func (c *Component) createGraphClient(_ context.Context) error {
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	return nil
}
