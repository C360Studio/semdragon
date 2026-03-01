package autonomy

import (
	"context"
	"encoding/json"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// KV WATCH HANDLER - Agent state monitoring
// =============================================================================
// Watches the ENTITY_STATES KV bucket for agent changes and creates/updates
// per-agent heartbeat trackers. Follows the same pattern as agentstore handler.
// =============================================================================

// processAgentWatchUpdates handles agent entity state changes from KV.
func (c *Component) processAgentWatchUpdates() {
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
		}
	}
}

// handleAgentUpdate processes an agent entity state change from KV.
func (c *Component) handleAgentUpdate(entry jetstream.KeyValueEntry) {
	if !c.running.Load() {
		return
	}

	key := entry.Key()
	instance := semdragons.ExtractInstance(key)
	if instance == "" || instance == key {
		c.logger.Warn("agent watch entry has unexpected key format", "key", key)
		return
	}

	// Handle deletion — cancel heartbeat, remove tracker
	if entry.Operation() == jetstream.KeyValueDelete {
		c.cancelHeartbeat(instance)
		c.logger.Debug("agent removed, heartbeat cancelled", "instance", instance)
		return
	}

	// Decode entity state and reconstruct the Agent
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		c.logger.Warn("failed to decode agent entity state", "instance", instance, "error", err)
		return
	}

	agent := semdragons.AgentFromEntityState(entityState)
	if agent == nil {
		c.logger.Warn("failed to reconstruct agent from entity state", "instance", instance)
		return
	}

	// Create or update heartbeat tracker for this agent
	c.resetHeartbeatForAgent(instance, agent)

	c.logger.Debug("agent heartbeat updated",
		"instance", instance,
		"status", agent.Status,
		"interval", c.config.IntervalForStatus(agent.Status))
}

// =============================================================================
// BOID SUGGESTION HANDLER - NATS pub/sub subscription
// =============================================================================
// Subscribes to boid.suggestions.> to cache the latest suggestion per agent.
// When a new suggestion arrives for an idle agent, the backoff is reset so the
// next heartbeat fires sooner.
// =============================================================================

// handleBoidSuggestion processes a boid engine suggestion from NATS.
func (c *Component) handleBoidSuggestion(ctx context.Context, msg *nats.Msg) {
	if ctx.Err() != nil {
		return
	}
	if !c.running.Load() {
		return
	}

	var suggestion boidengine.SuggestedClaim
	if err := json.Unmarshal(msg.Data, &suggestion); err != nil {
		c.errorsCount.Add(1)
		c.logger.Warn("failed to unmarshal boid suggestion", "error", err)
		return
	}

	// Extract instance from the agent ID in the suggestion
	instance := semdragons.ExtractInstance(string(suggestion.AgentID))
	if instance == "" {
		c.logger.Warn("boid suggestion has invalid agent ID", "agent_id", suggestion.AgentID)
		return
	}

	c.trackersMu.Lock()
	defer c.trackersMu.Unlock()

	tracker, exists := c.trackers[instance]
	if !exists {
		// Agent not yet tracked — suggestion will be picked up when agent appears
		return
	}

	// Only cache suggestions for idle agents
	if tracker.agent != nil && tracker.agent.Status == semdragons.AgentIdle {
		tracker.suggestion = &suggestion
		c.resetHeartbeatInterval(instance)
		c.logger.Debug("boid suggestion cached for agent",
			"instance", instance,
			"quest_id", suggestion.QuestID,
			"score", suggestion.Score)
	}
}
