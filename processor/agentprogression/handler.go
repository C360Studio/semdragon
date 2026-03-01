package agentprogression

import (
	"context"
	"time"

	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/questboard"
)

// =============================================================================
// KV WATCH HANDLER - Entity-centric quest state monitoring
// =============================================================================
// Replaces the "fat events" pattern. Instead of receiving pre-built event payloads
// with embedded agent context, this processor watches quest entity state in KV
// and reads agent state directly from the graph when needed. This gives us:
// - Always-current agent state (not stale snapshots from event emission time)
// - No cross-processor payload coupling
// - Built-in at-least-once delivery via KV watch semantics
// =============================================================================

// processQuestWatchUpdates handles quest entity state changes from KV.
// Detects transitions to "completed" or "failed" status.
func (c *Component) processQuestWatchUpdates() {
	defer close(c.watchDoneCh)

	for {
		select {
		case <-c.stopChan:
			return
		case entry, ok := <-c.questWatch.Updates():
			if !ok {
				return
			}
			if entry == nil {
				continue // Initial sync complete
			}
			c.handleQuestStateChange(entry)
		}
	}
}

// handleQuestStateChange processes a quest entity state change from KV.
// Detects when a quest transitions to "completed" or "failed" and processes XP.
func (c *Component) handleQuestStateChange(entry jetstream.KeyValueEntry) {
	if !c.running.Load() {
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.questCache.Delete(entry.Key())
		return
	}

	// Decode entity state
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		return
	}

	// Extract current quest status from triples
	var currentStatus domain.QuestStatus
	for _, triple := range entityState.Triples {
		if triple.Predicate == "quest.status.state" {
			if v, ok := triple.Object.(string); ok {
				currentStatus = domain.QuestStatus(v)
			}
			break
		}
	}

	// Check for transition (state diffing against cache)
	prevStatus, hadPrev := c.questCache.Load(entry.Key())
	c.questCache.Store(entry.Key(), currentStatus)

	if !hadPrev || prevStatus == currentStatus {
		return // Not a status transition, or first time seeing this entity
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// React to terminal status transitions
	switch currentStatus {
	case domain.QuestCompleted:
		c.handleQuestCompletedFromKV(entityState)
	case domain.QuestFailed:
		c.handleQuestFailedFromKV(entityState)
	}
}

// handleQuestCompletedFromKV processes a quest that transitioned to completed.
// Reads agent state directly from KV for current values.
func (c *Component) handleQuestCompletedFromKV(entityState *semgraph.EntityState) {
	quest := questFromEntityStateTriples(entityState)
	if quest == nil {
		return
	}

	// Extract agent ID from quest
	if quest.ClaimedBy == nil {
		return
	}
	agentID := *quest.ClaimedBy

	// Read agent state directly from KV (always current, not a stale snapshot)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agentEntity, err := c.graph.GetAgent(ctx, domain.AgentID(agentID))
	if err != nil {
		c.logger.Error("failed to read agent state for XP calculation",
			"agent", agentID,
			"quest", quest.ID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	fullAgent := semdragons.AgentFromEntityState(agentEntity)
	if fullAgent == nil {
		c.logger.Error("failed to decode agent from entity state",
			"agent", agentID,
			"quest", quest.ID)
		c.errorsCount.Add(1)
		return
	}

	// Determine if this is a guild quest
	isGuildQuest := quest.GuildPriority != nil

	// Read guild entity to populate GuildRank for XP multiplier (B1)
	var guildForStats *semdragons.Guild
	var guildRank domain.GuildRank
	if isGuildQuest {
		guildEntity, guildErr := c.graph.GetGuild(ctx, domain.GuildID(*quest.GuildPriority))
		if guildErr == nil {
			guild := semdragons.GuildFromEntityState(guildEntity)
			if guild != nil {
				guildForStats = guild
				for _, m := range guild.Members {
					if m.AgentID == semdragons.AgentID(agentID) {
						guildRank = m.Rank
						break
					}
				}
			}
		} else {
			c.logger.Debug("could not read guild for XP multiplier",
				"guild_id", *quest.GuildPriority,
				"error", guildErr)
		}
	}

	// Build XP context from entity state (replaces fat event pattern)
	xpCtx := XPContext{
		Quest:        *quest,
		Agent:        Agent{Level: fullAgent.Level, XP: fullAgent.XP, XPToLevel: fullAgent.XPToLevel, Tier: fullAgent.Tier},
		Duration:     quest.Duration,
		IsGuildQuest: isGuildQuest,
		GuildRank:    guildRank,
		Attempt:      quest.Attempts,
	}

	// Extract verdict from quest entity if available
	if quest.Verdict != nil {
		xpCtx.BattleResult = quest.Verdict
	}

	// Calculate XP - pure function
	award := c.xpEngine.CalculateXP(xpCtx)

	// Apply XP to temp agent for level calculation
	tempAgent := &Agent{
		Level:     fullAgent.Level,
		XP:        fullAgent.XP,
		XPToLevel: fullAgent.XPToLevel,
		Tier:      fullAgent.Tier,
	}
	levelEvent := c.xpEngine.ApplyXP(tempAgent, award.TotalXP)

	// Copy XP results back to full agent (read-modify-write preserves all fields)
	fullAgent.Level = tempAgent.Level
	fullAgent.XP = tempAgent.XP
	fullAgent.XPToLevel = tempAgent.XPToLevel
	fullAgent.Tier = tempAgent.Tier
	fullAgent.Status = semdragons.AgentIdle
	fullAgent.CurrentQuest = nil
	fullAgent.Stats.QuestsCompleted++
	fullAgent.UpdatedAt = time.Now()

	// Write full agent entity (preserves name, skills, guilds, etc.)
	if err := c.graph.EmitEntityUpdate(ctx, fullAgent, "agent.progression.xp"); err != nil {
		c.logger.Error("failed to emit agent XP update", "error", err)
		c.errorsCount.Add(1)
	}

	// Update guild reputation and member contribution on quest completion (B2)
	if isGuildQuest && guildForStats != nil {
		c.updateGuildStatsOnCompletion(ctx, guildForStats, semdragons.AgentID(agentID), award.TotalXP)
	}

	// Emit level up event if applicable
	if levelEvent.Direction == "up" {
		c.logger.Info("agent leveled up",
			"agent", agentID,
			"quest", quest.ID,
			"old_level", levelEvent.OldLevel,
			"new_level", levelEvent.NewLevel)
	}

	c.logger.Debug("processed quest completion",
		"agent", agentID,
		"quest", quest.ID,
		"xp_awarded", award.TotalXP,
		"new_level", fullAgent.Level)
}

// handleQuestFailedFromKV processes a quest that transitioned to failed.
// Reads agent state directly from KV for current values.
func (c *Component) handleQuestFailedFromKV(entityState *semgraph.EntityState) {
	quest := questFromEntityStateTriples(entityState)
	if quest == nil {
		return
	}

	// Extract agent ID from quest
	// Note: on repost (retries remaining), ClaimedBy is cleared. We need the PREVIOUS state.
	// The quest entity still has failure.reason set even when reposted.
	if quest.ClaimedBy == nil {
		// Quest was reposted (retries remaining) — ClaimedBy was cleared.
		// We can't penalize the agent without knowing who they were.
		// The quest triples should still have quest.assignment.agent from the previous emit.
		c.logger.Debug("skipping failed quest with no agent (reposted)",
			"quest", quest.ID)
		return
	}
	agentID := *quest.ClaimedBy

	// Read agent state directly from KV
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agentEntity, err := c.graph.GetAgent(ctx, domain.AgentID(agentID))
	if err != nil {
		c.logger.Error("failed to read agent state for penalty calculation",
			"agent", agentID,
			"quest", quest.ID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	fullAgent := semdragons.AgentFromEntityState(agentEntity)
	if fullAgent == nil {
		c.logger.Error("failed to decode agent from entity state",
			"agent", agentID,
			"quest", quest.ID)
		c.errorsCount.Add(1)
		return
	}

	// Map failure type from quest entity
	var failType FailureType
	switch quest.FailureType {
	case questboard.FailureTimeout:
		failType = FailureTimeout
	case questboard.FailureAbandoned:
		failType = FailureAbandon
	case questboard.FailureError:
		failType = FailureCatastrophic
	default:
		failType = FailureSoft
	}

	// Build penalty context from entity state
	penaltyCtx := PenaltyContext{
		Quest: *quest,
		Agent: Agent{
			Level: fullAgent.Level, XP: fullAgent.XP, XPToLevel: fullAgent.XPToLevel, Tier: fullAgent.Tier,
			Stats: AgentStats{QuestsFailed: fullAgent.Stats.QuestsFailed},
		},
		FailureType: failType,
		Attempt:     quest.Attempts,
	}

	// Calculate penalty - pure function
	penalty := c.xpEngine.CalculatePenalty(penaltyCtx)

	// Apply penalty to temp agent
	tempAgent := &Agent{
		Level:     fullAgent.Level,
		XP:        fullAgent.XP,
		XPToLevel: fullAgent.XPToLevel,
		Tier:      fullAgent.Tier,
	}
	c.xpEngine.ApplyXP(tempAgent, -penalty.XPLost)

	// Copy penalty results back to full agent (read-modify-write preserves all fields)
	fullAgent.Level = tempAgent.Level
	fullAgent.XP = tempAgent.XP
	fullAgent.XPToLevel = tempAgent.XPToLevel
	fullAgent.Tier = tempAgent.Tier

	// Set status based on penalty cooldown
	if penalty.CooldownDur > 0 {
		fullAgent.Status = semdragons.AgentCooldown
		cooldownUntil := time.Now().Add(penalty.CooldownDur)
		fullAgent.CooldownUntil = &cooldownUntil
	} else {
		fullAgent.Status = semdragons.AgentIdle
	}
	fullAgent.CurrentQuest = nil
	fullAgent.Stats.QuestsFailed++
	fullAgent.UpdatedAt = time.Now()

	// Write full agent entity (preserves name, skills, guilds, etc.)
	if err := c.graph.EmitEntityUpdate(ctx, fullAgent, "agent.progression.xp"); err != nil {
		c.logger.Error("failed to emit agent XP penalty", "error", err)
		c.errorsCount.Add(1)
	}

	// Update guild stats on quest failure (B2)
	if quest.GuildPriority != nil {
		guildEntity, guildErr := c.graph.GetGuild(ctx, domain.GuildID(*quest.GuildPriority))
		if guildErr == nil {
			guild := semdragons.GuildFromEntityState(guildEntity)
			if guild != nil {
				c.updateGuildStatsOnFailure(ctx, guild)
			}
		}
	}

	c.logger.Debug("processed quest failure",
		"agent", agentID,
		"quest", quest.ID,
		"xp_lost", penalty.XPLost,
		"new_status", fullAgent.Status)
}

// =============================================================================
// ENTITY STATE HELPERS
// =============================================================================

// questFromEntityStateTriples reconstructs a questboard.Quest from entity state triples.
func questFromEntityStateTriples(entity *semgraph.EntityState) *questboard.Quest {
	if entity == nil {
		return nil
	}

	quest := &questboard.Quest{
		ID: domain.QuestID(entity.ID),
	}

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		case "quest.identity.title":
			if v, ok := triple.Object.(string); ok {
				quest.Title = v
			}
		case "quest.identity.description":
			if v, ok := triple.Object.(string); ok {
				quest.Description = v
			}
		case "quest.status.state":
			if v, ok := triple.Object.(string); ok {
				quest.Status = domain.QuestStatus(v)
			}
		case "quest.difficulty.level":
			if v, ok := triple.Object.(float64); ok {
				quest.Difficulty = domain.QuestDifficulty(int(v))
			}
		case "quest.xp.base":
			if v, ok := triple.Object.(float64); ok {
				quest.BaseXP = int64(v)
			}
		case "quest.assignment.agent":
			if v, ok := triple.Object.(string); ok {
				agentID := domain.AgentID(v)
				quest.ClaimedBy = &agentID
			}
		case "quest.priority.guild":
			if v, ok := triple.Object.(string); ok {
				guildID := domain.GuildID(v)
				quest.GuildPriority = &guildID
			}
		case "quest.attempts.current":
			if v, ok := triple.Object.(float64); ok {
				quest.Attempts = int(v)
			}
		case "quest.attempts.max":
			if v, ok := triple.Object.(float64); ok {
				quest.MaxAttempts = int(v)
			}
		case "quest.verdict.passed":
			if v, ok := triple.Object.(bool); ok {
				if quest.Verdict == nil {
					quest.Verdict = &questboard.BattleVerdict{}
				}
				quest.Verdict.Passed = v
			}
		case "quest.verdict.score":
			if v, ok := triple.Object.(float64); ok {
				if quest.Verdict == nil {
					quest.Verdict = &questboard.BattleVerdict{}
				}
				quest.Verdict.QualityScore = v
			}
		case "quest.verdict.xp_awarded":
			if v, ok := triple.Object.(float64); ok {
				if quest.Verdict == nil {
					quest.Verdict = &questboard.BattleVerdict{}
				}
				quest.Verdict.XPAwarded = int64(v)
			}
		case "quest.failure.reason":
			if v, ok := triple.Object.(string); ok {
				quest.FailureReason = v
			}
		case "quest.failure.type":
			if v, ok := triple.Object.(string); ok {
				quest.FailureType = questboard.FailureType(v)
			}
		case "quest.duration":
			if v, ok := triple.Object.(string); ok {
				if d, err := time.ParseDuration(v); err == nil {
					quest.Duration = d
				}
			}
		}
	}

	return quest
}

// =============================================================================
// PUBLIC API - For direct invocation (testing, CLI, etc.)
// =============================================================================

// CalculateXP exposes the underlying XP calculation for external use.
func (c *Component) CalculateXP(xpCtx XPContext) XPAward {
	return c.xpEngine.CalculateXP(xpCtx)
}

// CalculatePenalty exposes penalty calculation for external use.
func (c *Component) CalculatePenalty(penaltyCtx PenaltyContext) XPPenalty {
	return c.xpEngine.CalculatePenalty(penaltyCtx)
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}

// =============================================================================
// GUILD STATS UPDATES (B2)
// =============================================================================

// updateGuildStatsOnCompletion increments guild reputation and member contribution
// when a guild quest is completed successfully.
func (c *Component) updateGuildStatsOnCompletion(ctx context.Context, guild *semdragons.Guild, agentID semdragons.AgentID, xpEarned int64) {
	guild.QuestsHandled++
	if guild.QuestsHandled > 0 {
		guild.SuccessRate = float64(guild.QuestsHandled-guild.QuestsFailed) / float64(guild.QuestsHandled)
	}

	// Nudge reputation toward success rate (weighted average favoring recent performance)
	guild.Reputation = guild.Reputation*0.9 + guild.SuccessRate*0.1

	// Update member contribution
	for i := range guild.Members {
		if guild.Members[i].AgentID == agentID {
			guild.Members[i].Contribution += float64(xpEarned)
			break
		}
	}

	// Persist updated guild to KV
	if err := c.graph.EmitEntity(ctx, guild, "guild.stats.updated"); err != nil {
		c.logger.Error("failed to persist guild stats update",
			"guild_id", guild.ID,
			"error", err)
		c.errorsCount.Add(1)
	}
}

// updateGuildStatsOnFailure increments failure counters and recalculates success rate
// when a guild quest fails.
func (c *Component) updateGuildStatsOnFailure(ctx context.Context, guild *semdragons.Guild) {
	guild.QuestsHandled++
	guild.QuestsFailed++
	if guild.QuestsHandled > 0 {
		guild.SuccessRate = float64(guild.QuestsHandled-guild.QuestsFailed) / float64(guild.QuestsHandled)
	}

	// Nudge reputation toward success rate
	guild.Reputation = guild.Reputation*0.9 + guild.SuccessRate*0.1

	// Persist updated guild to KV
	if err := c.graph.EmitEntity(ctx, guild, "guild.stats.updated"); err != nil {
		c.logger.Error("failed to persist guild stats update on failure",
			"guild_id", guild.ID,
			"error", err)
		c.errorsCount.Add(1)
	}
}
