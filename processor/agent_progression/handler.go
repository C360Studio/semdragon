package agent_progression

import (
	"context"
	"time"

	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
	graphclient "github.com/c360studio/semdragons/graph"
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
	entityState, err := graphclient.DecodeEntityState(entry)
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

	agent := agentFromEntityState(agentEntity)
	if agent == nil {
		c.logger.Error("failed to decode agent from entity state",
			"agent", agentID,
			"quest", quest.ID)
		c.errorsCount.Add(1)
		return
	}

	// Determine if this is a guild quest
	isGuildQuest := quest.GuildPriority != nil

	// Build XP context from entity state (replaces fat event pattern)
	xpCtx := XPContext{
		Quest: *quest,
		Agent: *agent,
		Duration: quest.Duration,
		IsGuildQuest: isGuildQuest,
		Attempt:      quest.Attempts,
	}

	// Extract verdict from quest entity if available
	if quest.Verdict != nil {
		xpCtx.BattleResult = quest.Verdict
	}

	// Calculate XP - pure function
	award := c.xpEngine.CalculateXP(xpCtx)

	// Apply XP to a temp agent for level calculation
	tempAgent := &Agent{
		Level:     agent.Level,
		XP:        agent.XP,
		XPToLevel: agent.XPToLevel,
		Tier:      agent.Tier,
	}
	levelEvent := c.xpEngine.ApplyXP(tempAgent, award.TotalXP)

	// Emit XP event via KV (entity-centric — update agent entity state)
	if err := c.emitAgentXPUpdate(ctx, agentID, quest.ID, &award, nil, tempAgent); err != nil {
		c.logger.Error("failed to emit agent XP update", "error", err)
		c.errorsCount.Add(1)
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
		"new_level", tempAgent.Level)
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

	agent := agentFromEntityState(agentEntity)
	if agent == nil {
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
		Quest:       *quest,
		Agent:       *agent,
		FailureType: failType,
		Attempt:     quest.Attempts,
	}

	// Calculate penalty - pure function
	penalty := c.xpEngine.CalculatePenalty(penaltyCtx)

	// Apply penalty to temp agent
	tempAgent := &Agent{
		Level:     agent.Level,
		XP:        agent.XP,
		XPToLevel: agent.XPToLevel,
		Tier:      agent.Tier,
	}
	c.xpEngine.ApplyXP(tempAgent, -penalty.XPLost)

	// Emit XP penalty event via KV (entity-centric)
	if err := c.emitAgentXPUpdate(ctx, agentID, quest.ID, nil, &penalty, tempAgent); err != nil {
		c.logger.Error("failed to emit agent XP penalty", "error", err)
		c.errorsCount.Add(1)
	}

	c.logger.Debug("processed quest failure",
		"agent", agentID,
		"quest", quest.ID,
		"xp_lost", penalty.XPLost)
}

// emitAgentXPUpdate publishes the updated agent XP state to the graph.
// In the entity-centric model, this is a KV write to ENTITY_STATES.
func (c *Component) emitAgentXPUpdate(ctx context.Context, agentID domain.AgentID, questID domain.QuestID, award *XPAward, penalty *XPPenalty, agent *Agent) error {
	// Build XP payload as a Graphable entity for emission
	payload := &AgentXPPayload{
		AgentID:     agentID,
		QuestID:     questID,
		Award:       award,
		Penalty:     penalty,
		XPAfter:     agent.XP,
		LevelAfter:  agent.Level,
		Timestamp:   time.Now(),
	}

	if award != nil {
		payload.XPDelta = award.TotalXP
	}
	if penalty != nil {
		payload.XPDelta = -penalty.XPLost
	}

	return c.graph.PutEntityState(ctx, payload, "agent.progression.xp")
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

// agentFromEntityState reconstructs an Agent from entity state triples.
func agentFromEntityState(entity *semgraph.EntityState) *Agent {
	if entity == nil {
		return nil
	}

	agent := &Agent{
		ID: domain.AgentID(entity.ID),
	}

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		case "agent.progression.level":
			if v, ok := triple.Object.(float64); ok {
				agent.Level = int(v)
			}
		case "agent.progression.xp.current":
			if v, ok := triple.Object.(float64); ok {
				agent.XP = int64(v)
			}
		case "agent.progression.xp.to_level":
			if v, ok := triple.Object.(float64); ok {
				agent.XPToLevel = int64(v)
			}
		case "agent.progression.tier":
			if v, ok := triple.Object.(float64); ok {
				agent.Tier = domain.TrustTier(int(v))
			}
		}
	}

	// Derive tier from level if not set
	if agent.Tier == 0 && agent.Level > 0 {
		agent.Tier = domain.TierFromLevel(agent.Level)
	}

	return agent
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
