package agent_progression

import (
	"context"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/questboard"
)

// =============================================================================
// EVENT HANDLERS - Pure functions using fat events
// =============================================================================
// Handlers receive all context they need in the event payload.
// No fetching from storage - this keeps handlers pure and replayable.
// =============================================================================

// handleQuestCompleted processes quest completion events and awards XP.
// Uses the fat event pattern - AgentContext is embedded in the payload.
func (c *Component) handleQuestCompleted(ctx context.Context, payload questboard.QuestCompletedPayload) error {
	if !c.running.Load() {
		return nil
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Skip if no agent (shouldn't happen, but be defensive)
	if payload.AgentID == "" {
		return nil
	}

	// Extract agent context from fat event - no fetching needed
	agentCtx := payload.AgentContext

	// Determine if this is a guild quest
	isGuildQuest := payload.Quest.GuildPriority != nil

	// Build XP context from the fat event
	xpCtx := XPContext{
		Quest: payload.Quest,
		Agent: Agent{
			ID:        payload.AgentID,
			Level:     agentCtx.Level,
			XP:        agentCtx.XP,
			XPToLevel: agentCtx.XPToLevel,
			Tier:      agentCtx.Tier,
			Guilds:    agentCtx.Guilds,
			Stats: AgentStats{
				QuestsCompleted: agentCtx.Stats.QuestsCompleted,
				QuestsFailed:    agentCtx.Stats.QuestsFailed,
			},
		},
		BattleResult: &payload.Verdict,
		Duration:     payload.Duration,
		Streak:       agentCtx.Streak,
		IsGuildQuest: isGuildQuest,
		GuildRank:    agentCtx.GuildRank,
		Attempt:      payload.Quest.Attempts,
	}

	// Calculate XP - pure function
	award := c.xpEngine.CalculateXP(xpCtx)

	// Calculate new state
	xpBefore := agentCtx.XP
	levelBefore := agentCtx.Level

	// Create a temporary agent to apply XP (for level calculation)
	tempAgent := &Agent{
		Level:     agentCtx.Level,
		XP:        agentCtx.XP,
		XPToLevel: agentCtx.XPToLevel,
		Tier:      agentCtx.Tier,
	}
	levelEvent := c.xpEngine.ApplyXP(tempAgent, award.TotalXP)

	// Emit XP event - this is the output, not a side effect
	if err := SubjectAgentXP.Publish(ctx, c.deps.NATSClient, AgentXPPayload{
		AgentID:     payload.AgentID,
		QuestID:     payload.Quest.ID,
		Award:       &award,
		XPDelta:     award.TotalXP,
		XPBefore:    xpBefore,
		XPAfter:     tempAgent.XP,
		LevelBefore: levelBefore,
		LevelAfter:  tempAgent.Level,
		Timestamp:   time.Now(),
		Trace:       TraceInfo(payload.Trace),
	}); err != nil {
		c.logger.Error("failed to publish XP event", "error", err)
		c.errorsCount.Add(1)
	}

	// Emit level up event if applicable
	if levelEvent.Direction == "up" {
		if err := SubjectAgentLevelUp.Publish(ctx, c.deps.NATSClient, AgentLevelUpPayload{
			AgentID:   payload.AgentID,
			QuestID:   payload.Quest.ID,
			OldLevel:  levelEvent.OldLevel,
			NewLevel:  levelEvent.NewLevel,
			OldTier:   levelEvent.OldTier,
			NewTier:   levelEvent.NewTier,
			XPCurrent: tempAgent.XP,
			XPToLevel: tempAgent.XPToLevel,
			Timestamp: time.Now(),
			Trace:     TraceInfo(payload.Trace),
		}); err != nil {
			c.logger.Error("failed to publish level up event", "error", err)
		}
	}

	c.logger.Debug("processed quest completion",
		"agent", payload.AgentID,
		"quest", payload.Quest.ID,
		"xp_awarded", award.TotalXP,
		"new_level", tempAgent.Level)

	return nil
}

// handleQuestFailed processes quest failure events and applies XP penalties.
// Uses the fat event pattern - AgentContext is embedded in the payload.
func (c *Component) handleQuestFailed(ctx context.Context, payload questboard.QuestFailedPayload) error {
	if !c.running.Load() {
		return nil
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	if payload.AgentID == "" {
		return nil
	}

	// Extract agent context from fat event
	agentCtx := payload.AgentContext

	// Map questboard.FailureType to our FailureType
	var failType FailureType
	switch payload.FailType {
	case questboard.FailureTimeout:
		failType = FailureTimeout
	case questboard.FailureAbandoned:
		failType = FailureAbandon
	case questboard.FailureError:
		failType = FailureCatastrophic
	default:
		failType = FailureSoft
	}

	// Build penalty context from fat event
	penaltyCtx := PenaltyContext{
		Quest: payload.Quest,
		Agent: Agent{
			ID:        payload.AgentID,
			Level:     agentCtx.Level,
			XP:        agentCtx.XP,
			XPToLevel: agentCtx.XPToLevel,
			Tier:      agentCtx.Tier,
			Stats: AgentStats{
				QuestsCompleted: agentCtx.Stats.QuestsCompleted,
				QuestsFailed:    agentCtx.Stats.QuestsFailed,
			},
		},
		FailureType: failType,
		Attempt:     payload.Attempt,
	}

	// Calculate penalty - pure function
	penalty := c.xpEngine.CalculatePenalty(penaltyCtx)

	// Calculate new state
	xpBefore := agentCtx.XP
	levelBefore := agentCtx.Level

	tempAgent := &Agent{
		Level:     agentCtx.Level,
		XP:        agentCtx.XP,
		XPToLevel: agentCtx.XPToLevel,
		Tier:      agentCtx.Tier,
	}
	c.xpEngine.ApplyXP(tempAgent, -penalty.XPLost)

	// Emit XP penalty event
	if err := SubjectAgentXP.Publish(ctx, c.deps.NATSClient, AgentXPPayload{
		AgentID:     payload.AgentID,
		QuestID:     payload.Quest.ID,
		Penalty:     &penalty,
		XPDelta:     -penalty.XPLost,
		XPBefore:    xpBefore,
		XPAfter:     tempAgent.XP,
		LevelBefore: levelBefore,
		LevelAfter:  tempAgent.Level,
		Timestamp:   time.Now(),
		Trace:       TraceInfo(payload.Trace),
	}); err != nil {
		c.logger.Error("failed to publish XP penalty event", "error", err)
		c.errorsCount.Add(1)
	}

	// Emit cooldown event if applicable
	if penalty.CooldownDur > 0 {
		cooldownUntil := time.Now().Add(penalty.CooldownDur)
		if err := SubjectAgentCooldown.Publish(ctx, c.deps.NATSClient, AgentCooldownPayload{
			AgentID:       payload.AgentID,
			QuestID:       payload.Quest.ID,
			FailType:      failType,
			CooldownUntil: cooldownUntil,
			Duration:      penalty.CooldownDur,
			Timestamp:     time.Now(),
			Trace:         TraceInfo(payload.Trace),
		}); err != nil {
			c.logger.Error("failed to publish cooldown event", "error", err)
		}
	}

	c.logger.Debug("processed quest failure",
		"agent", payload.AgentID,
		"quest", payload.Quest.ID,
		"xp_lost", penalty.XPLost)

	return nil
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
