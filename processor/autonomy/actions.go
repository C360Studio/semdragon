package autonomy

import (
	"context"
	"errors"
	"time"

	semdragons "github.com/c360studio/semdragons"
)

// errNoViableClaim is returned by executeClaimQuest when all suggestions were
// exhausted without claiming anything. The evaluator uses this to distinguish
// "action attempted but nothing viable" from a real failure, so it can enter
// the backoff path instead of treating it as a successful action.
var errNoViableClaim = errors.New("no viable quest to claim")

// =============================================================================
// AUTONOMOUS ACTIONS
// =============================================================================
// Each method returns an action struct with shouldExecute and execute closures
// that capture the component's graph client for KV reads/writes.
// claimQuestAction is fully implemented (Phase 2); others remain stubs.
// =============================================================================

// action represents an autonomous action an agent can take.
type action struct {
	name          string
	shouldExecute func(*semdragons.Agent, *agentTracker) bool
	execute       func(context.Context, *semdragons.Agent, *agentTracker) error
}

// claimQuestAction returns the action for claiming quests from boid suggestions.
// shouldExecute is true when agent is idle and has ranked suggestions cached.
// execute iterates suggestions best-first, claiming the first viable quest.
func (c *Component) claimQuestAction() action {
	return action{
		name: "claim_quest",
		shouldExecute: func(agent *semdragons.Agent, tracker *agentTracker) bool {
			return agent.Status == semdragons.AgentIdle && len(tracker.suggestions) > 0
		},
		execute: func(ctx context.Context, agent *semdragons.Agent, tracker *agentTracker) error {
			return c.executeClaimQuest(ctx, agent, tracker)
		},
	}
}

// executeClaimQuest iterates ranked suggestions and claims the first viable quest.
// If a quest is stale (no longer posted) or fails validation, it falls through
// to the next suggestion. KV write serialization handles concurrent claims.
func (c *Component) executeClaimQuest(ctx context.Context, agent *semdragons.Agent, tracker *agentTracker) error {
	for _, suggestion := range tracker.suggestions {
		// Read quest from KV
		entity, err := c.graph.GetQuest(ctx, semdragons.QuestID(suggestion.QuestID))
		if err != nil {
			c.logger.Debug("quest not found in KV, skipping suggestion",
				"quest_id", suggestion.QuestID,
				"error", err)
			continue
		}

		quest := semdragons.QuestFromEntityState(entity)
		if quest == nil {
			continue
		}

		// Only claim posted quests — if another agent claimed first, skip
		if quest.Status != semdragons.QuestPosted {
			c.logger.Debug("quest no longer posted, skipping",
				"quest_id", suggestion.QuestID,
				"status", quest.Status)
			continue
		}

		// Pre-flight validation
		if err := semdragons.ValidateAgentCanClaim(agent, quest); err != nil {
			c.logger.Debug("agent cannot claim quest, trying next",
				"quest_id", suggestion.QuestID,
				"reason", err)
			continue
		}

		// Write quest state: claimed
		now := time.Now()
		agentID := semdragons.AgentID(agent.ID)
		quest.Status = semdragons.QuestClaimed
		quest.ClaimedBy = &agentID
		quest.ClaimedAt = &now
		quest.Attempts++

		if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.claimed"); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to write quest claim",
				"quest_id", suggestion.QuestID,
				"error", err)
			continue
		}

		// Write agent state: on_quest
		now2 := time.Now()
		questIDRef := semdragons.QuestID(quest.ID)
		agent.Status = semdragons.AgentOnQuest
		agent.CurrentQuest = &questIDRef
		agent.UpdatedAt = now2

		if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.status.on_quest"); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to update agent status on claim",
				"agent_id", agent.ID,
				"error", err)
			// Quest is already claimed — don't roll back, just log
		}

		c.logger.Info("agent autonomously claimed quest",
			"agent_id", agent.ID,
			"quest_id", suggestion.QuestID,
			"score", suggestion.Score)

		// Clear cached suggestions eagerly so a second heartbeat firing before
		// the KV watch update arrives doesn't attempt another claim.
		c.trackersMu.Lock()
		instance := semdragons.ExtractInstance(string(agent.ID))
		if t, ok := c.trackers[instance]; ok {
			t.suggestions = nil
		}
		c.trackersMu.Unlock()

		return nil
	}

	// All suggestions exhausted — signal the evaluator to enter backoff
	return errNoViableClaim
}

// claimConcurrentAction returns a stub for claiming concurrent quests.
// TODO: Implement in Phase 3 — claim additional quests while on_quest.
func (c *Component) claimConcurrentAction() action {
	return action{
		name:          "claim_concurrent",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// shopAction returns a stub for browsing/purchasing from the store.
// TODO: Implement in Phase 4 — browse affordable items, decide to buy.
func (c *Component) shopAction() action {
	return action{
		name:          "shop",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// shopStrategicAction returns a stub for strategic purchases while on quest.
// TODO: Implement in Phase 4 — mid-quest tool/consumable acquisition.
func (c *Component) shopStrategicAction() action {
	return action{
		name:          "shop_strategic",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// useConsumableAction returns a stub for using consumables.
// TODO: Implement in Phase 4 — activate consumable at optimal moment.
func (c *Component) useConsumableAction() action {
	return action{
		name:          "use_consumable",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// useCooldownSkipAction returns a stub for using cooldown_skip consumable.
// TODO: Implement in Phase 4 — skip cooldown if agent owns the consumable.
func (c *Component) useCooldownSkipAction() action {
	return action{
		name:          "use_cooldown_skip",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}

// joinGuildAction returns a stub for joining a guild.
// TODO: Implement in Phase 5 — evaluate guild suggestions, auto-join.
func (c *Component) joinGuildAction() action {
	return action{
		name:          "join_guild",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}
