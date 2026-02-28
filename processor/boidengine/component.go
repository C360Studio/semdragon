// Package boidengine provides a native semstreams component for emergent agent
// behavior coordination. It implements boid flocking rules to suggest quest claims
// based on agent skills, preferences, and current state.
package boidengine

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// COMPONENT - BoidEngine as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Watches agent and quest state via KV, computes attractions periodically,
// and publishes suggestion events.
// =============================================================================

// Config holds the component configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// Boid rule weights
	SeparationWeight float64 `json:"separation_weight" schema:"type:float,description:Avoid quest overlap weight"`
	AlignmentWeight  float64 `json:"alignment_weight" schema:"type:float,description:Align with peers weight"`
	CohesionWeight   float64 `json:"cohesion_weight" schema:"type:float,description:Move toward skill clusters weight"`
	HungerWeight     float64 `json:"hunger_weight" schema:"type:float,description:Idle time urgency weight"`
	AffinityWeight   float64 `json:"affinity_weight" schema:"type:float,description:Skill/guild match weight"`
	CautionWeight    float64 `json:"caution_weight" schema:"type:float,description:Avoid over-leveled quests weight"`

	// Timing
	UpdateIntervalMs int `json:"update_interval_ms" schema:"type:int,description:How often to recompute suggestions"`
	NeighborRadius   int `json:"neighbor_radius" schema:"type:int,description:How many nearby agents to consider"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	rules := semdragons.DefaultBoidRules()
	return Config{
		Org:              "default",
		Platform:         "local",
		Board:            "main",
		SeparationWeight: rules.SeparationWeight,
		AlignmentWeight:  rules.AlignmentWeight,
		CohesionWeight:   rules.CohesionWeight,
		HungerWeight:     rules.HungerWeight,
		AffinityWeight:   rules.AffinityWeight,
		CautionWeight:    rules.CautionWeight,
		UpdateIntervalMs: rules.UpdateInterval,
		NeighborRadius:   rules.NeighborRadius,
	}
}

// ToBoardConfig converts component config to semdragons BoardConfig.
func (c *Config) ToBoardConfig() *semdragons.BoardConfig {
	return &semdragons.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}

// ToBoidRules converts component config to semdragons BoidRules.
func (c *Config) ToBoidRules() semdragons.BoidRules {
	return semdragons.BoidRules{
		SeparationWeight: c.SeparationWeight,
		AlignmentWeight:  c.AlignmentWeight,
		CohesionWeight:   c.CohesionWeight,
		HungerWeight:     c.HungerWeight,
		AffinityWeight:   c.AffinityWeight,
		CautionWeight:    c.CautionWeight,
		NeighborRadius:   c.NeighborRadius,
		UpdateInterval:   c.UpdateIntervalMs,
	}
}

// Component implements the BoidEngine as a semstreams processor.
type Component struct {
	config      *Config
	deps        component.Dependencies
	storage     *semdragons.Storage
	boidEngine  semdragons.BoidEngine
	logger      *slog.Logger
	boardConfig *semdragons.BoardConfig
	rules       semdragons.BoidRules

	// KV watches for real-time state updates
	agentWatch jetstream.KeyWatcher
	questWatch jetstream.KeyWatcher

	// Cached state
	agents   map[string]*semdragons.Agent
	quests   map[string]*semdragons.Quest
	agentsMu sync.RWMutex
	questsMu sync.RWMutex

	// Internal state
	running      atomic.Bool
	mu           sync.RWMutex
	stopChan     chan struct{}
	doneChan     chan struct{}
	watchDoneCh  chan struct{} // Signals watch goroutines are done
	stopOnce     sync.Once

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

	c.boardConfig = c.config.ToBoardConfig()
	c.rules = c.config.ToBoidRules()
	c.boidEngine = semdragons.NewDefaultBoidEngine()
	c.agents = make(map[string]*semdragons.Agent)
	c.quests = make(map[string]*semdragons.Quest)
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

	// Create storage (KV bucket)
	storage, err := semdragons.CreateStorage(ctx, c.deps.NATSClient, c.boardConfig)
	if err != nil {
		return errs.Wrap(err, "BoidEngine", "Start", "create storage")
	}
	c.storage = storage

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

// =============================================================================
// STATE MANAGEMENT
// =============================================================================

// loadInitialState loads all agents and quests from KV.
func (c *Component) loadInitialState(ctx context.Context) error {
	// Load all agents
	agents, err := c.storage.ListAllAgents(ctx)
	if err != nil {
		return err
	}
	c.agentsMu.Lock()
	for _, agent := range agents {
		instance := semdragons.ExtractInstance(string(agent.ID))
		c.agents[instance] = agent
	}
	c.agentsMu.Unlock()

	// Load all posted quests (returns quest instances, not Quest structs)
	questInstances, err := c.storage.ListQuestsByStatus(ctx, semdragons.QuestPosted)
	if err != nil {
		return err
	}
	c.questsMu.Lock()
	for _, instance := range questInstances {
		quest, err := c.storage.GetQuest(ctx, instance)
		if err != nil {
			c.logger.Warn("failed to load quest", "instance", instance, "error", err)
			continue
		}
		c.quests[instance] = quest
	}
	c.questsMu.Unlock()

	c.logger.Debug("loaded initial state",
		"agents", len(agents),
		"quests", len(questInstances))

	return nil
}

// startKVWatchers sets up watchers for agent and quest state changes.
func (c *Component) startKVWatchers(ctx context.Context) error {
	kv := c.storage.KV()

	// Watch agent state changes
	agentWatcher, err := kv.Watch(ctx, "agent.state.>")
	if err != nil {
		return err
	}
	c.agentWatch = agentWatcher

	// Watch quest state changes
	questWatcher, err := kv.Watch(ctx, "quest.state.>")
	if err != nil {
		c.agentWatch.Stop()
		return err
	}
	c.questWatch = questWatcher

	// Start goroutine to process watch updates
	go c.processWatchUpdates()

	c.logger.Debug("started KV watchers for agent and quest state")
	return nil
}

// stopKVWatchers stops the KV watchers.
func (c *Component) stopKVWatchers() {
	if c.agentWatch != nil {
		c.agentWatch.Stop()
	}
	if c.questWatch != nil {
		c.questWatch.Stop()
	}
}

// processWatchUpdates handles updates from KV watchers.
func (c *Component) processWatchUpdates() {
	defer close(c.watchDoneCh)

	for {
		select {
		case <-c.stopChan:
			return

		case entry, ok := <-c.agentWatch.Updates():
			if !ok {
				return
			}
			if entry == nil {
				continue // Initial sync complete
			}
			c.handleAgentUpdate(entry)

		case entry, ok := <-c.questWatch.Updates():
			if !ok {
				return
			}
			if entry == nil {
				continue // Initial sync complete
			}
			c.handleQuestUpdate(entry)
		}
	}
}

// handleAgentUpdate processes an agent state change from KV.
func (c *Component) handleAgentUpdate(entry jetstream.KeyValueEntry) {
	// Extract instance from key (format: agent.state.{instance})
	key := entry.Key()
	if len(key) <= len("agent.state.") {
		return
	}
	instance := key[len("agent.state."):]

	if entry.Operation() == jetstream.KeyValueDelete {
		c.agentsMu.Lock()
		delete(c.agents, instance)
		c.agentsMu.Unlock()
		c.logger.Debug("agent removed from cache", "instance", instance)
		return
	}

	// Parse agent data
	var agent semdragons.Agent
	if err := json.Unmarshal(entry.Value(), &agent); err != nil {
		c.logger.Warn("failed to unmarshal agent update", "instance", instance, "error", err)
		return
	}

	c.agentsMu.Lock()
	c.agents[instance] = &agent
	c.agentsMu.Unlock()

	c.logger.Debug("agent cache updated", "instance", instance, "status", agent.Status)
}

// handleQuestUpdate processes a quest state change from KV.
func (c *Component) handleQuestUpdate(entry jetstream.KeyValueEntry) {
	// Extract instance from key (format: quest.state.{instance})
	key := entry.Key()
	if len(key) <= len("quest.state.") {
		return
	}
	instance := key[len("quest.state."):]

	if entry.Operation() == jetstream.KeyValueDelete {
		c.questsMu.Lock()
		delete(c.quests, instance)
		c.questsMu.Unlock()
		c.logger.Debug("quest removed from cache", "instance", instance)
		return
	}

	// Parse quest data
	var quest semdragons.Quest
	if err := json.Unmarshal(entry.Value(), &quest); err != nil {
		c.logger.Warn("failed to unmarshal quest update", "instance", instance, "error", err)
		return
	}

	c.questsMu.Lock()
	// Only track posted quests for boid calculations
	if quest.Status == semdragons.QuestPosted {
		c.quests[instance] = &quest
	} else {
		// Remove non-posted quests from cache
		delete(c.quests, instance)
	}
	c.questsMu.Unlock()

	c.logger.Debug("quest cache updated", "instance", instance, "status", quest.Status)
}

// =============================================================================
// COMPUTATION LOOP
// =============================================================================

// runComputeLoop periodically computes attractions and publishes suggestions.
func (c *Component) runComputeLoop() {
	defer close(c.doneChan)

	interval := time.Duration(c.config.UpdateIntervalMs) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.computeAndPublish()
		}
	}
}

// computeAndPublish computes attractions and publishes suggestions.
func (c *Component) computeAndPublish() {
	if !c.running.Load() {
		return
	}

	c.lastActivity.Store(time.Now())

	// Gather current state
	c.agentsMu.RLock()
	agents := make([]semdragons.Agent, 0, len(c.agents))
	for _, agent := range c.agents {
		agents = append(agents, *agent)
	}
	c.agentsMu.RUnlock()

	c.questsMu.RLock()
	quests := make([]semdragons.Quest, 0, len(c.quests))
	for _, quest := range c.quests {
		// Only include posted (available) quests
		if quest.Status == semdragons.QuestPosted {
			quests = append(quests, *quest)
		}
	}
	c.questsMu.RUnlock()

	if len(agents) == 0 || len(quests) == 0 {
		return
	}

	// Compute attractions
	attractions := c.boidEngine.ComputeAttractions(agents, quests, c.rules)
	if len(attractions) == 0 {
		return
	}

	// Get suggested claims
	suggestions := c.boidEngine.SuggestClaims(attractions)
	if len(suggestions) == 0 {
		return
	}

	c.suggestionsComputed.Add(uint64(len(suggestions)))

	// Publish suggestions (fire and forget for now)
	ctx := context.Background()
	for _, suggestion := range suggestions {
		subject := "boid.suggestions." + semdragons.ExtractInstance(string(suggestion.AgentID))
		data, err := json.Marshal(suggestion)
		if err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to marshal suggestion", "error", err)
			continue
		}
		if err := c.deps.NATSClient.Publish(ctx, subject, data); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to publish suggestion",
				"agent", suggestion.AgentID,
				"quest", suggestion.QuestID,
				"error", err)
		}
	}

	c.logger.Debug("computed and published suggestions",
		"agents", len(agents),
		"quests", len(quests),
		"suggestions", len(suggestions))
}

// =============================================================================
// PUBLIC API
// =============================================================================

// UpdateRules updates the boid rules at runtime.
func (c *Component) UpdateRules(rules semdragons.BoidRules) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = rules
	c.boidEngine.UpdateRules(rules)
	c.logger.Info("updated boid rules", "rules", rules)
}

// GetRules returns the current boid rules.
func (c *Component) GetRules() semdragons.BoidRules {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rules
}

// ComputeAttractionsNow computes attractions immediately without waiting for the periodic loop.
func (c *Component) ComputeAttractionsNow() []semdragons.QuestAttraction {
	c.agentsMu.RLock()
	agents := make([]semdragons.Agent, 0, len(c.agents))
	for _, agent := range c.agents {
		agents = append(agents, *agent)
	}
	c.agentsMu.RUnlock()

	c.questsMu.RLock()
	quests := make([]semdragons.Quest, 0, len(c.quests))
	for _, quest := range c.quests {
		if quest.Status == semdragons.QuestPosted {
			quests = append(quests, *quest)
		}
	}
	c.questsMu.RUnlock()

	return c.boidEngine.ComputeAttractions(agents, quests, c.rules)
}

// SuggestClaimsNow suggests claims immediately.
func (c *Component) SuggestClaimsNow() []semdragons.SuggestedClaim {
	attractions := c.ComputeAttractionsNow()
	return c.boidEngine.SuggestClaims(attractions)
}

// Storage returns the underlying storage for external access.
func (c *Component) Storage() *semdragons.Storage {
	return c.storage
}

// Stats returns boid engine statistics.
func (c *Component) Stats() BoidStats {
	c.agentsMu.RLock()
	agentCount := len(c.agents)
	c.agentsMu.RUnlock()

	c.questsMu.RLock()
	questCount := len(c.quests)
	c.questsMu.RUnlock()

	return BoidStats{
		AgentsTracked:       agentCount,
		QuestsTracked:       questCount,
		SuggestionsComputed: c.suggestionsComputed.Load(),
		Errors:              c.errorsCount.Load(),
	}
}

// BoidStats holds boid engine statistics.
type BoidStats struct {
	AgentsTracked       int    `json:"agents_tracked"`
	QuestsTracked       int    `json:"quests_tracked"`
	SuggestionsComputed uint64 `json:"suggestions_computed"`
	Errors              int64  `json:"errors"`
}
