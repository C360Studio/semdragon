package autonomy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/guildformation"
)

// errNoViableClaim is returned by executeClaimQuest when all suggestions were
// exhausted without claiming anything. The evaluator uses this to distinguish
// "action attempted but nothing viable" from a real failure, so it can enter
// the backoff path instead of treating it as a successful action.
var errNoViableClaim = errors.New("no viable quest to claim")

// =============================================================================
// APPROVAL GATE
// =============================================================================
// requestApproval blocks until the DM approves or denies an autonomous action.
// Auto-approves when: approval component is nil, DMMode is FullAuto or Assisted,
// or SessionID is empty. Returns true if the action should proceed.
// =============================================================================

func (c *Component) requestApproval(ctx context.Context, approvalType domain.ApprovalType, title, details string, payload any) bool {
	if c.approval == nil || c.config.DMMode == domain.DMFullAuto || c.config.DMMode == domain.DMAssisted {
		return true
	}
	if c.config.SessionID == "" {
		return true
	}

	// Use the caller's context. In supervised/manual mode, evaluateAutonomy
	// extends the evaluation timeout to ApprovalTimeout+10s, providing
	// sufficient headroom for both the approval wait and subsequent I/O.
	resp, err := c.approval.RequestApproval(ctx, domain.ApprovalRequest{
		SessionID: c.config.SessionID,
		Type:      approvalType,
		Title:     title,
		Details:   details,
		Payload:   payload,
		Options: []domain.ApprovalOption{
			{ID: "approve", Label: "Approve", IsDefault: true},
			{ID: "deny", Label: "Deny"},
		},
	})
	if err != nil {
		c.logger.Warn("approval request failed, denying action", "type", approvalType, "error", err)
		return false
	}
	return resp.Approved
}

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
	shouldExecute func(*agentprogression.Agent, *agentTracker) bool
	execute       func(context.Context, *agentprogression.Agent, *agentTracker) error
}

// claimQuestAction returns the action for claiming quests from boid suggestions.
// shouldExecute is true when agent is idle and has ranked suggestions cached.
// execute iterates suggestions best-first, claiming the first viable quest.
func (c *Component) claimQuestAction() action {
	return action{
		name: "claim_quest",
		shouldExecute: func(agent *agentprogression.Agent, tracker *agentTracker) bool {
			return agent.Status == domain.AgentIdle && len(tracker.suggestions) > 0
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, tracker *agentTracker) error {
			return c.executeClaimQuest(ctx, agent, tracker)
		},
	}
}

// executeClaimQuest iterates ranked suggestions and claims the first viable quest.
// If a quest is stale (no longer posted) or fails validation, it falls through
// to the next suggestion. KV write serialization handles concurrent claims.
func (c *Component) executeClaimQuest(ctx context.Context, agent *agentprogression.Agent, tracker *agentTracker) error {
	for i, suggestion := range tracker.suggestions {
		// Read quest from KV
		entity, err := c.graph.GetQuest(ctx, domain.QuestID(suggestion.QuestID))
		if err != nil {
			c.logger.Debug("quest not found in KV, skipping suggestion",
				"quest_id", suggestion.QuestID,
				"error", err)
			continue
		}

		quest := domain.QuestFromEntityState(entity)
		if quest == nil {
			continue
		}

		// Only claim posted quests — if another agent claimed first, skip
		if quest.Status != domain.QuestPosted {
			c.logger.Debug("quest no longer posted, skipping",
				"quest_id", suggestion.QuestID,
				"status", quest.Status)
			continue
		}

		// Pre-flight validation
		if err := agentprogression.ValidateAgentCanClaim(agent, quest); err != nil {
			c.logger.Debug("agent cannot claim quest, trying next",
				"quest_id", suggestion.QuestID,
				"reason", err)
			continue
		}

		// Approval gate: block in supervised/manual mode.
		// On denial, stop trying — a denial means "do not claim right now,"
		// not "show me the next option." Prevents repeated DM prompts.
		if !c.requestApproval(ctx, domain.ApprovalAutonomyClaim,
			fmt.Sprintf("Agent %s wants to claim quest %s", agent.Name, quest.Title),
			fmt.Sprintf("Quest: %s (difficulty: %d, XP: %d)", quest.Title, quest.Difficulty, quest.BaseXP),
			map[string]any{"agent_id": agent.ID, "quest_id": suggestion.QuestID, "score": suggestion.Score},
		) {
			c.logger.Info("claim denied by DM", "agent_id", agent.ID, "quest_id", suggestion.QuestID)
			return errNoViableClaim
		}

		// Write quest state: claimed
		now := time.Now()
		agentID := domain.AgentID(agent.ID)
		quest.Status = domain.QuestClaimed
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
		questIDRef := domain.QuestID(quest.ID)
		agent.Status = domain.AgentOnQuest
		agent.CurrentQuest = &questIDRef
		agent.UpdatedAt = now2

		if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.status.on_quest"); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to update agent status on claim",
				"agent_id", agent.ID,
				"error", err)
			// Quest is already claimed — don't roll back, just log
		}

		// Emit claim intent for observability
		if err := SubjectAutonomyClaimIntent.Publish(ctx, c.deps.NATSClient, ClaimIntentPayload{
			AgentID:        domain.AgentID(agent.ID),
			QuestID:        suggestion.QuestID,
			Score:          suggestion.Score,
			SuggestionRank: i + 1,
			Timestamp:      time.Now(),
		}); err != nil {
			c.logger.Debug("failed to publish claim intent", "error", err)
		}

		c.logger.Info("agent autonomously claimed quest",
			"agent_id", agent.ID,
			"quest_id", suggestion.QuestID,
			"score", suggestion.Score)

		// Clear cached suggestions eagerly so a second heartbeat firing before
		// the KV watch update arrives doesn't attempt another claim.
		c.trackersMu.Lock()
		instance := domain.ExtractInstance(string(agent.ID))
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
func (c *Component) shopBudget(agent *agentprogression.Agent) int64 {
	switch agent.Status {
	case domain.AgentIdle:
		surplus := agent.XP - agent.XPToLevel
		if surplus < c.config.MinXPSurplusForShopping {
			return 0
		}
		return int64(float64(surplus) * c.config.MaxShopSpendRatio)
	case domain.AgentCooldown:
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
		shouldExecute: func(agent *agentprogression.Agent, _ *agentTracker) bool {
			if c.store == nil {
				return false
			}
			return c.shopBudget(agent) > 0
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			return c.executeShop(ctx, agent)
		},
	}
}

// executeShop computes budget, lists affordable items, and purchases the best one.
func (c *Component) executeShop(ctx context.Context, agent *agentprogression.Agent) error {
	budget := c.shopBudget(agent)
	if budget <= 0 {
		return nil
	}

	items := c.store.ListItems(agent.Tier)
	item := pickBestItem(agent, items, budget)
	if item == nil {
		return nil
	}

	// Approval gate: block in supervised/manual mode
	if !c.requestApproval(ctx, domain.ApprovalAutonomyShop,
		fmt.Sprintf("Agent %s wants to purchase %s", agent.Name, item.Name),
		fmt.Sprintf("Item: %s (cost: %d XP, budget: %d XP)", item.Name, item.XPCost, budget),
		map[string]any{"agent_id": agent.ID, "item_id": item.ID, "xp_cost": item.XPCost},
	) {
		c.logger.Info("shop purchase denied by DM", "agent_id", agent.ID, "item_id", item.ID)
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

	// Emit shop intent for observability
	if err := SubjectAutonomyShopIntent.Publish(ctx, c.deps.NATSClient, ShopIntentPayload{
		AgentID:   domain.AgentID(agent.ID),
		ItemID:    item.ID,
		ItemName:  item.Name,
		XPCost:    item.XPCost,
		Budget:    budget,
		Strategic: false,
		Timestamp: time.Now(),
	}); err != nil {
		c.logger.Debug("failed to publish shop intent", "error", err)
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
		shouldExecute: func(agent *agentprogression.Agent, _ *agentTracker) bool {
			if c.store == nil || agent.Status != domain.AgentOnQuest {
				return false
			}
			needsShield := !hasConsumable(agent, string(agentstore.ConsumableQualityShield))
			needsBoost := !hasConsumable(agent, string(agentstore.ConsumableXPBoost))
			if !needsShield && !needsBoost {
				return false
			}
			return agent.XP > 0
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			return c.executeShopStrategic(ctx, agent)
		},
	}
}

// executeShopStrategic buys quality_shield first (protects review), then xp_boost.
// Cap at one purchase per heartbeat.
func (c *Component) executeShopStrategic(ctx context.Context, agent *agentprogression.Agent) error {
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

			// Approval gate: block in supervised/manual mode
			if !c.requestApproval(ctx, domain.ApprovalAutonomyShop,
				fmt.Sprintf("Agent %s wants to buy %s (strategic)", agent.Name, item.Name),
				fmt.Sprintf("Item: %s (cost: %d XP, strategic mid-quest purchase)", item.Name, item.XPCost),
				map[string]any{"agent_id": agent.ID, "item_id": item.ID, "strategic": true},
			) {
				c.logger.Info("strategic purchase denied by DM", "agent_id", agent.ID, "item_id", item.ID)
				return nil
			}

			_, err := c.store.Purchase(ctx, agent.ID, item.ID, agent.XP, agent.Level, agent.Guilds)
			if err != nil {
				c.logger.Warn("autonomous strategic purchase failed",
					"agent_id", agent.ID,
					"item_id", item.ID,
					"error", err)
				return err
			}

			// Emit shop intent for observability (strategic purchase)
			if err := SubjectAutonomyShopIntent.Publish(ctx, c.deps.NATSClient, ShopIntentPayload{
				AgentID:   domain.AgentID(agent.ID),
				ItemID:    item.ID,
				ItemName:  item.Name,
				XPCost:    item.XPCost,
				Budget:    agent.XP,
				Strategic: true,
				Timestamp: time.Now(),
			}); err != nil {
				c.logger.Debug("failed to publish strategic shop intent", "error", err)
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
func consumableForStatus(status domain.AgentStatus) string {
	switch status {
	case domain.AgentIdle, domain.AgentOnQuest:
		return string(agentstore.ConsumableXPBoost)
	case domain.AgentInBattle:
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
		shouldExecute: func(agent *agentprogression.Agent, tracker *agentTracker) bool {
			if c.store == nil {
				return false
			}
			switch agent.Status {
			case domain.AgentIdle:
				// Use xp_boost when about to claim (has suggestions)
				return hasConsumable(agent, string(agentstore.ConsumableXPBoost)) &&
					len(tracker.suggestions) > 0 &&
					!hasActiveEffect(agent, string(agentstore.ConsumableXPBoost))
			case domain.AgentOnQuest:
				return hasConsumable(agent, string(agentstore.ConsumableXPBoost)) &&
					!hasActiveEffect(agent, string(agentstore.ConsumableXPBoost))
			case domain.AgentInBattle:
				return hasConsumable(agent, string(agentstore.ConsumableQualityShield)) &&
					!hasActiveEffect(agent, string(agentstore.ConsumableQualityShield))
			default:
				return false
			}
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			consumableID := consumableForStatus(agent.Status)
			if consumableID == "" {
				return nil
			}

			// Approval gate: block in supervised/manual mode
			if !c.requestApproval(ctx, domain.ApprovalAutonomyUse,
				fmt.Sprintf("Agent %s wants to use %s", agent.Name, consumableID),
				fmt.Sprintf("Consumable: %s (agent status: %s)", consumableID, string(agent.Status)),
				map[string]any{"agent_id": agent.ID, "consumable_id": consumableID},
			) {
				c.logger.Info("consumable use denied by DM", "agent_id", agent.ID, "consumable_id", consumableID)
				return nil
			}

			if err := c.store.UseConsumable(ctx, agent.ID, consumableID, agent.CurrentQuest); err != nil {
				c.logger.Warn("autonomous consumable use failed",
					"agent_id", agent.ID,
					"consumable_id", consumableID,
					"error", err)
				return err
			}

			// Emit use intent for observability
			if err := SubjectAutonomyUseIntent.Publish(ctx, c.deps.NATSClient, UseIntentPayload{
				AgentID:      domain.AgentID(agent.ID),
				ConsumableID: consumableID,
				AgentStatus:  agent.Status,
				QuestID:      agent.CurrentQuest,
				Timestamp:    time.Now(),
			}); err != nil {
				c.logger.Debug("failed to publish use intent", "error", err)
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
		shouldExecute: func(agent *agentprogression.Agent, _ *agentTracker) bool {
			if c.store == nil || agent.Status != domain.AgentCooldown {
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
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			// Approval gate: block in supervised/manual mode
			if !c.requestApproval(ctx, domain.ApprovalAutonomyUse,
				fmt.Sprintf("Agent %s wants to skip cooldown", agent.Name),
				"Using cooldown_skip consumable to end cooldown early",
				map[string]any{"agent_id": agent.ID, "consumable_id": string(agentstore.ConsumableCooldownSkip)},
			) {
				c.logger.Info("cooldown skip denied by DM", "agent_id", agent.ID)
				return nil
			}

			if err := c.store.UseConsumable(ctx, agent.ID, string(agentstore.ConsumableCooldownSkip), nil); err != nil {
				c.logger.Warn("autonomous cooldown skip failed",
					"agent_id", agent.ID,
					"error", err)
				return err
			}

			// Emit use intent for observability (cooldown skip)
			if err := SubjectAutonomyUseIntent.Publish(ctx, c.deps.NATSClient, UseIntentPayload{
				AgentID:      domain.AgentID(agent.ID),
				ConsumableID: string(agentstore.ConsumableCooldownSkip),
				AgentStatus:  agent.Status,
				Timestamp:    time.Now(),
			}); err != nil {
				c.logger.Debug("failed to publish cooldown skip intent", "error", err)
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
func hasConsumable(agent *agentprogression.Agent, consumableID string) bool {
	if agent.Consumables == nil {
		return false
	}
	return agent.Consumables[consumableID] > 0
}

// hasActiveEffect returns true if the agent has an active effect of the given type
// with quests remaining.
func hasActiveEffect(agent *agentprogression.Agent, effectType string) bool {
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
func pickBestItem(agent *agentprogression.Agent, items []agentstore.StoreItem, budget int64) *agentstore.StoreItem {
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

// joinGuildAction returns the action for autonomously joining guilds.
// The agent evaluates N guild choices scored by reputation, success rate,
// capacity, and skill affinity, then joins the highest-scored option.
// Only fires when idle or on cooldown, unguilded (or below max), and meets
// the minimum level threshold.
func (c *Component) joinGuildAction() action {
	return action{
		name: "join_guild",
		shouldExecute: func(agent *agentprogression.Agent, _ *agentTracker) bool {
			if c.guilds == nil {
				return false
			}
			if agent.Level < c.config.GuildJoinMinLevel {
				return false
			}
			if agent.Status != domain.AgentIdle && agent.Status != domain.AgentCooldown {
				return false
			}
			// Use agent entity's Guilds field (from KV snapshot) for the guard.
			// Agent.Guilds may be stale since JoinGuild doesn't update the agent entity,
			// but executeJoinGuild filters out already-joined guilds via GetAgentGuilds
			// and handles "already a member" errors gracefully.
			return len(agent.Guilds) < c.config.MaxGuildsPerAgent
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			return c.executeJoinGuild(ctx, agent)
		},
	}
}

// executeJoinGuild scores available guilds and joins the best match.
// Logs all N choices for observability before picking.
func (c *Component) executeJoinGuild(ctx context.Context, agent *agentprogression.Agent) error {
	allGuilds := c.guilds.ListGuilds()
	currentGuildIDs := c.guilds.GetAgentGuilds(domain.AgentID(agent.ID))

	suggestions := c.scoreGuilds(agent, allGuilds, currentGuildIDs)
	if len(suggestions) == 0 {
		return nil
	}

	// Log all evaluated choices for observability
	for i, s := range suggestions {
		c.logger.Debug("guild suggestion evaluated",
			"agent_id", agent.ID,
			"rank", i+1,
			"guild_id", s.GuildID,
			"guild_name", s.GuildName,
			"score", fmt.Sprintf("%.3f", s.Score),
			"reason", s.Reason)
	}

	// Pick the best (index 0 — sorted descending by score)
	best := suggestions[0]

	// Approval gate: block in supervised/manual mode
	if !c.requestApproval(ctx, domain.ApprovalAutonomyGuild,
		fmt.Sprintf("Agent %s wants to join guild %s", agent.Name, best.GuildName),
		fmt.Sprintf("Guild: %s (score: %.3f, reason: %s)", best.GuildName, best.Score, best.Reason),
		map[string]any{"agent_id": agent.ID, "guild_id": string(best.GuildID), "score": best.Score},
	) {
		c.logger.Info("guild join denied by DM", "agent_id", agent.ID, "guild_id", best.GuildID)
		return nil
	}

	if err := c.guilds.JoinGuild(ctx, domain.GuildID(best.GuildID), domain.AgentID(agent.ID)); err != nil {
		// "Already a member" means the KV agent snapshot was stale — not a real error.
		// "Guild is full" can happen between scoring and joining — also not actionable.
		if errors.Is(err, guildformation.ErrAlreadyMember) || errors.Is(err, guildformation.ErrGuildFull) {
			c.logger.Debug("guild join skipped (stale data)",
				"agent_id", agent.ID,
				"guild_id", best.GuildID,
				"reason", err)
			return nil
		}
		c.logger.Warn("autonomous guild join failed",
			"agent_id", agent.ID,
			"guild_id", best.GuildID,
			"error", err)
		return err
	}

	// Emit guild intent for observability
	if err := SubjectAutonomyGuildIntent.Publish(ctx, c.deps.NATSClient, GuildIntentPayload{
		AgentID:          domain.AgentID(agent.ID),
		GuildID:          string(best.GuildID),
		GuildName:        best.GuildName,
		Score:            best.Score,
		ChoicesEvaluated: len(suggestions),
		Timestamp:        time.Now(),
	}); err != nil {
		c.logger.Debug("failed to publish guild intent", "error", err)
	}

	c.logger.Info("agent autonomously joined guild",
		"agent_id", agent.ID,
		"guild_id", best.GuildID,
		"guild_name", best.GuildName,
		"score", fmt.Sprintf("%.3f", best.Score),
		"choices_evaluated", len(suggestions))

	return nil
}

// =============================================================================
// GUILD SCORING
// =============================================================================
// Scoring weights for guild selection. Each factor produces a 0.0-1.0 score.
// =============================================================================

const (
	guildWeightReputation = 0.35
	guildWeightSuccess    = 0.25
	guildWeightCapacity   = 0.15
	guildWeightAffinity   = 0.25
)

// GuildSuggestion represents a scored guild option for an agent.
type GuildSuggestion struct {
	GuildID   domain.GuildID
	GuildName string
	Score     float64
	Reason    string
}

// scoreGuilds filters, scores, and ranks guilds for an agent.
// Returns top N suggestions sorted descending by score.
func (c *Component) scoreGuilds(agent *agentprogression.Agent, guilds []*domain.Guild, currentGuildIDs []domain.GuildID) []GuildSuggestion {
	memberSet := make(map[domain.GuildID]bool, len(currentGuildIDs))
	for _, gid := range currentGuildIDs {
		memberSet[domain.GuildID(gid)] = true
	}

	var suggestions []GuildSuggestion
	for _, guild := range guilds {
		// Filter: already a member
		if memberSet[guild.ID] {
			continue
		}
		// Filter: guild is full
		if guild.MaxMembers > 0 && len(guild.Members) >= guild.MaxMembers {
			continue
		}
		// Filter: agent below guild MinLevel
		if agent.Level < guild.MinLevel {
			continue
		}

		score, reason := scoreGuild(agent, guild)
		suggestions = append(suggestions, GuildSuggestion{
			GuildID:   guild.ID,
			GuildName: guild.Name,
			Score:     score,
			Reason:    reason,
		})
	}

	// Sort descending by score (stable to preserve determinism on ties)
	sort.SliceStable(suggestions, func(i, j int) bool {
		return suggestions[i].Score > suggestions[j].Score
	})

	// Return top N
	if len(suggestions) > c.config.GuildSuggestionsN {
		suggestions = suggestions[:c.config.GuildSuggestionsN]
	}

	return suggestions
}

// scoreGuild computes a weighted score for how well a guild fits an agent.
func scoreGuild(agent *agentprogression.Agent, guild *domain.Guild) (float64, string) {
	reputation := guild.Reputation // Already 0.0-1.0

	successRate := guild.SuccessRate // Already 0.0-1.0

	var capacity float64
	if guild.MaxMembers > 0 {
		capacity = float64(guild.MaxMembers-len(guild.Members)) / float64(guild.MaxMembers)
	} else {
		capacity = 1.0 // No cap = always room
	}

	affinity := guildSkillAffinity(agent, guild)

	score := reputation*guildWeightReputation +
		successRate*guildWeightSuccess +
		capacity*guildWeightCapacity +
		affinity*guildWeightAffinity

	// Build reason string from dominant factor
	var parts []string
	if reputation >= 0.7 {
		parts = append(parts, "high reputation")
	}
	if successRate >= 0.7 {
		parts = append(parts, "strong success rate")
	}
	if affinity >= 0.5 {
		parts = append(parts, "skill affinity")
	}
	reason := "general fit"
	if len(parts) > 0 {
		reason = strings.Join(parts, ", ")
	}

	return score, reason
}

// guildSkillAffinity computes overlap between agent's skills and guild's QuestTypes.
// Returns 0.0-1.0: higher means the guild handles quest types matching the agent's skills.
// If the guild has no QuestTypes, returns 0.5 (neutral — no signal).
func guildSkillAffinity(agent *agentprogression.Agent, guild *domain.Guild) float64 {
	if len(guild.QuestTypes) == 0 {
		return 0.5 // No quest type signal — neutral
	}
	if len(agent.SkillProficiencies) == 0 {
		return 0.0 // Agent has no skills — no affinity
	}

	// Build set of guild quest types for O(1) lookup
	questTypeSet := make(map[string]bool, len(guild.QuestTypes))
	for _, qt := range guild.QuestTypes {
		questTypeSet[qt] = true
	}

	// Count agent skills that match guild quest types
	matches := 0
	for skill := range agent.SkillProficiencies {
		if questTypeSet[string(skill)] {
			matches++
		}
	}

	return float64(matches) / float64(len(agent.SkillProficiencies))
}
