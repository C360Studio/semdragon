package boidengine

import (
	"context"
	"encoding/json"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
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
		agent := agentprogression.AgentFromEntityState(&entity)
		if agent != nil {
			instance := domain.ExtractInstance(string(agent.ID))
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
		quest := domain.QuestFromEntityState(&entity)
		if quest != nil && quest.Status == domain.QuestPosted && quest.PartyID == nil {
			instance := domain.ExtractInstance(string(quest.ID))
			c.quests[instance] = quest
		}
	}
	c.questsMu.Unlock()

	// Load all guilds from graph for rank/reputation lookups
	guildEntities, err := c.graph.ListGuildsByPrefix(ctx, 100)
	if err != nil {
		c.logger.Warn("failed to load guilds (affinity scoring degraded)", "error", err)
		// Non-fatal: boid engine works without guild context, just with binary matching
	} else {
		c.guildsMu.Lock()
		for _, entity := range guildEntities {
			guild := domain.GuildFromEntityState(&entity)
			if guild != nil {
				instance := domain.ExtractInstance(string(guild.ID))
				c.guilds[instance] = guild
			}
		}
		c.guildsMu.Unlock()
	}

	c.logger.Debug("loaded initial state",
		"agents", len(agentEntities),
		"quests", len(c.quests),
		"guilds", len(c.guilds))

	return nil
}

// startKVWatchers sets up watchers for agent and quest state changes.
func (c *Component) startKVWatchers(ctx context.Context) error {
	kv, err := c.graph.KVBucket(ctx)
	if err != nil {
		return err
	}

	// Watch agent state changes using entity ID prefix pattern
	// Keys in ENTITY_STATES follow: org.platform.game.board.type.instance
	agentPrefix := c.graph.Config().TypePrefix("agent") + ".>"
	agentWatcher, err := kv.Watch(ctx, agentPrefix)
	if err != nil {
		return err
	}
	c.agentWatch = agentWatcher

	// Watch quest state changes using entity ID prefix pattern
	questPrefix := c.graph.Config().TypePrefix("quest") + ".>"
	questWatcher, err := kv.Watch(ctx, questPrefix)
	if err != nil {
		c.agentWatch.Stop()
		return err
	}
	c.questWatch = questWatcher

	// Watch guild state changes for rank/reputation updates
	guildPrefix := c.graph.Config().TypePrefix("guild") + ".>"
	guildWatcher, err := kv.Watch(ctx, guildPrefix)
	if err != nil {
		c.logger.Warn("failed to start guild KV watcher (affinity scoring degraded)", "error", err)
		// Non-fatal: guild context from initial load still available
	} else {
		c.guildWatch = guildWatcher
	}

	// Start goroutine to process watch updates
	go c.processWatchUpdates()

	c.logger.Debug("started KV watchers for agent, quest, and guild state")
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
	if c.guildWatch != nil {
		c.guildWatch.Stop()
	}
}

// processWatchUpdates handles updates from KV watchers.
func (c *Component) processWatchUpdates() {
	defer close(c.watchDoneCh)

	// Guild watcher may be nil if setup failed (non-fatal)
	var guildUpdates <-chan jetstream.KeyValueEntry
	if c.guildWatch != nil {
		guildUpdates = c.guildWatch.Updates()
	}

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

		case entry, ok := <-guildUpdates:
			if !ok {
				guildUpdates = nil // Closed, stop selecting
				continue
			}
			if entry == nil {
				continue // Initial sync complete
			}
			c.handleGuildUpdate(entry)
		}
	}
}

// handleAgentUpdate processes an agent state change from KV.
// Keys in the ENTITY_STATES bucket use the full 6-part entity ID format:
// org.platform.game.board.agent.instance (e.g., test.integration.game.board1.agent.abc123)
func (c *Component) handleAgentUpdate(entry jetstream.KeyValueEntry) {
	key := entry.Key()
	instance := domain.ExtractInstance(key)
	if instance == "" || instance == key {
		// Key did not contain a dot separator — not a valid entity ID.
		c.logger.Warn("agent watch entry has unexpected key format", "key", key)
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.agentsMu.Lock()
		delete(c.agents, instance)
		c.agentsMu.Unlock()
		c.logger.Debug("agent removed from cache", "instance", instance)
		return
	}

	// Decode entity state and reconstruct the Agent from its triples.
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		c.logger.Warn("failed to decode agent entity state", "instance", instance, "error", err)
		return
	}
	agent := agentprogression.AgentFromEntityState(entityState)
	if agent == nil {
		c.logger.Warn("failed to reconstruct agent from entity state", "instance", instance)
		return
	}

	c.agentsMu.Lock()
	c.agents[instance] = agent
	c.agentsMu.Unlock()

	c.logger.Debug("agent cache updated", "instance", instance, "status", agent.Status, "guild", agent.Guild)
}

// handleQuestUpdate processes a quest state change from KV.
// Keys in the ENTITY_STATES bucket use the full 6-part entity ID format:
// org.platform.game.board.quest.instance (e.g., test.integration.game.board1.quest.abc123)
func (c *Component) handleQuestUpdate(entry jetstream.KeyValueEntry) {
	key := entry.Key()
	instance := domain.ExtractInstance(key)
	if instance == "" || instance == key {
		// Key did not contain a dot separator — not a valid entity ID.
		c.logger.Warn("quest watch entry has unexpected key format", "key", key)
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.questsMu.Lock()
		delete(c.quests, instance)
		c.questsMu.Unlock()
		c.logger.Debug("quest removed from cache", "instance", instance)
		return
	}

	// Decode entity state and reconstruct the Quest from its triples.
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		c.logger.Warn("failed to decode quest entity state", "instance", instance, "error", err)
		return
	}
	quest := domain.QuestFromEntityState(entityState)
	if quest == nil {
		c.logger.Warn("failed to reconstruct quest from entity state", "instance", instance)
		return
	}

	c.questsMu.Lock()
	// Only track posted quests without a party — party sub-quests are
	// managed by questdagexec and should not appear in boid suggestions.
	if quest.Status == domain.QuestPosted && quest.PartyID == nil {
		c.quests[instance] = quest
	} else {
		// Remove non-posted or party quests from cache.
		delete(c.quests, instance)
	}
	c.questsMu.Unlock()

	c.logger.Debug("quest cache updated", "instance", instance, "status", quest.Status)
}

// handleGuildUpdate processes a guild state change from KV.
func (c *Component) handleGuildUpdate(entry jetstream.KeyValueEntry) {
	key := entry.Key()
	instance := domain.ExtractInstance(key)
	if instance == "" || instance == key {
		c.logger.Warn("guild watch entry has unexpected key format", "key", key)
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.guildsMu.Lock()
		delete(c.guilds, instance)
		c.guildsMu.Unlock()
		c.logger.Debug("guild removed from cache", "instance", instance)
		return
	}

	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		c.logger.Warn("failed to decode guild entity state", "instance", instance, "error", err)
		return
	}
	guild := domain.GuildFromEntityState(entityState)
	if guild == nil {
		c.logger.Warn("failed to reconstruct guild from entity state", "instance", instance)
		return
	}

	c.guildsMu.Lock()
	c.guilds[instance] = guild
	c.guildsMu.Unlock()

	c.logger.Debug("guild cache updated", "instance", instance, "status", guild.Status, "members", len(guild.Members), "reputation", guild.Reputation)
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

	// When paused, skip computation. Timer still fires so we re-check next tick.
	if c.pauseChecker != nil && c.pauseChecker.Paused() {
		return
	}

	c.lastActivity.Store(time.Now())

	// Gather current state
	c.agentsMu.RLock()
	agents := make([]agentprogression.Agent, 0, len(c.agents))
	for _, agent := range c.agents {
		agents = append(agents, *agent)
	}
	c.agentsMu.RUnlock()

	c.questsMu.RLock()
	quests := make([]domain.Quest, 0, len(c.quests))
	for _, quest := range c.quests {
		// Only include posted (available) quests
		if quest.Status == domain.QuestPosted {
			quests = append(quests, *quest)
		}
	}
	c.questsMu.RUnlock()

	// Provide guild context for rank/reputation scoring
	c.guildsMu.RLock()
	guildCtx := make(map[domain.GuildID]*domain.Guild, len(c.guilds))
	for _, guild := range c.guilds {
		guildCtx[guild.ID] = guild
	}
	c.guildsMu.RUnlock()

	if engine, ok := c.boidEngine.(*DefaultBoidEngine); ok {
		engine.SetGuildContext(guildCtx)
		engine.SetCohesionData(c.resolveCohesion())
	}

	ctx := context.Background()

	// --- Quest attractions (only when quests are available) ---
	if len(agents) > 0 && len(quests) > 0 {
		c.computeAndPublishQuestSuggestions(ctx, agents, quests)
	}

	// --- Guild suggestions (runs even with no quests) ---
	if c.config.EnableGuildSuggestions && len(agents) > 0 {
		c.computeAndPublishGuildSuggestions(ctx, agents, guildCtx)
	}
}

// computeAndPublishQuestSuggestions computes quest attractions and publishes per-agent suggestions.
func (c *Component) computeAndPublishQuestSuggestions(ctx context.Context, agents []agentprogression.Agent, quests []domain.Quest) {
	attractions := c.boidEngine.ComputeAttractions(agents, quests, c.rules)
	if len(attractions) == 0 {
		return
	}

	maxSuggestions := c.config.MaxSuggestionsPerAgent
	if maxSuggestions <= 0 {
		maxSuggestions = 3
	}
	topN := c.boidEngine.SuggestTopN(attractions, maxSuggestions)
	if len(topN) == 0 {
		return
	}

	totalPublished := 0
	for agentID, suggestions := range topN {
		subject := "boid.suggestions." + domain.ExtractInstance(string(agentID))
		data, err := json.Marshal(suggestions)
		if err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to marshal suggestions", "agent", agentID, "error", err)
			continue
		}
		if err := c.deps.NATSClient.Publish(ctx, subject, data); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to publish suggestions", "agent", agentID, "error", err)
			continue
		}
		if c.suggestionsBucket != nil {
			agentInstance := domain.ExtractInstance(string(agentID))
			if _, putErr := c.suggestionsBucket.Put(ctx, agentInstance, data); putErr != nil {
				c.logger.Warn("failed to persist boid suggestion to KV", "agent", agentID, "error", putErr)
			}
		}
		totalPublished += len(suggestions)
	}

	c.suggestionsComputed.Add(uint64(totalPublished))

	c.logger.Debug("computed and published quest suggestions",
		"agents", len(agents),
		"quests", len(quests),
		"agents_with_suggestions", len(topN),
		"total_suggestions", totalPublished)
}

// computeAndPublishGuildSuggestions computes guild join/form suggestions and publishes per-agent.
func (c *Component) computeAndPublishGuildSuggestions(ctx context.Context, agents []agentprogression.Agent, guildCtx map[domain.GuildID]*domain.Guild) {
	engine, ok := c.boidEngine.(*DefaultBoidEngine)
	if !ok {
		return
	}

	guildSuggestions := engine.ComputeGuildSuggestions(
		agents, guildCtx,
		c.config.GuildFormMinUnguilded,
		c.config.GuildJoinThreshold,
	)
	if len(guildSuggestions) == 0 {
		return
	}

	published := 0
	for _, gs := range guildSuggestions {
		subject := "boid.guild." + domain.ExtractInstance(string(gs.AgentID))
		data, err := json.Marshal(gs)
		if err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to marshal guild suggestion", "agent", gs.AgentID, "error", err)
			continue
		}
		if err := c.deps.NATSClient.Publish(ctx, subject, data); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to publish guild suggestion", "agent", gs.AgentID, "error", err)
			continue
		}
		if c.suggestionsBucket != nil {
			agentInstance := domain.ExtractInstance(string(gs.AgentID))
			if _, putErr := c.suggestionsBucket.Put(ctx, "guild."+agentInstance, data); putErr != nil {
				c.logger.Warn("failed to persist guild suggestion to KV", "agent", gs.AgentID, "error", putErr)
			}
		}
		published++
	}

	c.logger.Debug("computed and published guild suggestions",
		"agents", len(agents),
		"guilds", len(guildCtx),
		"guild_suggestions", published)
}

// resolveCohesion lazily resolves the CohesionData from the component registry.
// Returns nil if guildformation is not registered (guild suggestions degrade gracefully).
func (c *Component) resolveCohesion() CohesionData {
	if c.deps.ComponentRegistry == nil {
		return nil
	}
	comp := c.deps.ComponentRegistry.Component("guildformation")
	if comp == nil {
		return nil
	}
	cd, ok := comp.(CohesionData)
	if !ok {
		return nil
	}
	return cd
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
	agents := make([]agentprogression.Agent, 0, len(c.agents))
	for _, agent := range c.agents {
		agents = append(agents, *agent)
	}
	c.agentsMu.RUnlock()

	c.questsMu.RLock()
	quests := make([]domain.Quest, 0, len(c.quests))
	for _, quest := range c.quests {
		if quest.Status == domain.QuestPosted {
			quests = append(quests, *quest)
		}
	}
	c.questsMu.RUnlock()

	// Provide guild context for rank/reputation scoring
	c.guildsMu.RLock()
	guildCtx := make(map[domain.GuildID]*domain.Guild, len(c.guilds))
	for _, guild := range c.guilds {
		guildCtx[guild.ID] = guild
	}
	c.guildsMu.RUnlock()

	if engine, ok := c.boidEngine.(*DefaultBoidEngine); ok {
		engine.SetGuildContext(guildCtx)
	}

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
// Context is unused: NewGraphClient is a synchronous in-memory constructor.
func (c *Component) createGraphClient(_ context.Context) error {
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	return nil
}
