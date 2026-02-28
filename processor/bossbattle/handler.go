package bossbattle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/pkg/errs"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// EVENT HANDLERS
// =============================================================================

// handleQuestSubmitted processes quest submission events and starts battles.
func (c *Component) handleQuestSubmitted(ctx context.Context, payload semdragons.QuestSubmittedPayload) error {
	if !c.running.Load() {
		return nil
	}

	c.lastActivity.Store(time.Now())

	// Check if quest requires review
	if c.config.RequireReviewLevel && payload.Quest.Constraints.ReviewLevel == semdragons.ReviewAuto {
		c.logger.Debug("skipping battle for quest without review level",
			"quest", payload.Quest.ID)
		return nil
	}

	// Start battle
	battle, err := c.StartBattle(ctx, &payload.Quest, payload.Result)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to start battle",
			"quest", payload.Quest.ID,
			"error", err)
		return nil // Don't return error to avoid NATS redelivery
	}

	c.logger.Debug("started battle for submitted quest",
		"quest", payload.Quest.ID,
		"battle", battle.ID)

	return nil
}

// =============================================================================
// BATTLE OPERATIONS
// =============================================================================

// StartBattle initiates a boss battle for a quest.
func (c *Component) StartBattle(ctx context.Context, quest *semdragons.Quest, output any) (*semdragons.BossBattle, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	// Generate battle ID
	battleID := semdragons.BattleID(c.boardConfig.EntityID("battle", c.generateID()))

	// Build battle from quest review level
	battle := c.buildBattle(battleID, quest)

	// Store battle using graph client (EmitEntity for initial creation)
	if err := c.graph.EmitEntity(ctx, battle, "battle.started"); err != nil {
		return nil, errs.Wrap(err, "BossBattle", "StartBattle", "emit battle entity")
	}

	// Add to active battles
	battleCtx, cancel := context.WithTimeout(ctx, c.config.DefaultTimeout)
	ab := &activeBattle{
		battle:    battle,
		quest:     quest,
		output:    output,
		startTime: time.Now(),
		cancel:    cancel,
	}
	c.activeBattles.Store(battle.ID, ab)

	// Emit battle started event
	c.events.PublishBattleStarted(ctx, semdragons.BattleStartedPayload{
		Battle:    *battle,
		Quest:     *quest,
		StartedAt: battle.StartedAt,
	})

	c.battlesStarted.Add(1)

	// Run evaluation asynchronously with its own context
	go c.runEvaluation(battleCtx, ab)

	return battle, nil
}

// buildBattle constructs a BossBattle from quest settings.
func (c *Component) buildBattle(id semdragons.BattleID, quest *semdragons.Quest) *semdragons.BossBattle {
	now := time.Now()

	reviewLevel := quest.Constraints.ReviewLevel

	// Default criteria based on review level
	criteria := c.defaultCriteria(reviewLevel)
	judges := c.defaultJudges(reviewLevel)

	// Get agent ID (handle pointer)
	var agentID semdragons.AgentID
	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}

	return &semdragons.BossBattle{
		ID:        id,
		QuestID:   quest.ID,
		AgentID:   agentID,
		Level:     reviewLevel,
		Status:    semdragons.BattleActive,
		Criteria:  criteria,
		Judges:    judges,
		StartedAt: now,
	}
}

// defaultCriteria returns standard review criteria for a level.
func (c *Component) defaultCriteria(level semdragons.ReviewLevel) []semdragons.ReviewCriterion {
	switch level {
	case semdragons.ReviewStrict:
		return []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.4, Threshold: 0.8},
			{Name: "completeness", Description: "All requirements addressed", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "High quality, production-ready", Weight: 0.2, Threshold: 0.7},
			{Name: "style", Description: "Follows conventions and best practices", Weight: 0.1, Threshold: 0.6},
		}
	case semdragons.ReviewHuman:
		return []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.3, Threshold: 0.8},
			{Name: "completeness", Description: "All requirements addressed", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "High quality, production-ready", Weight: 0.2, Threshold: 0.7},
			{Name: "style", Description: "Follows conventions and best practices", Weight: 0.2, Threshold: 0.6},
		}
	case semdragons.ReviewStandard:
		return []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.5, Threshold: 0.7},
			{Name: "completeness", Description: "Key requirements addressed", Weight: 0.3, Threshold: 0.6},
			{Name: "quality", Description: "Acceptable quality", Weight: 0.2, Threshold: 0.5},
		}
	default: // ReviewAuto
		return []semdragons.ReviewCriterion{
			{Name: "acceptance", Description: "Output is acceptable", Weight: 1.0, Threshold: 0.5},
		}
	}
}

// defaultJudges returns standard judges for a review level.
func (c *Component) defaultJudges(level semdragons.ReviewLevel) []semdragons.Judge {
	switch level {
	case semdragons.ReviewStrict:
		return []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-llm-2", Type: semdragons.JudgeLLM, Config: map[string]any{}},
		}
	case semdragons.ReviewHuman:
		return []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-human", Type: semdragons.JudgeHuman, Config: map[string]any{}},
		}
	case semdragons.ReviewStandard:
		return []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
		}
	default: // ReviewAuto
		return []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: nil},
		}
	}
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
		ab.battle.Status = semdragons.BattleDefeat
		ab.battle.Verdict = &semdragons.BattleVerdict{
			Passed:       false,
			QualityScore: 0,
			Feedback:     fmt.Sprintf("Evaluation error: %v", err),
		}
	} else if result.Pending {
		// Battle awaiting human review - keep active
		// The battle stays in Active status until human provides verdict
		c.logger.Info("battle awaiting human review",
			"battle", ab.battle.ID,
			"pending_judge", result.PendingJudge)
		// Don't complete - leave in active state, don't remove from tracking
		return
	} else {
		// Complete with verdict - use Victory or Defeat based on result
		if result.Verdict.Passed {
			ab.battle.Status = semdragons.BattleVictory
		} else {
			ab.battle.Status = semdragons.BattleDefeat
		}
		ab.battle.Results = result.Results
		ab.battle.Verdict = &result.Verdict
	}

	// Persist final battle state (only if component is still running)
	// After shutdown, we skip persistence to avoid race conditions
	if c.running.Load() {
		// Use a short timeout for final persistence since the battle context may be cancelled
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// Emit update with appropriate event type based on outcome
		eventType := "battle.completed"
		if ab.battle.Status == semdragons.BattleVictory {
			eventType = "battle.victory"
		} else if ab.battle.Status == semdragons.BattleDefeat {
			eventType = "battle.defeat"
		}
		if err := c.graph.EmitEntityUpdate(persistCtx, ab.battle, eventType); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to persist battle result",
				"battle", ab.battle.ID,
				"error", err)
		}
		cancel()

		// Emit verdict event
		c.events.PublishBattleVerdict(persistCtx, semdragons.BattleVerdictPayload{
			Battle:  *ab.battle,
			Quest:   *ab.quest,
			Verdict: *ab.battle.Verdict,
			EndedAt: now,
		})
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
func (c *Component) GetBattle(ctx context.Context, id semdragons.BattleID) (*semdragons.BossBattle, error) {
	entity, err := c.graph.GetBattle(ctx, id)
	if err != nil {
		return nil, err
	}
	battle := semdragons.BattleFromEntityState(entity)
	if battle == nil {
		return nil, errors.New("invalid battle data")
	}
	return battle, nil
}

// ListActiveBattles returns all currently active battles.
func (c *Component) ListActiveBattles() []*semdragons.BossBattle {
	var battles []*semdragons.BossBattle
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
func (c *Component) createGraphClient(_ context.Context) error {
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	return nil
}

// =============================================================================
// HELPERS
// =============================================================================

func (c *Component) generateID() string {
	return semdragons.GenerateInstance()
}

func (c *Component) countActiveBattles() int {
	count := 0
	c.activeBattles.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
