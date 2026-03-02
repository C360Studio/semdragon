package autonomy

import (
	"context"
	"errors"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/agentstore"
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
// Both shouldExecute and execute receive the agentTracker to access
// suggestions and other tracker state. Actions that don't need tracker
// data may ignore it (the signature is uniform for simplicity).
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

// shopBudget returns the XP budget for autonomous shopping based on agent status.
// Returns 0 if the agent should not shop.
func (c *Component) shopBudget(agent *semdragons.Agent) int64 {
	switch agent.Status {
	case semdragons.AgentIdle:
		surplus := agent.XP - agent.XPToLevel
		if surplus < c.config.MinXPSurplusForShopping {
			return 0
		}
		return int64(float64(surplus) * c.config.MaxShopSpendRatio)
	case semdragons.AgentCooldown:
		if agent.XP < c.config.CooldownShopMinXP {
			return 0
		}
		return agent.XP
	default:
		return 0
	}
}

// shopAction returns the action for idle/cooldown browsing and purchasing.
// Idle agents spend a fraction of surplus XP (above XPToLevel); cooldown agents
// spend more liberally since they have nothing else to do.
func (c *Component) shopAction() action {
	return action{
		name: "shop",
		shouldExecute: func(agent *semdragons.Agent, _ *agentTracker) bool {
			if c.store == nil {
				return false
			}
			return c.shopBudget(agent) > 0
		},
		execute: func(ctx context.Context, agent *semdragons.Agent, _ *agentTracker) error {
			return c.executeShop(ctx, agent)
		},
	}
}

// executeShop computes budget, lists affordable items, and purchases the best one.
func (c *Component) executeShop(ctx context.Context, agent *semdragons.Agent) error {
	budget := c.shopBudget(agent)
	if budget <= 0 {
		return nil
	}

	items := c.store.ListItems(agent.Tier)
	item := pickBestItem(agent, items, budget)
	if item == nil {
		return nil
	}

	_, err := c.store.Purchase(ctx, agent.ID, item.ID, agent.XP, agent.Level, agent.Guilds)
	if err != nil {
		c.logger.Warn("autonomous shop purchase failed",
			"agent_id", agent.ID,
			"item_id", item.ID,
			"error", err)
		return err
	}

	c.logger.Info("agent autonomously purchased item",
		"agent_id", agent.ID,
		"item_id", item.ID,
		"item_name", item.Name,
		"cost", item.XPCost)
	return nil
}

// shopStrategicAction returns the action for targeted on-quest consumable acquisition.
// Only buys quality_shield or xp_boost — items that help the current quest.
// The affordability check is deferred to executeShopStrategic to keep this guard cheap.
func (c *Component) shopStrategicAction() action {
	return action{
		name: "shop_strategic",
		shouldExecute: func(agent *semdragons.Agent, _ *agentTracker) bool {
			if c.store == nil || agent.Status != semdragons.AgentOnQuest {
				return false
			}
			needsShield := !hasConsumable(agent, string(agentstore.ConsumableQualityShield))
			needsBoost := !hasConsumable(agent, string(agentstore.ConsumableXPBoost))
			if !needsShield && !needsBoost {
				return false
			}
			return agent.XP > 0
		},
		execute: func(ctx context.Context, agent *semdragons.Agent, _ *agentTracker) error {
			return c.executeShopStrategic(ctx, agent)
		},
	}
}

// executeShopStrategic buys quality_shield first (protects review), then xp_boost.
// Cap at one purchase per heartbeat.
func (c *Component) executeShopStrategic(ctx context.Context, agent *semdragons.Agent) error {
	items := c.store.ListItems(agent.Tier)

	// Priority: quality_shield > xp_boost
	priorities := []agentstore.ConsumableType{
		agentstore.ConsumableQualityShield,
		agentstore.ConsumableXPBoost,
	}

	for _, wanted := range priorities {
		if hasConsumable(agent, string(wanted)) {
			continue
		}
		for i := range items {
			item := &items[i]
			if item.ItemType != agentstore.ItemTypeConsumable || item.Effect == nil {
				continue
			}
			if item.Effect.Type != wanted {
				continue
			}
			if item.XPCost > c.config.StrategicShopMaxCost || item.XPCost > agent.XP {
				continue
			}
			_, err := c.store.Purchase(ctx, agent.ID, item.ID, agent.XP, agent.Level, agent.Guilds)
			if err != nil {
				c.logger.Warn("autonomous strategic purchase failed",
					"agent_id", agent.ID,
					"item_id", item.ID,
					"error", err)
				return err
			}
			c.logger.Info("agent strategically purchased consumable",
				"agent_id", agent.ID,
				"item_id", item.ID,
				"item_name", item.Name)
			return nil
		}
	}
	return nil
}

// consumableForStatus returns the consumable ID to use for a given agent status,
// or empty string if no consumable applies.
func consumableForStatus(status semdragons.AgentStatus) string {
	switch status {
	case semdragons.AgentIdle, semdragons.AgentOnQuest:
		return string(agentstore.ConsumableXPBoost)
	case semdragons.AgentInBattle:
		return string(agentstore.ConsumableQualityShield)
	default:
		return ""
	}
}

// useConsumableAction returns the action for activating consumables at optimal moments.
// State-dependent: idle + xp_boost before claiming, on_quest + xp_boost,
// in_battle + quality_shield.
func (c *Component) useConsumableAction() action {
	return action{
		name: "use_consumable",
		shouldExecute: func(agent *semdragons.Agent, tracker *agentTracker) bool {
			if c.store == nil {
				return false
			}
			switch agent.Status {
			case semdragons.AgentIdle:
				// Use xp_boost when about to claim (has suggestions)
				return hasConsumable(agent, string(agentstore.ConsumableXPBoost)) &&
					len(tracker.suggestions) > 0 &&
					!hasActiveEffect(agent, string(agentstore.ConsumableXPBoost))
			case semdragons.AgentOnQuest:
				return hasConsumable(agent, string(agentstore.ConsumableXPBoost)) &&
					!hasActiveEffect(agent, string(agentstore.ConsumableXPBoost))
			case semdragons.AgentInBattle:
				return hasConsumable(agent, string(agentstore.ConsumableQualityShield)) &&
					!hasActiveEffect(agent, string(agentstore.ConsumableQualityShield))
			default:
				return false
			}
		},
		execute: func(ctx context.Context, agent *semdragons.Agent, _ *agentTracker) error {
			consumableID := consumableForStatus(agent.Status)
			if consumableID == "" {
				return nil
			}
			if err := c.store.UseConsumable(ctx, agent.ID, consumableID, agent.CurrentQuest); err != nil {
				c.logger.Warn("autonomous consumable use failed",
					"agent_id", agent.ID,
					"consumable_id", consumableID,
					"error", err)
				return err
			}
			c.logger.Info("agent autonomously used consumable",
				"agent_id", agent.ID,
				"consumable_id", consumableID)
			return nil
		},
	}
}

// useCooldownSkipAction returns the action for using cooldown_skip consumable.
// Only fires when remaining cooldown exceeds the threshold — avoids wasting a
// consumable when the cooldown is nearly over.
func (c *Component) useCooldownSkipAction() action {
	return action{
		name: "use_cooldown_skip",
		shouldExecute: func(agent *semdragons.Agent, _ *agentTracker) bool {
			if c.store == nil || agent.Status != semdragons.AgentCooldown {
				return false
			}
			if !hasConsumable(agent, string(agentstore.ConsumableCooldownSkip)) {
				return false
			}
			if agent.CooldownUntil == nil {
				return false
			}
			remaining := time.Until(*agent.CooldownUntil)
			return remaining > c.config.CooldownSkipMinRemaining()
		},
		execute: func(ctx context.Context, agent *semdragons.Agent, _ *agentTracker) error {
			if err := c.store.UseConsumable(ctx, agent.ID, string(agentstore.ConsumableCooldownSkip), nil); err != nil {
				c.logger.Warn("autonomous cooldown skip failed",
					"agent_id", agent.ID,
					"error", err)
				return err
			}
			c.logger.Info("agent autonomously skipped cooldown",
				"agent_id", agent.ID)
			return nil
		},
	}
}

// =============================================================================
// SHOPPING HELPERS
// =============================================================================
// NOTE: This code assumes store item IDs match ConsumableType string values
// (e.g., item ID "quality_shield" == ConsumableQualityShield). If the catalog
// introduces items with IDs differing from their ConsumableType, the hasConsumable
// and hasActiveEffect lookups must be updated to scan by effect type instead.
// =============================================================================

// hasConsumable returns true if the agent owns at least one of the given consumable.
func hasConsumable(agent *semdragons.Agent, consumableID string) bool {
	if agent.Consumables == nil {
		return false
	}
	return agent.Consumables[consumableID] > 0
}

// hasActiveEffect returns true if the agent has an active effect of the given type
// with quests remaining.
func hasActiveEffect(agent *semdragons.Agent, effectType string) bool {
	for _, eff := range agent.ActiveEffects {
		if eff.EffectType == effectType && eff.QuestsRemaining > 0 {
			return true
		}
	}
	return false
}

// maxConsumableStock is the maximum number of any single consumable an agent should hoard.
const maxConsumableStock = 2

// pickBestItem selects the best item to purchase within budget.
// Priority: tools over consumables, most expensive tool first, skip owned tools,
// cap consumable stock at maxConsumableStock per type.
func pickBestItem(agent *semdragons.Agent, items []agentstore.StoreItem, budget int64) *agentstore.StoreItem {
	var bestTool *agentstore.StoreItem
	var bestConsumable *agentstore.StoreItem

	for i := range items {
		item := &items[i]
		if item.XPCost > budget {
			continue
		}

		switch item.ItemType {
		case agentstore.ItemTypeTool:
			// Skip tools the agent already owns
			if agent.OwnedTools != nil {
				if _, owned := agent.OwnedTools[item.ID]; owned {
					continue
				}
			}
			if bestTool == nil || item.XPCost > bestTool.XPCost {
				bestTool = item
			}

		case agentstore.ItemTypeConsumable:
			// Cap consumable stock
			if agent.Consumables != nil && agent.Consumables[item.ID] >= maxConsumableStock {
				continue
			}
			if bestConsumable == nil || item.XPCost > bestConsumable.XPCost {
				bestConsumable = item
			}
		}
	}

	// Prefer tools (permanent capability) over consumables
	if bestTool != nil {
		return bestTool
	}
	return bestConsumable
}

// joinGuildAction returns a stub for joining a guild.
// TODO: Implement in Phase 4 — evaluate guild suggestions, auto-join.
func (c *Component) joinGuildAction() action {
	return action{
		name:          "join_guild",
		shouldExecute: func(*semdragons.Agent, *agentTracker) bool { return false },
		execute:       nil,
	}
}
