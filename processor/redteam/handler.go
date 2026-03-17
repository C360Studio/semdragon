package redteam

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// processQuestWatchUpdates handles quest entity state changes from KV.
// Detects two transitions:
//   - Normal quest → in_review: posts a red-team review quest.
//   - Red-team quest → completed/failed: signals findings to original quest.
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
				continue
			}
			c.handleQuestStateChange(entry)
		}
	}
}

// handleQuestStateChange processes a single KV update.
func (c *Component) handleQuestStateChange(entry jetstream.KeyValueEntry) {
	if !c.running.Load() {
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.questCache.Delete(entry.Key())
		return
	}

	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		return
	}

	// Extract status and quest type from triples.
	var currentStatus domain.QuestStatus
	var questType domain.QuestType
	var redTeamTarget string
	for _, triple := range entityState.Triples {
		switch triple.Predicate {
		case "quest.status.state":
			if v, ok := triple.Object.(string); ok {
				currentStatus = domain.QuestStatus(v)
			}
		case "quest.classification.type":
			if v, ok := triple.Object.(string); ok {
				questType = domain.QuestType(v)
			}
		case "quest.classification.red_team_target":
			if v, ok := triple.Object.(string); ok {
				redTeamTarget = v
			}
		}
	}

	// State diffing against cache.
	prevStatus, hadPrev := c.questCache.Load(entry.Key())
	c.questCache.Store(entry.Key(), currentStatus)

	if !hadPrev || prevStatus == currentStatus {
		return
	}

	c.lastActivity.Store(time.Now())

	// Route based on quest type and transition.
	switch {
	case questType == domain.QuestTypeNormal && currentStatus == domain.QuestInReview:
		// Normal quest entered review — post a red-team quest for it.
		quest := domain.QuestFromEntityState(entityState)
		if quest == nil {
			return
		}
		c.handleNormalQuestInReview(quest)

	case questType == domain.QuestTypeRedTeam && (currentStatus == domain.QuestCompleted || currentStatus == domain.QuestFailed):
		// Red-team quest finished — attach findings to original quest.
		quest := domain.QuestFromEntityState(entityState)
		if quest == nil {
			return
		}
		c.handleRedTeamQuestFinished(quest, redTeamTarget)
	}
}

// handleNormalQuestInReview posts a red-team review quest for a submitted quest.
func (c *Component) handleNormalQuestInReview(quest *domain.Quest) {
	// Eligibility checks.
	if !quest.Constraints.RequireReview {
		return
	}
	if quest.Difficulty < c.config.MinDifficulty {
		c.logger.Debug("quest below min difficulty for red-team", "quest", quest.ID, "difficulty", quest.Difficulty)
		c.emitSkipped(quest.ID, "below_min_difficulty")
		return
	}
	// Skip sub-quests — they're reviewed by party lead via questdagexec.
	if quest.ParentQuest != nil {
		return
	}
	// Don't red-team red-team quests (prevent recursion).
	if quest.QuestType == domain.QuestTypeRedTeam {
		return
	}

	qb := c.resolveQuestBoard()
	if qb == nil {
		c.logger.Warn("questboard not available, cannot post red-team quest", "quest", quest.ID)
		c.emitSkipped(quest.ID, "no_questboard")
		return
	}

	// Find the implementing agent's guild for cross-guild routing.
	var blueTeamGuild *domain.GuildID
	if quest.ClaimedBy != nil {
		blueTeamGuild = c.resolveAgentGuild(*quest.ClaimedBy)
	}

	// Choose a guild priority for the red-team quest (cross-guild if possible).
	var guildPriority *domain.GuildID
	if c.config.PreferCrossGuild && blueTeamGuild != nil {
		guildPriority = c.pickCrossGuild(*blueTeamGuild)
	}

	// Build red-team quest.
	rtQuest := domain.Quest{
		QuestType:     domain.QuestTypeRedTeam,
		RedTeamTarget: &quest.ID,
		Title:         fmt.Sprintf("Red-Team Review: %s", quest.Title),
		Description:   fmt.Sprintf("Review and red-team the output of quest %q. Find strengths, risks, and improvement suggestions.", quest.Title),
		Difficulty:    quest.Difficulty,
		RequiredSkills: quest.RequiredSkills,
		GuildPriority: guildPriority,
		Input: map[string]any{
			"target_quest_id": string(quest.ID),
			"target_title":    quest.Title,
			"target_output":   quest.Output,
			"acceptance":      quest.Acceptance,
			"required_skills": quest.RequiredSkills,
		},
		Constraints: domain.QuestConstraints{
			RequireReview: false, // Red-team quests don't themselves go through boss battle
			MaxDuration:   c.config.ExecutionTimeout(),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.config.ExecutionTimeout())

	posted, err := qb.PostQuest(ctx, rtQuest)
	if err != nil {
		cancel()
		c.logger.Error("failed to post red-team quest", "quest", quest.ID, "error", err)
		c.errorsCount.Add(1)
		c.emitSkipped(quest.ID, "post_failed")
		return
	}

	c.reviewsPosted.Add(1)

	// Write the red-team quest ID onto the original quest entity so bossbattle
	// can look it up by ID instead of scanning all quests.
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()
	originalEntity, readErr := c.graph.GetQuest(writeCtx, quest.ID)
	if readErr == nil && originalEntity != nil {
		origQuest := domain.QuestFromEntityState(originalEntity)
		if origQuest != nil {
			origQuest.RedTeamQuestID = &posted.ID
			if err := c.graph.EmitEntityUpdate(writeCtx, origQuest, domain.PredicateRedTeamPosted); err != nil {
				c.logger.Warn("failed to write red-team quest ID on original", "error", err)
			}
		}
	}

	c.logger.Info("posted red-team quest",
		"original", quest.ID,
		"red_team", posted.ID,
		"guild_priority", guildPriority)

	// Track pending review with timeout.
	pending := &pendingRedTeam{
		RedTeamQuestID: posted.ID,
		OriginalQuest:  quest.ID,
		PostedAt:       time.Now(),
		cancel:         cancel,
	}
	c.pendingReviews.Store(quest.ID, pending)

	// Start timeout watcher.
	go c.watchTimeout(ctx, cancel, quest.ID, posted.ID)
}

// handleRedTeamQuestFinished processes a completed/failed red-team quest.
func (c *Component) handleRedTeamQuestFinished(rtQuest *domain.Quest, targetQuestIDStr string) {
	if targetQuestIDStr == "" && rtQuest.RedTeamTarget != nil {
		targetQuestIDStr = string(*rtQuest.RedTeamTarget)
	}
	if targetQuestIDStr == "" {
		c.logger.Warn("red-team quest has no target", "quest", rtQuest.ID)
		return
	}

	targetQuestID := domain.QuestID(targetQuestIDStr)

	// Clean up pending tracking.
	if pending, ok := c.pendingReviews.LoadAndDelete(targetQuestID); ok {
		if p, ok := pending.(*pendingRedTeam); ok && p.cancel != nil {
			p.cancel()
		}
	}

	if rtQuest.Status == domain.QuestCompleted && rtQuest.Output != nil {
		// Attach findings to the original quest entity.
		c.attachFindings(targetQuestID, rtQuest)

		// Extract lessons from findings and store on guild entities.
		findingsCtx, findingsCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer findingsCancel()
		originalEntity, _ := c.graph.GetQuest(findingsCtx, targetQuestID)
		if originalEntity != nil {
			originalQuest := domain.QuestFromEntityState(originalEntity)
			if originalQuest != nil {
				c.extractAndStoreLessons(findingsCtx, originalQuest, rtQuest)
			}
		}

		c.reviewsDone.Add(1)
		c.logger.Info("red-team review completed",
			"original", targetQuestID,
			"red_team", rtQuest.ID)
	} else {
		// Red-team failed — skip gracefully.
		c.emitSkipped(targetQuestID, "red_team_failed")
		c.reviewsSkipped.Add(1)
		c.logger.Info("red-team review failed, skipping",
			"original", targetQuestID,
			"red_team", rtQuest.ID)
	}
}

// watchTimeout monitors the execution timeout for a red-team quest.
// If the context expires before the red-team quest completes, it emits a skip signal.
func (c *Component) watchTimeout(ctx context.Context, cancel context.CancelFunc, originalID, redTeamID domain.QuestID) {
	defer cancel()

	select {
	case <-ctx.Done():
		// Check if this was already handled (completed or cancelled by stop).
		if _, still := c.pendingReviews.LoadAndDelete(originalID); still {
			c.reviewsSkipped.Add(1)
			c.logger.Info("red-team review timed out",
				"original", originalID,
				"red_team", redTeamID)
			c.emitSkipped(originalID, "timeout")
		}
	case <-c.stopChan:
		return
	}
}

// attachFindings writes the red-team findings to the original quest entity
// and emits the redteam.lifecycle.completed predicate.
func (c *Component) attachFindings(targetQuestID domain.QuestID, rtQuest *domain.Quest) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load the original quest to update it.
	entityState, err := c.graph.GetQuest(ctx, targetQuestID)
	if err != nil {
		c.logger.Error("failed to load target quest for findings", "quest", targetQuestID, "error", err)
		c.errorsCount.Add(1)
		return
	}

	quest := domain.QuestFromEntityState(entityState)
	if quest == nil {
		c.logger.Error("failed to reconstruct target quest", "quest", targetQuestID)
		return
	}

	// Log the red-team output for observability.
	c.logger.Info("attaching red-team findings to target quest",
		"target", targetQuestID,
		"red_team", rtQuest.ID,
		"has_output", rtQuest.Output != nil)

	// Set the red-team status triple so bossbattle's KV watcher can detect it.
	quest.RedTeamStatus = "completed"
	if err := c.graph.EmitEntityUpdate(ctx, quest, domain.PredicateRedTeamCompleted); err != nil {
		c.logger.Error("failed to emit red-team completed", "quest", targetQuestID, "error", err)
		c.errorsCount.Add(1)
	}
}

// emitSkipped writes a skip signal on the original quest so bossbattle proceeds.
func (c *Component) emitSkipped(questID domain.QuestID, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entityState, err := c.graph.GetQuest(ctx, questID)
	if err != nil {
		c.logger.Error("failed to load quest for skip signal", "quest", questID, "error", err)
		return
	}

	quest := domain.QuestFromEntityState(entityState)
	if quest == nil {
		return
	}

	quest.RedTeamStatus = "skipped"
	c.logger.Debug("emitting red-team skip", "quest", questID, "reason", reason)
	if err := c.graph.EmitEntityUpdate(ctx, quest, domain.PredicateRedTeamSkipped); err != nil {
		c.logger.Error("failed to emit red-team skipped", "quest", questID, "error", err)
	}
}

// resolveAgentGuild loads an agent and returns their guild ID, or nil.
func (c *Component) resolveAgentGuild(agentID domain.AgentID) *domain.GuildID {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entityState, err := c.graph.GetAgent(ctx, agentID)
	if err != nil || entityState == nil {
		return nil
	}

	agent := agentprogression.AgentFromEntityState(entityState)
	if agent == nil || agent.Guild == "" {
		return nil
	}
	return &agent.Guild
}

// pickCrossGuild finds an active guild that is different from the blue team's guild.
// Returns the guild with the highest reputation as the priority target.
// Returns nil if no suitable guild exists (early game, no guilds formed yet).
func (c *Component) pickCrossGuild(blueTeamGuild domain.GuildID) *domain.GuildID {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entities, err := c.graph.ListGuildsByPrefix(ctx, 100)
	if err != nil {
		c.logger.Debug("failed to list guilds for cross-guild routing", "error", err)
		return nil
	}

	var bestGuild *domain.GuildID
	var bestReputation float64

	for i := range entities {
		guild := domain.GuildFromEntityState(&entities[i])
		if guild == nil || guild.Status != domain.GuildActive {
			continue
		}
		if guild.ID == blueTeamGuild {
			continue // Skip the blue team's guild.
		}
		if guild.Reputation > bestReputation {
			bestReputation = guild.Reputation
			id := guild.ID
			bestGuild = &id
		}
	}

	return bestGuild
}
