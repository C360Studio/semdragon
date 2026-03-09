package autonomy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semstreams/natsclient"

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
// to the next suggestion. CAS (Compare-And-Swap) on the quest KV entry ensures
// only one agent wins the claim — losers get ErrKVRevisionMismatch and skip.
func (c *Component) executeClaimQuest(ctx context.Context, agent *agentprogression.Agent, tracker *agentTracker) error {
	for i, suggestion := range tracker.suggestions {
		// Read quest from KV with revision for CAS
		entity, revision, err := c.graph.GetQuestWithRevision(ctx, domain.QuestID(suggestion.QuestID))
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

		// Skip party sub-quests — these are managed by questdagexec,
		// not available for autonomous claiming.
		if quest.PartyID != nil {
			c.logger.Debug("quest belongs to a party, skipping",
				"quest_id", suggestion.QuestID,
				"party_id", *quest.PartyID)
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

		// CAS write quest state: claimed. If another agent already wrote to this
		// quest (changing the revision), EmitEntityCAS returns ErrKVRevisionMismatch
		// and we skip to the next suggestion.
		now := time.Now()
		agentID := domain.AgentID(agent.ID)
		quest.Status = domain.QuestClaimed
		quest.ClaimedBy = &agentID
		quest.ClaimedAt = &now
		quest.Attempts++

		if err := c.graph.EmitEntityCAS(ctx, quest, "quest.claimed", revision); err != nil {
			if errors.Is(err, natsclient.ErrKVRevisionMismatch) {
				c.logger.Debug("quest already claimed by another agent (CAS conflict), skipping",
					"quest_id", suggestion.QuestID,
					"agent_id", agent.ID)
				continue
			}
			c.errorsCount.Add(1)
			c.logger.Error("failed to write quest claim",
				"quest_id", suggestion.QuestID,
				"error", err)
			continue
		}

		// CAS succeeded — we own this quest. Transition to in_progress.
		// Re-read revision after our claim write for the next CAS.
		_, revision, err = c.graph.GetQuestWithRevision(ctx, domain.QuestID(quest.ID))
		if err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to re-read quest after claim", "error", err)
			// Quest is claimed but we can't transition — recoverable via manual StartQuest.
		} else {
			now1 := time.Now()
			quest.Status = domain.QuestInProgress
			quest.StartedAt = &now1

			if err := c.graph.EmitEntityCAS(ctx, quest, "quest.started", revision); err != nil {
				c.errorsCount.Add(1)
				c.logger.Error("failed to start quest after claim",
					"quest_id", suggestion.QuestID,
					"error", err)
			}
		}

		// Write agent state: on_quest (no CAS needed — only this agent's heartbeat writes its state)
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

	_, err := c.store.Purchase(ctx, agent.ID, item.ID, agent.XP, agent.Level, agent.Guild)
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

			_, err := c.store.Purchase(ctx, agent.ID, item.ID, agent.XP, agent.Level, agent.Guild)
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
			// Use agent entity's Guild field (from KV snapshot) for the guard.
			// The field may be stale but executeJoinGuild uses the authoritative
			// in-memory projection and handles "already a member" errors gracefully.
			return agent.Guild == ""
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
	currentGuild := c.guilds.GetAgentGuild(domain.AgentID(agent.ID))

	// Build a slice for scoreGuilds filtering (already-member check).
	var currentGuildIDs []domain.GuildID
	if currentGuild != "" {
		currentGuildIDs = []domain.GuildID{currentGuild}
	}

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

// createGuildAction returns the action for autonomously proposing guild creation.
// Only Master-tier (level 16+) agents can propose founding a guild when they have
// strong fellowship with enough unguilded agents. The proposal goes through the
// DM approval gate before creating the guild.
func (c *Component) createGuildAction() action {
	return action{
		name: "create_guild",
		shouldExecute: func(agent *agentprogression.Agent, _ *agentTracker) bool {
			if c.guilds == nil {
				return false
			}
			if agent.Level < c.config.GuildCreateMinLevel {
				return false
			}
			if agent.Status != domain.AgentIdle && agent.Status != domain.AgentCooldown {
				return false
			}
			// Only agents who are not already a guildmaster should propose.
			// GetAgentGuild uses the in-memory projection which is authoritative.
			currentGuild := c.guilds.GetAgentGuild(agent.ID)
			if currentGuild != "" {
				guild, ok := c.guilds.GetGuild(currentGuild)
				if ok && guild.FoundedBy == agent.ID {
					return false // Already a guildmaster
				}
			}
			return true
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			return c.executeCreateGuild(ctx, agent)
		},
	}
}

// executeCreateGuild scores fellowship with other agents and proposes a guild
// if enough candidates have strong affinity with this agent.
func (c *Component) executeCreateGuild(ctx context.Context, agent *agentprogression.Agent) error {
	allGuilds := c.guilds.ListGuilds()

	// Collect unguilded agents from KV cache (exclude self).
	c.trackersMu.RLock()
	var candidates []*agentprogression.Agent
	for _, tracker := range c.trackers {
		if tracker.agent == nil || tracker.agent.ID == agent.ID {
			continue
		}
		// Only consider agents not already in a guild.
		if c.guilds.GetAgentGuild(tracker.agent.ID) == "" {
			agentCopy := *tracker.agent
			candidates = append(candidates, &agentCopy)
		}
	}
	c.trackersMu.RUnlock()

	// Score fellowship with each candidate. Use authoritative guild membership
	// from GetAgentGuild (in-memory projection) rather than the potentially
	// stale Agent.Guild field from the KV snapshot.
	var fellows []fellowCandidate
	for _, candidate := range candidates {
		var peerGuildCount int
		if c.guilds.GetAgentGuild(candidate.ID) != "" {
			peerGuildCount = 1
		}
		score := scoreFellowship(agent, candidate, allGuilds, peerGuildCount)
		if score > 0.3 { // Minimum fellowship threshold
			fellows = append(fellows, fellowCandidate{agent: candidate, score: score})
		}
	}

	// Need enough fellowship candidates to form a guild.
	if len(fellows) < c.config.GuildCreateMinFellows {
		return nil
	}

	// Sort by fellowship score descending.
	sort.Slice(fellows, func(i, j int) bool {
		return fellows[i].score > fellows[j].score
	})

	// Select diverse candidates (mixed skills) for the founding group.
	maxFounders := c.config.GuildCreateMaxFounders
	selected := selectFellowshipCandidates(agent, fellows, maxFounders)
	if len(selected) < c.config.GuildCreateMinFellows {
		// Fall back to all fellows if diversity selection came up short.
		selected = fellows
		if len(selected) > maxFounders {
			selected = selected[:maxFounders]
		}
	}

	// Build candidate ID list and score map for observability.
	candidateIDs := make([]domain.AgentID, 0, len(selected))
	fellowshipScores := make(map[string]float64, len(selected))
	for _, f := range selected {
		candidateIDs = append(candidateIDs, f.agent.ID)
		fellowshipScores[string(f.agent.ID)] = f.score
	}

	guildName := agentGuildName(agent)

	// Emit proposal intent for observability — before approval gate so
	// watchers see the proposal regardless of approval outcome.
	if err := SubjectAutonomyGuildCreateIntent.Publish(ctx, c.deps.NATSClient, GuildCreateIntentPayload{
		AgentID:          agent.ID,
		GuildName:        guildName,
		CandidateIDs:     candidateIDs,
		FellowshipScores: fellowshipScores,
		Timestamp:        time.Now(),
	}); err != nil {
		c.logger.Warn("failed to publish guild create intent", "error", err)
	}

	// DM approval gate — guild founding is a high-trust action.
	if !c.requestApproval(ctx, domain.ApprovalAutonomyGuildCreate,
		fmt.Sprintf("Agent %s proposes founding guild %q with %d candidates", agent.Name, guildName, len(candidateIDs)),
		fmt.Sprintf("Guild: %s (founder level: %d, candidates: %d)", guildName, agent.Level, len(candidateIDs)),
		map[string]any{
			"agent_id":    agent.ID,
			"guild_name":  guildName,
			"candidates":  candidateIDs,
			"fellowships": fellowshipScores,
		},
	) {
		c.logger.Info("guild creation denied by DM", "agent_id", agent.ID, "guild_name", guildName)
		return nil
	}

	// Create the guild via guildformation.
	guild, err := c.guilds.CreateGuild(ctx, guildformation.CreateGuildParams{
		Name:      guildName,
		Culture:   "Founded through fellowship and shared purpose",
		FounderID: agent.ID,
		MinLevel:  1,
	})
	if err != nil {
		c.logger.Warn("autonomous guild creation failed",
			"agent_id", agent.ID,
			"error", err)
		return err
	}

	// Invite top candidates. The founder is already in the guild (added by
	// CreateGuild), so we invite the remaining fellows up to the minimum.
	invited := 0
	for _, f := range selected {
		if invited >= c.config.GuildCreateMinFellows-1 {
			break
		}
		if joinErr := c.guilds.JoinGuild(ctx, guild.ID, f.agent.ID); joinErr != nil {
			c.logger.Debug("failed to add fellowship candidate to guild",
				"guild_id", guild.ID,
				"agent_id", f.agent.ID,
				"error", joinErr)
			continue
		}
		invited++
	}

	c.logger.Info("agent autonomously proposed and created guild",
		"agent_id", agent.ID,
		"guild_id", guild.ID,
		"guild_name", guildName,
		"candidates", len(candidateIDs),
		"invited", invited)

	return nil
}

// =============================================================================
// GUILD APPLICATION AND REVIEW ACTIONS
// =============================================================================

// applyToGuildAction returns the action for submitting applications to pending guilds.
// Candidates discover pending guilds and apply based on skill affinity scoring.
func (c *Component) applyToGuildAction() action {
	return action{
		name: "apply_to_guild",
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
			// Only unguilded agents can apply.
			if agent.Guild != "" {
				return false
			}
			return c.guilds.HasPendingGuilds()
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			return c.executeApplyToGuild(ctx, agent)
		},
	}
}

// executeApplyToGuild scores pending guilds and submits an application to the best match.
func (c *Component) executeApplyToGuild(ctx context.Context, agent *agentprogression.Agent) error {
	pending := c.guilds.ListPendingGuilds()
	currentGuild := c.guilds.GetAgentGuild(agent.ID)

	var currentGuildIDs []domain.GuildID
	if currentGuild != "" {
		currentGuildIDs = []domain.GuildID{currentGuild}
	}

	// Filter and score pending guilds using the same scoring infrastructure as joinGuild.
	suggestions := c.scoreGuilds(agent, pending, currentGuildIDs)
	if len(suggestions) == 0 {
		return nil
	}

	best := suggestions[0]

	// Check we haven't already applied to this guild.
	guild, ok := c.guilds.GetGuild(best.GuildID)
	if !ok {
		return nil
	}
	for _, app := range guild.Applications {
		if app.ApplicantID == agent.ID && app.Status == domain.ApplicationPending {
			return nil // Already applied
		}
	}

	// Build a reason message from skills
	var skillNames []string
	for skill := range agent.SkillProficiencies {
		skillNames = append(skillNames, string(skill))
	}
	message := fmt.Sprintf("Skill affinity (score: %.2f): %s", best.Score, strings.Join(skillNames, ", "))

	if err := c.guilds.SubmitApplication(ctx, best.GuildID, agent, message); err != nil {
		if errors.Is(err, guildformation.ErrDuplicateApplication) || errors.Is(err, guildformation.ErrAlreadyMember) {
			return nil // Stale data, not a real error
		}
		c.logger.Warn("autonomous guild application failed",
			"agent_id", agent.ID,
			"guild_id", best.GuildID,
			"error", err)
		return err
	}

	c.logger.Info("agent autonomously applied to pending guild",
		"agent_id", agent.ID,
		"guild_id", best.GuildID,
		"guild_name", best.GuildName,
		"score", fmt.Sprintf("%.3f", best.Score))

	return nil
}

// reviewGuildApplicationsAction returns the action for founders to review pending applications.
// Uses a scoring heuristic: skill complementarity, tier, and level proximity.
func (c *Component) reviewGuildApplicationsAction() action {
	return action{
		name: "review_guild_applications",
		shouldExecute: func(agent *agentprogression.Agent, _ *agentTracker) bool {
			if c.guilds == nil {
				return false
			}
			if agent.Status != domain.AgentIdle && agent.Status != domain.AgentCooldown {
				return false
			}
			// Check: is this agent the founder of a pending guild with pending applications?
			gid := c.guilds.GetAgentGuild(agent.ID)
			if gid == "" {
				return false
			}
			guild, ok := c.guilds.GetGuild(gid)
			if !ok || guild.Status != domain.GuildPending || guild.FoundedBy != agent.ID {
				return false
			}
			for _, app := range guild.Applications {
				if app.Status == domain.ApplicationPending {
					return true
				}
			}
			return false
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			return c.executeReviewGuildApplications(ctx, agent)
		},
	}
}

// Scoring weights for guild application review.
const (
	reviewWeightSkillComplement = 0.40
	reviewWeightTier            = 0.30
	reviewWeightLevelProximity  = 0.30

	// Minimum score to accept an applicant.
	reviewAcceptThreshold = 0.35
)

// executeReviewGuildApplications reviews all pending applications for guilds founded by this agent.
func (c *Component) executeReviewGuildApplications(ctx context.Context, agent *agentprogression.Agent) error {
	gid := c.guilds.GetAgentGuild(agent.ID)
	if gid == "" {
		return nil
	}
	guild, ok := c.guilds.GetGuild(gid)
	if !ok || guild.Status != domain.GuildPending || guild.FoundedBy != agent.ID {
		return nil
	}

	for _, app := range guild.Applications {
		if app.Status != domain.ApplicationPending {
			continue
		}

		score, reason := c.scoreApplication(agent, guild, &app)
		accepted := score >= reviewAcceptThreshold

		decision := "rejected"
		if accepted {
			decision = "accepted"
		}

		reviewReason := fmt.Sprintf("%s (score: %.2f, %s)", decision, score, reason)

		if err := c.guilds.ReviewApplication(ctx, guild.ID, app.ID, agent.ID, accepted, reviewReason); err != nil {
			c.logger.Warn("failed to review guild application",
				"guild_id", guild.ID,
				"app_id", app.ID,
				"error", err)
			continue
		}

		c.logger.Info("founder reviewed guild application",
			"guild_id", guild.ID,
			"applicant", app.ApplicantID,
			"decision", decision,
			"score", fmt.Sprintf("%.3f", score),
			"reason", reason)
	}

	return nil
}

// scoreApplication computes how well an applicant fits the guild.
// Factors: skill complementarity with existing members, tier, level proximity to founder.
func (c *Component) scoreApplication(founder *agentprogression.Agent, guild *domain.Guild, app *domain.GuildApplication) (float64, string) {
	// Skill complementarity: does the applicant bring new skills?
	// Build skill map from all members in a single lock acquisition.
	c.trackersMu.RLock()
	memberSkills := make(map[domain.AgentID]map[domain.SkillTag]domain.SkillProficiency)
	for _, tracker := range c.trackers {
		if tracker.agent != nil {
			memberSkills[tracker.agent.ID] = tracker.agent.SkillProficiencies
		}
	}
	c.trackersMu.RUnlock()

	existingSkills := make(map[domain.SkillTag]bool)
	for _, m := range guild.Members {
		if skills, ok := memberSkills[m.AgentID]; ok {
			for skill := range skills {
				existingSkills[skill] = true
			}
		}
	}

	newSkills := 0
	for _, skill := range app.Skills {
		if !existingSkills[skill] {
			newSkills++
		}
	}
	var skillScore float64
	if len(app.Skills) > 0 {
		skillScore = float64(newSkills) / float64(len(app.Skills))
	} else {
		skillScore = 0.3 // No skill data — slight penalty
	}

	// Tier score: higher tier = more capable.
	// Normalize 1-5 (Apprentice-Grandmaster) to 0.2-1.0.
	tierScore := float64(app.Tier) / float64(domain.TierGrandmaster)

	// Level proximity: closer to founder = better fit.
	levelDiff := levelAbs(founder.Level - app.Level)
	levelScore := 1.0 / (1.0 + float64(levelDiff)/5.0)

	total := skillScore*reviewWeightSkillComplement +
		tierScore*reviewWeightTier +
		levelScore*reviewWeightLevelProximity

	// Build reason
	var parts []string
	if skillScore >= 0.5 {
		parts = append(parts, "brings new skills")
	}
	if tierScore >= 0.4 {
		parts = append(parts, fmt.Sprintf("tier %d", int(app.Tier)))
	}
	if levelScore >= 0.5 {
		parts = append(parts, "close level")
	}
	reason := "general evaluation"
	if len(parts) > 0 {
		reason = strings.Join(parts, ", ")
	}

	return total, reason
}

// =============================================================================
// FELLOWSHIP SCORING
// =============================================================================
// Fellowship measures social affinity between two agents. Higher scores indicate
// agents who would work well together in a guild. Factors:
//   - Skill complementarity (40%): diverse skills attract
//   - Reputation      (30%): high peer review scores indicate good collaborators
//   - Level proximity (15%): similar level agents relate better
//   - Guild need      (15%): unguilded agents have higher incentive
//
// =============================================================================

const (
	fellowWeightSkillComplement = 0.40
	fellowWeightReputation      = 0.30
	fellowWeightLevelProximity  = 0.15
	fellowWeightGuildNeed       = 0.15
)

// fellowCandidate pairs an agent with its computed fellowship score.
type fellowCandidate struct {
	agent *agentprogression.Agent
	score float64
}

// scoreFellowship computes social affinity between two agents.
// peerGuildCount is 0 or 1 based on whether the peer currently belongs to a guild
// (from GetAgentGuild), reflecting the single-guild constraint.
// Returns 0.0-1.0: higher means stronger fellowship bond.
func scoreFellowship(self, peer *agentprogression.Agent, guilds []*domain.Guild, peerGuildCount int) float64 {
	// Skill complementarity: agents with different skills complement each other.
	skillComplement := skillComplementarity(self, peer)

	// Reputation: both agents should have good peer review scores.
	reputation := averageReputation(self, peer)

	// Level proximity: agents within a few levels relate better.
	// Decays toward 0 as gap grows; same level = 1.0.
	levelDiff := levelAbs(self.Level - peer.Level)
	levelProximity := 1.0 / (1.0 + float64(levelDiff)/5.0)

	// Guild need: unguilded agents have higher incentive to form guilds.
	// Penalise candidates who already share a guild with the proposer —
	// they already collaborate; founding a new guild together adds less value.
	guildNeed := 1.0
	if peerGuildCount > 0 {
		guildNeed = 0.3 // Already in a guild, less incentive
		for _, g := range guilds {
			if isMemberOf(g, self.ID) && isMemberOf(g, peer.ID) {
				guildNeed = 0.1 // Already share a guild — minimal incentive
				break
			}
		}
	}

	return skillComplement*fellowWeightSkillComplement +
		reputation*fellowWeightReputation +
		levelProximity*fellowWeightLevelProximity +
		guildNeed*fellowWeightGuildNeed
}

// skillComplementarity returns 0.0-1.0 measuring how well two agents'
// skill sets complement each other. Higher when skills are diverse.
func skillComplementarity(a, b *agentprogression.Agent) float64 {
	if len(a.SkillProficiencies) == 0 && len(b.SkillProficiencies) == 0 {
		return 0.5 // No skill data — neutral
	}

	allSkills := make(map[domain.SkillTag]bool)
	sharedSkills := 0

	for skill := range a.SkillProficiencies {
		allSkills[skill] = true
	}
	for skill := range b.SkillProficiencies {
		if allSkills[skill] {
			sharedSkills++
		}
		allSkills[skill] = true
	}

	if len(allSkills) == 0 {
		return 0.5
	}

	// Complementarity = unique skills / total skills.
	// Higher when agents bring different skills to the table.
	uniqueSkills := len(allSkills) - sharedSkills
	return float64(uniqueSkills) / float64(len(allSkills))
}

// averageReputation returns the average normalized reputation of two agents.
// Normalises PeerReviewAvg (1-5 scale) to 0.0-1.0. Defaults to 0.5 when no
// peer reviews have been recorded yet.
func averageReputation(a, b *agentprogression.Agent) float64 {
	repA := 0.5 // Default neutral
	repB := 0.5
	if a.Stats.PeerReviewCount > 0 {
		repA = (a.Stats.PeerReviewAvg - 1.0) / 4.0 // Normalise 1-5 → 0-1
	}
	if b.Stats.PeerReviewCount > 0 {
		repB = (b.Stats.PeerReviewAvg - 1.0) / 4.0
	}
	return (repA + repB) / 2.0
}

// agentGuildName creates a guild name from the founding agent's display name or name.
func agentGuildName(agent *agentprogression.Agent) string {
	name := agent.Name
	if agent.DisplayName != "" {
		name = agent.DisplayName
	}
	return name + "'s Guild"
}

// selectFellowshipCandidates picks up to limit candidates from fellows,
// preferring those who bring skills not already represented.
// Always includes as many as possible from the sorted (best-first) input.
func selectFellowshipCandidates(founder *agentprogression.Agent, candidates []fellowCandidate, limit int) []fellowCandidate {
	if len(candidates) <= limit {
		return candidates
	}

	selected := make([]fellowCandidate, 0, limit)
	seenSkills := make(map[domain.SkillTag]bool)

	// Seed seen skills with the founder's proficiencies.
	for skill := range founder.SkillProficiencies {
		seenSkills[skill] = true
	}

	// First pass: prefer candidates who introduce at least one new skill.
	for _, c := range candidates {
		if len(selected) >= limit {
			break
		}
		hasNewSkill := false
		for skill := range c.agent.SkillProficiencies {
			if !seenSkills[skill] {
				hasNewSkill = true
				break
			}
		}
		if hasNewSkill {
			selected = append(selected, c)
			for skill := range c.agent.SkillProficiencies {
				seenSkills[skill] = true
			}
		}
	}

	// Second pass: fill remaining slots from highest fellowship score.
	if len(selected) < limit {
		selectedIDs := make(map[domain.AgentID]bool, len(selected))
		for _, s := range selected {
			selectedIDs[s.agent.ID] = true
		}
		for _, c := range candidates {
			if len(selected) >= limit {
				break
			}
			if !selectedIDs[c.agent.ID] {
				selected = append(selected, c)
			}
		}
	}

	return selected
}

// isMemberOf checks if an agent is a member of a guild.
func isMemberOf(guild *domain.Guild, agentID domain.AgentID) bool {
	for _, m := range guild.Members {
		if m.AgentID == agentID {
			return true
		}
	}
	return false
}

// levelAbs returns the absolute value of an integer level difference.
func levelAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
