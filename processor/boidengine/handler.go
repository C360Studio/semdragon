package boidengine

import (
	"context"
	"encoding/json"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// STATE MANAGEMENT
// =============================================================================

// loadInitialState loads all agents and quests from the graph.
func (c *Component) loadInitialState(ctx context.Context) error {
	// Load all agents from graph
	agentEntities, err := c.graph.ListAgentsByPrefix(ctx, 100)
	if err != nil {
		return err
	}
	c.agentsMu.Lock()
	for _, entity := range agentEntities {
		agent := semdragons.AgentFromEntityState(&entity)
		if agent != nil {
			instance := semdragons.ExtractInstance(string(agent.ID))
			c.agents[instance] = agent
		}
	}
	c.agentsMu.Unlock()

	// Load all quests from graph
	questEntities, err := c.graph.ListQuestsByPrefix(ctx, 100)
	if err != nil {
		return err
	}
	c.questsMu.Lock()
	for _, entity := range questEntities {
		quest := semdragons.QuestFromEntityState(&entity)
		if quest != nil && quest.Status == semdragons.QuestPosted {
			instance := semdragons.ExtractInstance(string(quest.ID))
			c.quests[instance] = quest
		}
	}
	c.questsMu.Unlock()

	c.logger.Debug("loaded initial state",
		"agents", len(agentEntities),
		"quests", len(c.quests))

	return nil
}

// startKVWatchers sets up watchers for agent and quest state changes.
func (c *Component) startKVWatchers(ctx context.Context) error {
	kv, err := c.graph.KVBucket(ctx)
	if err != nil {
		return err
	}

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
func (c *Component) UpdateRules(rules BoidRules) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = rules
	c.boidEngine.UpdateRules(rules)
	c.logger.Info("updated boid rules", "rules", rules)
}

// GetRules returns the current boid rules.
func (c *Component) GetRules() BoidRules {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rules
}

// ComputeAttractionsNow computes attractions immediately without waiting for the periodic loop.
func (c *Component) ComputeAttractionsNow() []QuestAttraction {
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
func (c *Component) SuggestClaimsNow() []SuggestedClaim {
	attractions := c.ComputeAttractionsNow()
	return c.boidEngine.SuggestClaims(attractions)
}

// Graph returns the underlying graph client for external access.
func (c *Component) Graph() *semdragons.GraphClient {
	return c.graph
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

// createGraphClient creates the graph client for the component.
func (c *Component) createGraphClient(_ context.Context) error {
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	return nil
}
