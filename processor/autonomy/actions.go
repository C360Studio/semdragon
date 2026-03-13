package autonomy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/natsclient"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/boidengine"
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
	approval := c.resolveApproval()
	if approval == nil || c.config.DMMode == domain.DMFullAuto || c.config.DMMode == domain.DMAssisted {
		return true
	}
	if c.config.SessionID == "" {
		return true
	}

	// Use the caller's context. In supervised/manual mode, evaluateAutonomy
	// extends the evaluation timeout to ApprovalTimeout+10s, providing
	// sufficient headroom for both the approval wait and subsequent I/O.
	resp, err := approval.RequestApproval(ctx, domain.ApprovalRequest{
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

		// Skip party-required quests — these must be claimed through
		// partycoord which forms a party and assigns via ClaimAndStartForParty.
		if quest.PartyRequired {
			c.logger.Debug("quest requires a party, skipping",
				"quest_id", suggestion.QuestID)
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

	// All suggestions exhausted — clear stale suggestions from the live tracker
	// so the agent doesn't retry the same failed list on the next heartbeat.
	// Fresh suggestions from the boid engine will repopulate if valid quests exist.
	c.trackersMu.Lock()
	instance := domain.ExtractInstance(string(agent.ID))
	if t, ok := c.trackers[instance]; ok {
		t.suggestions = nil
	}
	c.trackersMu.Unlock()

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
			if c.resolveStore() == nil {
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
	store := c.resolveStore()
	if store == nil {
		return nil
	}

	budget := c.shopBudget(agent)
	if budget <= 0 {
		return nil
	}

	items := store.ListItems(agent.Tier)
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

	_, err := store.Purchase(ctx, agent.ID, item.ID, agent.XP, agent.Level, agent.Guild)
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
			if c.resolveStore() == nil || agent.Status != domain.AgentOnQuest {
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
	store := c.resolveStore()
	if store == nil {
		return nil
	}

	items := store.ListItems(agent.Tier)

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

			_, err := store.Purchase(ctx, agent.ID, item.ID, agent.XP, agent.Level, agent.Guild)
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
			if c.resolveStore() == nil {
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

			store := c.resolveStore()
			if store == nil {
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

			if err := store.UseConsumable(ctx, agent.ID, consumableID, agent.CurrentQuest); err != nil {
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
			if c.resolveStore() == nil || agent.Status != domain.AgentCooldown {
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
			store := c.resolveStore()
			if store == nil {
				return nil
			}

			// Approval gate: block in supervised/manual mode
			if !c.requestApproval(ctx, domain.ApprovalAutonomyUse,
				fmt.Sprintf("Agent %s wants to skip cooldown", agent.Name),
				"Using cooldown_skip consumable to end cooldown early",
				map[string]any{"agent_id": agent.ID, "consumable_id": string(agentstore.ConsumableCooldownSkip)},
			) {
				c.logger.Info("cooldown skip denied by DM", "agent_id", agent.ID)
				return nil
			}

			if err := store.UseConsumable(ctx, agent.ID, string(agentstore.ConsumableCooldownSkip), nil); err != nil {
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
// Driven by boid guild suggestions: only fires when the boid engine has
// computed a "join" suggestion for this agent based on peer cohesion,
// shared wins, skill gaps, and guild reputation.
func (c *Component) joinGuildAction() action {
	return action{
		name: "join_guild",
		shouldExecute: func(agent *agentprogression.Agent, tracker *agentTracker) bool {
			if c.resolveGuilds() == nil {
				return false
			}
			if agent.Guild != "" {
				return false
			}
			// Only fire when boid engine suggests joining a specific guild
			return tracker.guildSuggestion != nil && tracker.guildSuggestion.Type == "join"
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, tracker *agentTracker) error {
			return c.executeJoinGuildFromSuggestion(ctx, agent, tracker.guildSuggestion)
		},
	}
}

// executeJoinGuildFromSuggestion joins the guild recommended by the boid engine.
// The boid engine has already scored guilds based on peer cohesion, shared wins,
// skill gaps, and reputation — we just execute the decision.
func (c *Component) executeJoinGuildFromSuggestion(ctx context.Context, agent *agentprogression.Agent, gs *boidengine.GuildSuggestion) error {
	guilds := c.resolveGuilds()
	if guilds == nil || gs == nil {
		return nil
	}

	guildID := domain.GuildID(gs.GuildID)

	// Look up guild name for logging/approval
	guildName := string(guildID)
	if guild, ok := guilds.GetGuild(guildID); ok {
		guildName = guild.Name
	}

	c.logger.Debug("boid guild join suggestion",
		"agent_id", agent.ID,
		"guild_id", guildID,
		"guild_name", guildName,
		"score", fmt.Sprintf("%.3f", gs.Score),
		"reason", gs.Reason)

	// Approval gate: block in supervised/manual mode
	if !c.requestApproval(ctx, domain.ApprovalAutonomyGuild,
		fmt.Sprintf("Agent %s wants to join guild %s", agent.Name, guildName),
		fmt.Sprintf("Guild: %s (score: %.3f, reason: %s)", guildName, gs.Score, gs.Reason),
		map[string]any{"agent_id": agent.ID, "guild_id": string(guildID), "score": gs.Score},
	) {
		c.logger.Info("guild join denied by DM", "agent_id", agent.ID, "guild_id", guildID)
		return nil
	}

	if err := guilds.JoinGuild(ctx, guildID, domain.AgentID(agent.ID)); err != nil {
		if errors.Is(err, guildformation.ErrAlreadyMember) || errors.Is(err, guildformation.ErrGuildFull) {
			c.logger.Debug("guild join skipped (stale data)",
				"agent_id", agent.ID,
				"guild_id", guildID,
				"reason", err)
			return nil
		}
		c.logger.Warn("autonomous guild join failed",
			"agent_id", agent.ID,
			"guild_id", guildID,
			"error", err)
		return err
	}

	// Emit guild intent for observability
	if err := SubjectAutonomyGuildIntent.Publish(ctx, c.deps.NATSClient, GuildIntentPayload{
		AgentID:          domain.AgentID(agent.ID),
		GuildID:          string(guildID),
		GuildName:        guildName,
		Score:            gs.Score,
		ChoicesEvaluated: 1, // Boid engine pre-selected the best
		Timestamp:        time.Now(),
	}); err != nil {
		c.logger.Debug("failed to publish guild intent", "error", err)
	}

	c.logger.Info("agent autonomously joined guild (boid-driven)",
		"agent_id", agent.ID,
		"guild_id", guildID,
		"guild_name", guildName,
		"score", fmt.Sprintf("%.3f", gs.Score),
		"reason", gs.Reason)

	return nil
}

// createGuildAction returns the action for autonomously proposing guild creation.
// Driven by boid guild suggestions: only fires when the boid engine has computed
// a "form" suggestion for this Expert+ agent based on peer cohesion and skill diversity.
func (c *Component) createGuildAction() action {
	return action{
		name: "create_guild",
		shouldExecute: func(agent *agentprogression.Agent, tracker *agentTracker) bool {
			if c.resolveGuilds() == nil {
				return false
			}
			if agent.Guild != "" {
				return false
			}
			if agent.Tier < domain.TierExpert {
				return false
			}
			// Only fire when boid engine suggests forming a guild
			return tracker.guildSuggestion != nil && tracker.guildSuggestion.Type == "form"
		},
		execute: func(ctx context.Context, agent *agentprogression.Agent, _ *agentTracker) error {
			return c.executeCreateGuildFromSuggestion(ctx, agent)
		},
	}
}

// executeCreateGuildFromSuggestion creates a guild when the boid engine has
// determined that formation conditions are met (peer cohesion + skill diversity).
// The actual member selection is left to the boid engine's join suggestions —
// this just creates the guild with the founder.
func (c *Component) executeCreateGuildFromSuggestion(ctx context.Context, agent *agentprogression.Agent) error {
	guilds := c.resolveGuilds()
	if guilds == nil {
		return nil
	}

	guildName := agentGuildName(agent)

	// Emit proposal intent for observability
	if err := SubjectAutonomyGuildCreateIntent.Publish(ctx, c.deps.NATSClient, GuildCreateIntentPayload{
		AgentID:   agent.ID,
		GuildName: guildName,
		Timestamp: time.Now(),
	}); err != nil {
		c.logger.Warn("failed to publish guild create intent", "error", err)
	}

	// DM approval gate — guild founding is a high-trust action
	if !c.requestApproval(ctx, domain.ApprovalAutonomyGuildCreate,
		fmt.Sprintf("Agent %s proposes founding guild %q (boid-driven)", agent.Name, guildName),
		fmt.Sprintf("Guild: %s (founder level: %d)", guildName, agent.Level),
		map[string]any{"agent_id": agent.ID, "guild_name": guildName},
	) {
		c.logger.Info("guild creation denied by DM", "agent_id", agent.ID, "guild_name", guildName)
		return nil
	}

	// Create the guild — founder is auto-added by CreateGuild.
	// Other agents will join organically via boid "join" suggestions.
	guild, err := guilds.CreateGuild(ctx, guildformation.CreateGuildParams{
		Name:      guildName,
		Culture:   "Founded through demonstrated peer cohesion",
		FounderID: agent.ID,
		MinLevel:  1,
	})
	if err != nil {
		c.logger.Warn("autonomous guild creation failed",
			"agent_id", agent.ID,
			"error", err)
		return err
	}

	c.logger.Info("agent autonomously founded guild (boid-driven)",
		"agent_id", agent.ID,
		"guild_id", guild.ID,
		"guild_name", guildName)

	return nil
}

// =============================================================================
// GUILD APPLICATION AND REVIEW ACTIONS
// =============================================================================

// reviewGuildApplicationsAction returns the action for founders to review pending applications.
// Uses a scoring heuristic: skill complementarity, tier, and level proximity.
func (c *Component) reviewGuildApplicationsAction() action {
	return action{
		name: "review_guild_applications",
		shouldExecute: func(agent *agentprogression.Agent, _ *agentTracker) bool {
			guilds := c.resolveGuilds()
			if guilds == nil {
				return false
			}
			if agent.Status != domain.AgentIdle && agent.Status != domain.AgentCooldown {
				return false
			}
			// Check: is this agent the founder of a pending guild with pending applications?
			gid := guilds.GetAgentGuild(agent.ID)
			if gid == "" {
				return false
			}
			guild, ok := guilds.GetGuild(gid)
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
	guilds := c.resolveGuilds()
	if guilds == nil {
		return nil
	}

	gid := guilds.GetAgentGuild(agent.ID)
	if gid == "" {
		return nil
	}
	guild, ok := guilds.GetGuild(gid)
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

		if err := guilds.ReviewApplication(ctx, guild.ID, app.ID, agent.ID, accepted, reviewReason); err != nil {
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

// agentGuildName creates a guild name from the founding agent's display name or name.
func agentGuildName(agent *agentprogression.Agent) string {
	name := agent.Name
	if agent.DisplayName != "" {
		name = agent.DisplayName
	}
	return name + "'s Guild"
}

// levelAbs returns the absolute value of an integer level difference.
func levelAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
