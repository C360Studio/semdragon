package agentprogression

import (
	"context"
	"sync"
	"time"

	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// peerReviewThreshold is the rating below which agents receive corrective
// feedback in their next quest prompt. Matches the threshold in questbridge.
const peerReviewThreshold = 3.0

// lockAgent returns a per-agent mutex from the agentLocks sync.Map,
// creating one on first access. This serializes read-modify-write cycles
// when both the quest watcher and review watcher update the same agent.
func (c *Component) lockAgent(agentID string) *sync.Mutex {
	v, _ := c.agentLocks.LoadOrStore(agentID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

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

	// Serialize with any concurrent review-watcher updates for this agent.
	agentMu := c.lockAgent(string(agentID))
	agentMu.Lock()
	defer agentMu.Unlock()

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

	fullAgent := AgentFromEntityState(agentEntity)
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
	var guildForStats *domain.Guild
	var guildRank domain.GuildRank
	if isGuildQuest {
		guildEntity, guildErr := c.graph.GetGuild(ctx, domain.GuildID(*quest.GuildPriority))
		if guildErr == nil {
			guild := domain.GuildFromEntityState(guildEntity)
			if guild != nil {
				guildForStats = guild
				for _, m := range guild.Members {
					if m.AgentID == domain.AgentID(agentID) {
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
	fullAgent.Status = domain.AgentIdle
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
		c.updateGuildStatsOnCompletion(ctx, guildForStats, domain.AgentID(agentID), award.TotalXP)
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

	// Release any orphaned agents that lost the CAS claim race but still have
	// status=on_quest pointing at this quest.
	c.releaseOrphanedAgents(ctx, quest.ID, domain.AgentID(agentID))
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

	// Serialize with any concurrent review-watcher updates for this agent.
	agentMu := c.lockAgent(string(agentID))
	agentMu.Lock()
	defer agentMu.Unlock()

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

	fullAgent := AgentFromEntityState(agentEntity)
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
	case domain.FailureTimeout:
		failType = FailureTimeout
	case domain.FailureAbandoned:
		failType = FailureAbandon
	case domain.FailureError:
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
		fullAgent.Status = domain.AgentCooldown
		cooldownUntil := time.Now().Add(penalty.CooldownDur)
		fullAgent.CooldownUntil = &cooldownUntil
	} else {
		fullAgent.Status = domain.AgentIdle
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
			guild := domain.GuildFromEntityState(guildEntity)
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

	// Release any orphaned agents that lost the CAS claim race but still have
	// status=on_quest pointing at this quest.
	c.releaseOrphanedAgents(ctx, quest.ID, domain.AgentID(agentID))
}

// releaseOrphanedAgents finds agents stuck in on_quest for a completed/failed quest
// and releases them back to idle. This handles agents that lost the CAS claim race
// but still set their own status to on_quest before the race was resolved.
func (c *Component) releaseOrphanedAgents(ctx context.Context, questID domain.QuestID, winnerAgentID domain.AgentID) {
	agents, err := c.graph.ListAgentsByPrefix(ctx, 200)
	if err != nil {
		c.logger.Error("failed to list agents for orphan cleanup", "error", err)
		return
	}

	for _, agentEntity := range agents {
		agent := AgentFromEntityState(&agentEntity)
		if agent == nil {
			continue
		}
		// Skip the winner — already handled
		if domain.AgentID(agent.ID) == winnerAgentID {
			continue
		}
		// Only release agents stuck on this specific quest
		if agent.Status != domain.AgentOnQuest || agent.CurrentQuest == nil || *agent.CurrentQuest != questID {
			continue
		}

		agent.Status = domain.AgentIdle
		agent.CurrentQuest = nil
		agent.UpdatedAt = time.Now()

		if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.status.idle"); err != nil {
			c.logger.Error("failed to release orphaned agent",
				"agent", agent.ID,
				"quest", questID,
				"error", err)
		} else {
			c.logger.Info("released orphaned agent from completed quest",
				"agent", agent.ID,
				"quest", questID)
		}
	}
}

// =============================================================================
// ENTITY STATE HELPERS
// =============================================================================

// questFromEntityStateTriples reconstructs a questboard.Quest from entity state triples.
func questFromEntityStateTriples(entity *semgraph.EntityState) *domain.Quest {
	if entity == nil {
		return nil
	}

	quest := &domain.Quest{
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
					quest.Verdict = &domain.BattleVerdict{}
				}
				quest.Verdict.Passed = v
			}
		case "quest.verdict.score":
			if v, ok := triple.Object.(float64); ok {
				if quest.Verdict == nil {
					quest.Verdict = &domain.BattleVerdict{}
				}
				quest.Verdict.QualityScore = v
			}
		case "quest.verdict.xp_awarded":
			if v, ok := triple.Object.(float64); ok {
				if quest.Verdict == nil {
					quest.Verdict = &domain.BattleVerdict{}
				}
				quest.Verdict.XPAwarded = int64(v)
			}
		case "quest.failure.reason":
			if v, ok := triple.Object.(string); ok {
				quest.FailureReason = v
			}
		case "quest.failure.type":
			if v, ok := triple.Object.(string); ok {
				quest.FailureType = domain.FailureType(v)
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
// PEER REVIEW HANDLER - Updates agent reputation stats when a review completes
// =============================================================================

// processPeerReviewUpdates watches PeerReview entity state changes from KV
// and updates the member agent's running reputation average on completion.
func (c *Component) processPeerReviewUpdates() {
	defer close(c.reviewWatchDone)

	// KV twofer bootstrap: the nil sentinel from WatchAll marks the end of
	// the historical replay. We skip all entries during bootstrap to avoid
	// re-applying running average increments for reviews that were already
	// counted before this instance started.
	bootstrapping := true

	for {
		select {
		case <-c.stopChan:
			return
		case entry, ok := <-c.reviewWatch.Updates():
			if !ok {
				return
			}
			if entry == nil {
				// Nil sentinel — bootstrap replay complete, begin live processing.
				bootstrapping = false
				continue
			}
			if entry.Operation() == jetstream.KeyValueDelete {
				continue
			}
			if bootstrapping {
				continue
			}
			c.handlePeerReviewStateChange(entry)
		}
	}
}

// handlePeerReviewStateChange processes a single PeerReview KV entry. It
// reacts only to reviews that have just transitioned to "completed" and updates
// the member agent's PeerReviewAvg/PeerReviewCount running average.
func (c *Component) handlePeerReviewStateChange(entry jetstream.KeyValueEntry) {
	if !c.running.Load() {
		return
	}

	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		return
	}

	// Only process completed reviews — partial/pending reviews carry no aggregate data.
	var status string
	for _, t := range entityState.Triples {
		if t.Predicate == "review.status.state" {
			if v, ok := t.Object.(string); ok {
				status = v
			}
			break
		}
	}
	if status != string(domain.PeerReviewCompleted) {
		return
	}

	// Reconstruct the full PeerReview entity to access leader avg rating and member ID.
	review := domain.PeerReviewFromEntityState(entityState)
	if review == nil || review.MemberID == "" {
		return
	}

	// Only react to leader-to-member reviews; DM-to-agent or member-to-leader
	// reviews use different rating scales and do not feed member reputation.
	if review.LeaderAvgRating == 0 {
		return
	}

	// Serialize with any concurrent quest-watcher updates for this agent.
	mu := c.lockAgent(string(review.MemberID))
	mu.Lock()
	defer mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agentEntity, err := c.graph.GetAgent(ctx, review.MemberID)
	if err != nil {
		c.logger.Error("failed to load agent for peer review stat update",
			"agent_id", review.MemberID,
			"review_id", review.ID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	agent := AgentFromEntityState(agentEntity)
	if agent == nil {
		c.logger.Error("agent reconstruction returned nil for peer review stat update",
			"agent_id", review.MemberID)
		c.errorsCount.Add(1)
		return
	}

	// Compute running averages: (oldAvg * oldCount + thisVal) / (oldCount + 1).
	oldCount := agent.Stats.PeerReviewCount
	newCount := oldCount + 1
	fc := float64(oldCount)
	fn := float64(newCount)

	agent.Stats.PeerReviewAvg = (agent.Stats.PeerReviewAvg*fc + review.LeaderAvgRating) / fn
	if review.LeaderReview != nil {
		agent.Stats.PeerReviewQ1Avg = (agent.Stats.PeerReviewQ1Avg*fc + float64(review.LeaderReview.Ratings.Q1)) / fn
		agent.Stats.PeerReviewQ2Avg = (agent.Stats.PeerReviewQ2Avg*fc + float64(review.LeaderReview.Ratings.Q2)) / fn
		agent.Stats.PeerReviewQ3Avg = (agent.Stats.PeerReviewQ3Avg*fc + float64(review.LeaderReview.Ratings.Q3)) / fn
	}
	agent.Stats.PeerReviewCount = newCount
	agent.UpdatedAt = time.Now()

	if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.progression.xp"); err != nil {
		c.logger.Error("failed to emit agent peer review stat update",
			"agent_id", review.MemberID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	c.messagesProcessed.Add(1)
	c.lastActivity.Store(time.Now())

	c.logger.Info("updated agent peer review stats",
		"agent_id", review.MemberID,
		"review_id", review.ID,
		"avg", agent.Stats.PeerReviewAvg,
		"q1_avg", agent.Stats.PeerReviewQ1Avg,
		"q2_avg", agent.Stats.PeerReviewQ2Avg,
		"q3_avg", agent.Stats.PeerReviewQ3Avg,
		"review_count", newCount,
		"below_threshold", agent.Stats.PeerReviewAvg < peerReviewThreshold)
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
func (c *Component) updateGuildStatsOnCompletion(ctx context.Context, guild *domain.Guild, agentID domain.AgentID, xpEarned int64) {
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
func (c *Component) updateGuildStatsOnFailure(ctx context.Context, guild *domain.Guild) {
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
