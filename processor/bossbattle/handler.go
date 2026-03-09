package bossbattle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// KV WATCH HANDLER - Entity-centric quest state monitoring
// =============================================================================

// processQuestWatchUpdates handles quest entity state changes from KV.
// Detects transitions to "in_review" status and triggers battles.
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
// Detects when a quest transitions to "in_review" and starts a battle.
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
	var reviewLevel domain.ReviewLevel
	for _, triple := range entityState.Triples {
		switch triple.Predicate {
		case "quest.status.state":
			if v, ok := triple.Object.(string); ok {
				currentStatus = domain.QuestStatus(v)
			}
		case "quest.review.level":
			if v, ok := triple.Object.(float64); ok {
				reviewLevel = domain.ReviewLevel(int(v))
			}
		}
	}

	// Check for transition to in_review (state diffing against cache)
	prevStatus, hadPrev := c.questCache.Load(entry.Key())
	c.questCache.Store(entry.Key(), currentStatus)

	if !hadPrev || prevStatus == currentStatus {
		return // Not a status transition, or first time seeing this entity
	}

	// Only react to transitions INTO in_review
	if currentStatus != domain.QuestInReview {
		return
	}

	c.lastActivity.Store(time.Now())

	// Reconstruct quest from entity state
	quest := domain.QuestFromEntityState(entityState)
	if quest == nil {
		c.logger.Warn("failed to reconstruct quest from entity state", "quest", entry.Key())
		return
	}

	// Sub-quests in a party DAG are reviewed by the party lead via questdagexec,
	// not by the automated boss battle judge. Skip to avoid racing.
	if quest.ParentQuest != nil {
		c.logger.Debug("skipping boss battle for DAG sub-quest (lead reviews instead)",
			"quest", quest.ID, "parent", *quest.ParentQuest)
		return
	}

	// Start battle with background context (watcher goroutine context)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Auto-pass: only when difficulty is in the domain's auto-pass list AND
	// review level is explicitly set to ReviewAuto.
	if c.shouldAutoPass(quest) && reviewLevel == domain.ReviewAuto {
		c.autoPassQuest(ctx, quest)
		return
	}

	battle, err := c.StartBattle(ctx, quest, quest.Output)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to start battle",
			"quest", quest.ID,
			"error", err)
		return
	}

	// Set agent to in_battle
	if quest.ClaimedBy != nil {
		agentEntity, agentErr := c.graph.GetAgent(ctx, domain.AgentID(*quest.ClaimedBy))
		if agentErr == nil {
			agent := agentprogression.AgentFromEntityState(agentEntity)
			if agent != nil {
				agent.Status = domain.AgentInBattle
				agent.UpdatedAt = time.Now()
				if writeErr := c.graph.EmitEntityUpdate(ctx, agent, "agent.status.in_battle"); writeErr != nil {
					c.errorsCount.Add(1)
					c.logger.Error("failed to set agent in_battle", "error", writeErr)
				}
			}
		}
	}

	c.logger.Debug("started battle for submitted quest",
		"quest", quest.ID,
		"battle", battle.ID)
}

// =============================================================================
// BATTLE OPERATIONS
// =============================================================================

// StartBattle initiates a boss battle for a quest.
func (c *Component) StartBattle(ctx context.Context, quest *domain.Quest, output any) (*BossBattle, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	// Generate battle ID
	battleID := domain.BattleID(c.boardConfig.EntityID("battle", c.generateID()))

	// Build battle from quest review level
	battle := c.buildBattle(battleID, quest)

	// Store battle using graph client (EmitEntity for initial creation)
	if err := c.graph.EmitEntity(ctx, battle, "battle.started"); err != nil {
		return nil, errs.Wrap(err, "BossBattle", "StartBattle", "emit battle entity")
	}

	// Add to active battles — use Background context so the goroutine
	// isn't canceled when the caller (handleQuestStateChange) returns.
	battleCtx, cancel := context.WithTimeout(context.Background(), c.config.DefaultTimeout)
	ab := &activeBattle{
		battle:    battle,
		quest:     quest,
		output:    output,
		startTime: time.Now(),
		cancel:    cancel,
	}
	c.activeBattles.Store(battle.ID, ab)

	c.battlesStarted.Add(1)

	// Snapshot the battle BEFORE launching the goroutine — runEvaluation
	// will mutate Status/Verdict/Results on the original. Callers get a
	// consistent point-in-time view without racing.
	snapshot := *battle

	// Run evaluation asynchronously with its own context
	go c.runEvaluation(battleCtx, ab)

	return &snapshot, nil
}

// buildBattle constructs a BossBattle from quest settings.
func (c *Component) buildBattle(id domain.BattleID, quest *domain.Quest) *BossBattle {
	now := time.Now()

	reviewLevel := quest.Constraints.ReviewLevel
	// If quest doesn't specify a level, use domain default
	if reviewLevel == domain.ReviewAuto && c.catalog != nil && c.catalog.ReviewConfig != nil {
		reviewLevel = c.catalog.ReviewConfig.DefaultReviewLevel
	}

	// Resolve criteria and judges: per-level overrides → domain defaults → hardcoded.
	criteria := c.resolveCriteria(reviewLevel)
	judges := c.resolveJudges(reviewLevel)

	// Get agent ID (handle pointer)
	var agentID domain.AgentID
	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}

	return &BossBattle{
		ID:        id,
		QuestID:   quest.ID,
		AgentID:   agentID,
		Level:     reviewLevel,
		Status:    domain.BattleActive,
		Criteria:  criteria,
		Judges:    judges,
		StartedAt: now,
	}
}

// defaultCriteria returns standard review criteria for a level.
func (c *Component) defaultCriteria(level domain.ReviewLevel) []domain.ReviewCriterion {
	switch level {
	case domain.ReviewStrict:
		return []domain.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.4, Threshold: 0.8},
			{Name: "completeness", Description: "All requirements addressed", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "High quality, production-ready", Weight: 0.2, Threshold: 0.7},
			{Name: "style", Description: "Follows conventions and best practices", Weight: 0.1, Threshold: 0.6},
		}
	case domain.ReviewHuman:
		return []domain.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.3, Threshold: 0.8},
			{Name: "completeness", Description: "All requirements addressed", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "High quality, production-ready", Weight: 0.2, Threshold: 0.7},
			{Name: "style", Description: "Follows conventions and best practices", Weight: 0.2, Threshold: 0.6},
		}
	case domain.ReviewStandard:
		return []domain.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.5, Threshold: 0.7},
			{Name: "completeness", Description: "Key requirements addressed", Weight: 0.3, Threshold: 0.6},
			{Name: "quality", Description: "Acceptable quality", Weight: 0.2, Threshold: 0.5},
		}
	default: // ReviewAuto
		return []domain.ReviewCriterion{
			{Name: "acceptance", Description: "Output is acceptable", Weight: 1.0, Threshold: 0.5},
		}
	}
}

// defaultJudges returns standard judges for a review level.
func (c *Component) defaultJudges(level domain.ReviewLevel) []domain.Judge {
	switch level {
	case domain.ReviewStrict:
		return []domain.Judge{
			{ID: "judge-auto", Type: domain.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: domain.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-llm-2", Type: domain.JudgeLLM, Config: map[string]any{}},
		}
	case domain.ReviewHuman:
		return []domain.Judge{
			{ID: "judge-auto", Type: domain.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: domain.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-human", Type: domain.JudgeHuman, Config: map[string]any{}},
		}
	case domain.ReviewStandard:
		return []domain.Judge{
			{ID: "judge-auto", Type: domain.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: domain.JudgeLLM, Config: map[string]any{}},
		}
	default: // ReviewAuto
		return []domain.Judge{
			{ID: "judge-auto", Type: domain.JudgeAutomated, Config: nil},
		}
	}
}

// resolveCriteria returns criteria from the domain catalog if available,
// checking per-level overrides first, then domain defaults, then hardcoded.
func (c *Component) resolveCriteria(level domain.ReviewLevel) []domain.ReviewCriterion {
	if c.catalog != nil && c.catalog.ReviewConfig != nil {
		rc := c.catalog.ReviewConfig
		// Per-level override takes precedence
		if rc.CriteriaByLevel != nil {
			if criteria, ok := rc.CriteriaByLevel[level]; ok && len(criteria) > 0 {
				return copyReviewCriteria(criteria)
			}
		}
		// Fall back to domain defaults
		if len(rc.DefaultCriteria) > 0 {
			return copyReviewCriteria(rc.DefaultCriteria)
		}
	}
	return c.defaultCriteria(level)
}

// resolveJudges returns judges from the domain catalog if available,
// checking per-level overrides first, then domain defaults, then hardcoded.
// ReviewHuman always includes a human judge — even when the catalog doesn't
// define one — so that computeVerdict returns Pending and the quest parks
// at in_review for manual resolution.
func (c *Component) resolveJudges(level domain.ReviewLevel) []domain.Judge {
	var judges []domain.Judge

	if c.catalog != nil && c.catalog.ReviewConfig != nil {
		rc := c.catalog.ReviewConfig
		// Per-level override takes precedence
		if rc.JudgesByLevel != nil {
			if lvl, ok := rc.JudgesByLevel[level]; ok && len(lvl) > 0 {
				judges = copyJudges(lvl)
			}
		}
		// Fall back to domain defaults
		if len(judges) == 0 && len(rc.DefaultJudges) > 0 {
			judges = copyJudges(rc.DefaultJudges)
		}
	}

	if len(judges) == 0 {
		judges = c.defaultJudges(level)
	}

	// ReviewHuman must always include a human judge so computeVerdict
	// returns Pending. Append one if the resolved list lacks it.
	if level == domain.ReviewHuman {
		hasHuman := false
		for _, j := range judges {
			if j.Type == domain.JudgeHuman {
				hasHuman = true
				break
			}
		}
		if !hasHuman {
			judges = append(judges, domain.Judge{
				ID:   "judge-human",
				Type: domain.JudgeHuman,
			})
		}
	}

	return judges
}

// copyReviewCriteria returns a defensive copy of a criteria slice.
func copyReviewCriteria(src []domain.ReviewCriterion) []domain.ReviewCriterion {
	dst := make([]domain.ReviewCriterion, len(src))
	copy(dst, src)
	return dst
}

// copyJudges returns a defensive copy of a judges slice.
// Deep-copies each Judge's Config map to prevent shared mutation.
func copyJudges(src []domain.Judge) []domain.Judge {
	dst := make([]domain.Judge, len(src))
	for i, j := range src {
		dst[i] = j
		if j.Config != nil {
			dst[i].Config = make(map[string]any, len(j.Config))
			for k, v := range j.Config {
				dst[i].Config[k] = v
			}
		}
	}
	return dst
}

// shouldAutoPass checks if this quest's difficulty is in the domain's auto-pass list.
func (c *Component) shouldAutoPass(quest *domain.Quest) bool {
	if c.catalog == nil || c.catalog.ReviewConfig == nil {
		return false
	}
	for _, d := range c.catalog.ReviewConfig.AutoPassDifficulties {
		if quest.Difficulty == d {
			return true
		}
	}
	return false
}

// autoPassQuest completes a quest directly with a synthetic victory,
// bypassing the full battle pipeline.
func (c *Component) autoPassQuest(ctx context.Context, quest *domain.Quest) {
	now := time.Now()
	quest.Status = domain.QuestCompleted
	quest.CompletedAt = &now
	if quest.StartedAt != nil {
		quest.Duration = now.Sub(*quest.StartedAt)
	}
	quest.Verdict = &domain.BattleVerdict{
		Passed:       true,
		QualityScore: 1.0,
		Feedback:     "Auto-passed: quest difficulty below review threshold",
	}

	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.completed"); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to auto-pass quest", "quest_id", quest.ID, "error", err)
		return
	}

	c.logger.Info("quest auto-passed review",
		"quest_id", quest.ID, "difficulty", quest.Difficulty)
}

// runEvaluation performs the actual battle evaluation.
func (c *Component) runEvaluation(ctx context.Context, ab *activeBattle) {
	defer func() {
		ab.cancel()
		c.activeBattles.Delete(ab.battle.ID)
	}()

	// Run evaluation
	result, err := c.evaluator.Evaluate(ctx, ab.battle, ab.quest, ab.output)

	now := time.Now()
	ab.battle.CompletedAt = &now

	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("battle evaluation failed",
			"battle", ab.battle.ID,
			"error", err)

		// Mark as failed with default verdict (defeat)
		ab.battle.Status = domain.BattleDefeat
		ab.battle.Verdict = &domain.BattleVerdict{
			Passed:       false,
			QualityScore: 0,
			Feedback:     fmt.Sprintf("Evaluation error: %v", err),
		}
	} else if result.Pending {
		// Battle awaiting human review - keep active
		c.logger.Info("battle awaiting human review",
			"battle", ab.battle.ID,
			"pending_judge", result.PendingJudge)
		return
	} else {
		// Complete with verdict
		if result.Verdict.Passed {
			ab.battle.Status = domain.BattleVictory
		} else {
			ab.battle.Status = domain.BattleDefeat
		}
		ab.battle.Results = result.Results
		ab.battle.Verdict = &result.Verdict
		ab.battle.LoopID = result.LoopID
	}

	// Persist final battle state (only if component is still running)
	// KV write IS the event — watchers (e.g., questboard) are notified of battle completion.
	if c.running.Load() {
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		eventType := "battle.completed"
		if ab.battle.Status == domain.BattleVictory {
			eventType = "battle.victory"
		} else if ab.battle.Status == domain.BattleDefeat {
			eventType = "battle.defeat"
		}
		if err := c.graph.EmitEntityUpdate(persistCtx, ab.battle, eventType); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to persist battle result",
				"battle", ab.battle.ID,
				"error", err)
		}

		// Create DM peer review entity BEFORE quest mutations (retry clears ClaimedBy).
		if ab.quest != nil && result != nil && result.PeerRatings != nil && ab.quest.ClaimedBy != nil {
			c.emitDMPeerReview(persistCtx, ab.quest, result.PeerRatings, ab.battle.Verdict.Feedback)
		}

		// Bridge battle verdict → quest completion/failure
		// Safe: no other processor modifies a quest while it's in_review.
		if ab.quest != nil {
			if ab.battle.Verdict.Passed {
				verdictNow := time.Now()
				ab.quest.Status = domain.QuestCompleted
				ab.quest.CompletedAt = &verdictNow
				ab.quest.Verdict = &domain.BattleVerdict{
					Passed:       ab.battle.Verdict.Passed,
					QualityScore: ab.battle.Verdict.QualityScore,
					XPAwarded:    ab.battle.Verdict.XPAwarded,
					Feedback:     ab.battle.Verdict.Feedback,
				}
				if ab.quest.StartedAt != nil {
					ab.quest.Duration = verdictNow.Sub(*ab.quest.StartedAt)
				}
			} else if ab.quest.Attempts < ab.quest.MaxAttempts && ab.quest.MaxAttempts > 1 {
				// Retry: repost for another attempt with feedback injected
				ab.quest.Attempts++
				c.logger.Info("reposting quest for retry after battle defeat",
					"quest", ab.quest.ID,
					"attempt", ab.quest.Attempts,
					"max_attempts", ab.quest.MaxAttempts,
					"feedback", ab.battle.Verdict.Feedback)

				// Release the agent so it (or another) can claim again
				if ab.quest.ClaimedBy != nil {
					agentEntity, agentErr := c.graph.GetAgent(persistCtx, *ab.quest.ClaimedBy)
					if agentErr == nil {
						agent := agentprogression.AgentFromEntityState(agentEntity)
						if agent != nil {
							agent.Status = domain.AgentIdle
							agent.CurrentQuest = nil
							agent.UpdatedAt = time.Now()
							if writeErr := c.graph.EmitEntityUpdate(persistCtx, agent, "agent.status.idle"); writeErr != nil {
								c.logger.Error("failed to release agent on retry repost", "error", writeErr)
							}
						}
					}
				}

				ab.quest.Status = domain.QuestPosted
				ab.quest.ClaimedBy = nil
				ab.quest.ClaimedAt = nil
				ab.quest.StartedAt = nil
				ab.quest.FailureReason = ab.battle.Verdict.Feedback
				ab.quest.FailureType = domain.FailureQuality
			} else {
				ab.quest.Status = domain.QuestFailed
				ab.quest.FailureReason = ab.battle.Verdict.Feedback
				ab.quest.FailureType = domain.FailureQuality
			}
			if questErr := c.graph.EmitEntityUpdate(persistCtx, ab.quest, "quest."+string(ab.quest.Status)); questErr != nil {
				c.errorsCount.Add(1)
				c.logger.Error("failed to transition quest after battle verdict",
					"quest", ab.quest.ID,
					"status", ab.quest.Status,
					"error", questErr)
			}

		}
	} else {
		c.logger.Debug("skipping battle persistence after shutdown",
			"battle", ab.battle.ID)
	}

	c.battlesCompleted.Add(1)
	if ab.battle.Verdict.Passed {
		c.battlesVictory.Add(1)
	} else {
		c.battlesDefeat.Add(1)
	}

	c.logger.Info("battle completed",
		"battle", ab.battle.ID,
		"quest", ab.quest.ID,
		"passed", ab.battle.Verdict.Passed,
		"quality", ab.battle.Verdict.QualityScore)
}

// GetBattle retrieves a battle by ID.
func (c *Component) GetBattle(ctx context.Context, id domain.BattleID) (*BossBattle, error) {
	entity, err := c.graph.GetBattle(ctx, domain.BattleID(id))
	if err != nil {
		return nil, err
	}
	battle := battleFromEntityState(entity)
	if battle == nil {
		return nil, errors.New("invalid battle data")
	}
	return battle, nil
}

// ListActiveBattles returns all currently active battles.
func (c *Component) ListActiveBattles() []*BossBattle {
	var battles []*BossBattle
	c.activeBattles.Range(func(_, value any) bool {
		if ab, ok := value.(*activeBattle); ok {
			battles = append(battles, ab.battle)
		}
		return true
	})
	return battles
}

// Stats returns battle statistics.
func (c *Component) Stats() BattleStats {
	return BattleStats{
		Started:   c.battlesStarted.Load(),
		Completed: c.battlesCompleted.Load(),
		Victory:   c.battlesVictory.Load(),
		Defeat:    c.battlesDefeat.Load(),
		Active:    c.countActiveBattles(),
		Errors:    c.errorsCount.Load(),
	}
}

// BattleStats holds battle statistics.
type BattleStats struct {
	Started   uint64 `json:"started"`
	Completed uint64 `json:"completed"`
	Victory   uint64 `json:"victory"`
	Defeat    uint64 `json:"defeat"`
	Active    int    `json:"active"`
	Errors    int64  `json:"errors"`
}

// Graph returns the underlying graph client for external access.
func (c *Component) Graph() *semdragons.GraphClient {
	return c.graph
}

// createGraphClient creates the graph client for the component.
// Context is unused: NewGraphClient is a synchronous in-memory constructor.
func (c *Component) createGraphClient(_ context.Context) error {
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	return nil
}

// emitDMPeerReview creates a PeerReview entity for the DM's review of an agent.
// This feeds into the existing agentprogression peer review pipeline.
func (c *Component) emitDMPeerReview(ctx context.Context, quest *domain.Quest, ratings *domain.ReviewRatings, feedback string) {
	now := time.Now()
	reviewID := domain.PeerReviewID(c.boardConfig.PeerReviewEntityID(c.generateID()))

	review := &domain.PeerReview{
		ID:       reviewID,
		Status:   domain.PeerReviewCompleted,
		QuestID:  quest.ID,
		LeaderID: domain.AgentID("dm"),
		MemberID: *quest.ClaimedBy,
		IsSoloTask: quest.PartyID == nil,
		LeaderReview: &domain.ReviewSubmission{
			ReviewerID:  domain.AgentID("dm"),
			RevieweeID:  *quest.ClaimedBy,
			Direction:   domain.ReviewDirectionDMToAgent,
			Ratings:     *ratings,
			Explanation: feedback,
			SubmittedAt: now,
		},
		LeaderAvgRating: ratings.Average(),
		CreatedAt:       now,
		CompletedAt:     &now,
	}

	if err := c.graph.EmitEntity(ctx, review, domain.PredicateReviewCompleted); err != nil {
		c.logger.Error("failed to emit DM peer review",
			"quest", quest.ID,
			"agent", *quest.ClaimedBy,
			"error", err)
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func (c *Component) generateID() string {
	return domain.GenerateInstance()
}

func (c *Component) countActiveBattles() int {
	count := 0
	c.activeBattles.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// battleFromEntityState reconstructs a BossBattle from a graph entity.
func battleFromEntityState(entity *graph.EntityState) *BossBattle {
	if entity == nil {
		return nil
	}
	// Delegate to semdragons.BattleFromEntityState and convert
	semBattle := BattleFromEntityState(entity)
	if semBattle == nil {
		return nil
	}
	// Convert semdragons.BossBattle to local BossBattle
	battle := &BossBattle{
		ID:        domain.BattleID(semBattle.ID),
		QuestID:   domain.QuestID(semBattle.QuestID),
		AgentID:   domain.AgentID(semBattle.AgentID),
		Level:     domain.ReviewLevel(semBattle.Level),
		Status:    domain.BattleStatus(semBattle.Status),
		LoopID:    semBattle.LoopID,
		StartedAt: semBattle.StartedAt,
	}
	if semBattle.CompletedAt != nil {
		battle.CompletedAt = semBattle.CompletedAt
	}
	// Copy criteria
	for _, c := range semBattle.Criteria {
		battle.Criteria = append(battle.Criteria, domain.ReviewCriterion{
			Name:        c.Name,
			Description: c.Description,
			Weight:      c.Weight,
			Threshold:   c.Threshold,
		})
	}
	// Copy judges
	for _, j := range semBattle.Judges {
		battle.Judges = append(battle.Judges, domain.Judge{
			ID:     j.ID,
			Type:   domain.JudgeType(j.Type),
			Config: j.Config,
		})
	}
	// Copy results
	for _, r := range semBattle.Results {
		battle.Results = append(battle.Results, domain.ReviewResult{
			CriterionName: r.CriterionName,
			Score:         r.Score,
			Passed:        r.Passed,
			Reasoning:     r.Reasoning,
			JudgeID:       r.JudgeID,
		})
	}
	// Copy verdict
	if semBattle.Verdict != nil {
		battle.Verdict = &domain.BattleVerdict{
			Passed:       semBattle.Verdict.Passed,
			QualityScore: semBattle.Verdict.QualityScore,
			XPAwarded:    semBattle.Verdict.XPAwarded,
			XPPenalty:    semBattle.Verdict.XPPenalty,
			Feedback:     semBattle.Verdict.Feedback,
			LevelChange:  semBattle.Verdict.LevelChange,
		}
	}
	return battle
}
