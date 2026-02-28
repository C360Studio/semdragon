package xpengine

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/c360studio/semstreams/pkg/errs"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// EVENT HANDLERS
// =============================================================================

// handleQuestCompleted processes quest completion events and awards XP.
// The typed Subject.Subscribe handles unmarshaling, so we receive the payload directly.
func (c *Component) handleQuestCompleted(ctx context.Context, payload semdragons.QuestCompletedPayload) error {
	if !c.running.Load() {
		return nil
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Skip if no agent (shouldn't happen, but be defensive)
	if payload.AgentID == "" {
		return nil
	}

	// Load agent
	agentEntity, err := c.graph.GetAgent(ctx, payload.AgentID)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to load agent", "agent", payload.AgentID, "error", err)
		return nil // Don't return error to avoid NATS redelivery for data issues
	}
	agent := semdragons.AgentFromEntityState(agentEntity)
	if agent == nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to convert agent entity", "agent", payload.AgentID)
		return nil
	}
	agentInstance := semdragons.ExtractInstance(string(payload.AgentID))

	// Get streak
	kv, err := c.graph.KVBucket(ctx)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to get KV bucket", "error", err)
		return nil
	}
	streak := 0
	streakKey := "agent.streak." + agentInstance
	if entry, err := kv.Get(ctx, streakKey); err == nil {
		if len(entry.Value()) > 0 {
			var val int
			if err := json.Unmarshal(entry.Value(), &val); err == nil {
				streak = val
			}
		}
	}
	streak++ // Increment for this success

	// Determine if this is a guild quest
	isGuildQuest := payload.Quest.GuildPriority != nil
	var guildRank semdragons.GuildRank
	if isGuildQuest && len(agent.Guilds) > 0 {
		// Get rank in the priority guild
		guildEntity, guildErr := c.graph.GetGuild(ctx, *payload.Quest.GuildPriority)
		if guildErr == nil {
			guild := semdragons.GuildFromEntityState(guildEntity)
			if guild != nil {
				for _, m := range guild.Members {
					if m.AgentID == payload.AgentID {
						guildRank = m.Rank
						break
					}
				}
			}
		}
	}

	// Build XP context
	xpCtx := semdragons.XPContext{
		Quest:        payload.Quest,
		Agent:        *agent,
		BattleResult: payload.Verdict,
		Duration:     payload.Duration,
		Streak:       streak,
		IsGuildQuest: isGuildQuest,
		GuildRank:    guildRank,
		Attempt:      payload.Quest.Attempts,
	}

	// Calculate XP
	award := c.xpEngine.CalculateXP(xpCtx)

	// Apply XP
	xpBefore := agent.XP
	levelBefore := agent.Level
	levelEvent := c.xpEngine.ApplyXP(agent, award.TotalXP)

	// Update streak
	streakData, _ := json.Marshal(streak)
	if _, err := kv.Put(ctx, streakKey, streakData); err != nil {
		c.logger.Debug("failed to save streak", "agent", payload.AgentID, "error", err)
	}

	// Persist agent
	if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.updated"); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to save agent", "agent", payload.AgentID, "error", err)
		return nil // Don't return error to avoid NATS redelivery for data issues
	}

	// Emit XP event
	c.events.PublishAgentXP(ctx, semdragons.AgentXPPayload{
		AgentID:     payload.AgentID,
		QuestID:     payload.Quest.ID,
		Award:       &award,
		XPDelta:     award.TotalXP,
		XPBefore:    xpBefore,
		XPAfter:     agent.XP,
		LevelBefore: levelBefore,
		LevelAfter:  agent.Level,
		Timestamp:   time.Now(),
		Trace:       payload.Trace,
	})

	// Emit level up event if applicable
	if levelEvent.Direction == "up" {
		c.events.PublishAgentLevelUp(ctx, semdragons.AgentLevelPayload{
			AgentID:   payload.AgentID,
			QuestID:   payload.Quest.ID,
			OldLevel:  levelEvent.OldLevel,
			NewLevel:  levelEvent.NewLevel,
			OldTier:   levelEvent.OldTier,
			NewTier:   levelEvent.NewTier,
			XPCurrent: agent.XP,
			XPToLevel: agent.XPToLevel,
			Timestamp: time.Now(),
			Trace:     payload.Trace,
		})
	}

	c.logger.Debug("processed quest completion",
		"agent", payload.AgentID,
		"quest", payload.Quest.ID,
		"xp_awarded", award.TotalXP,
		"new_level", agent.Level)

	return nil
}

// =============================================================================
// PUBLIC API
// =============================================================================

// ProcessFailure handles quest failure events and applies XP penalties.
func (c *Component) ProcessFailure(ctx context.Context, agentID semdragons.AgentID, quest semdragons.Quest, failType semdragons.FailureType) (*semdragons.XPPenalty, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Load agent
	agentEntity, err := c.graph.GetAgent(ctx, agentID)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "XPEngine", "ProcessFailure", "load agent")
	}
	agent := semdragons.AgentFromEntityState(agentEntity)
	if agent == nil {
		c.errorsCount.Add(1)
		return nil, errors.New("failed to convert agent entity")
	}
	agentInstance := semdragons.ExtractInstance(string(agentID))

	// Build penalty context
	penaltyCtx := semdragons.PenaltyContext{
		Quest:       quest,
		Agent:       *agent,
		FailureType: failType,
		Attempt:     quest.Attempts,
	}

	// Calculate penalty
	penalty := c.xpEngine.CalculatePenalty(penaltyCtx)

	// Apply penalty
	xpBefore := agent.XP
	levelBefore := agent.Level
	c.xpEngine.ApplyXP(agent, -penalty.XPLost)

	// Apply cooldown
	if penalty.CooldownDur > 0 {
		cooldownUntil := time.Now().Add(penalty.CooldownDur)
		agent.CooldownUntil = &cooldownUntil
	}

	// Reset streak
	kv, err := c.graph.KVBucket(ctx)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "XPEngine", "ProcessFailure", "get KV bucket")
	}
	streakKey := "agent.streak." + agentInstance
	_ = kv.Delete(ctx, streakKey)

	// Handle permadeath
	if penalty.Permadeath {
		agent.Status = semdragons.AgentRetired
		agent.DeathCount++
	}

	// Persist agent
	if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.updated"); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "XPEngine", "ProcessFailure", "save agent")
	}

	// Emit XP event
	c.events.PublishAgentXP(ctx, semdragons.AgentXPPayload{
		AgentID:     agentID,
		QuestID:     quest.ID,
		Penalty:     &penalty,
		XPDelta:     -penalty.XPLost,
		XPBefore:    xpBefore,
		XPAfter:     agent.XP,
		LevelBefore: levelBefore,
		LevelAfter:  agent.Level,
		Timestamp:   time.Now(),
	})

	// Emit cooldown event if applicable
	if penalty.CooldownDur > 0 {
		c.events.PublishAgentCooldown(ctx, semdragons.AgentCooldownPayload{
			AgentID:       agentID,
			QuestID:       quest.ID,
			FailType:      failType,
			CooldownUntil: *agent.CooldownUntil,
			Duration:      penalty.CooldownDur,
			Timestamp:     time.Now(),
		})
	}

	return &penalty, nil
}

// CalculateXP exposes the underlying XP calculation for external use.
func (c *Component) CalculateXP(ctx semdragons.XPContext) semdragons.XPAward {
	return c.xpEngine.CalculateXP(ctx)
}

// Graph returns the underlying graph client for external access.
func (c *Component) Graph() *semdragons.GraphClient {
	return c.graph
}

// createGraphClient creates the graph client for the component.
func (c *Component) createGraphClient(_ context.Context) error {
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	return nil
}
