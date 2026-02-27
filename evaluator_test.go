package semdragons

import (
	"context"
	"testing"
)

func TestEvaluator_SingleAutomatedJudge(t *testing.T) {
	evaluator := NewDefaultBattleEvaluator()
	ctx := context.Background()

	battle := &BossBattle{
		ID:      "test-battle",
		QuestID: "test-quest",
		Level:   ReviewAuto,
		Criteria: []ReviewCriterion{
			{Name: "format", Weight: 0.5, Threshold: 0.9},
			{Name: "completeness", Weight: 0.5, Threshold: 0.9},
		},
		Judges: []Judge{
			{ID: "auto", Type: JudgeAutomated},
		},
	}

	quest := &Quest{
		ID:     "test-quest",
		Title:  "Test Quest",
		BaseXP: 100,
	}

	output := map[string]string{"result": "valid data"}

	result, err := evaluator.Evaluate(ctx, battle, quest, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Pending {
		t.Error("expected result to not be pending")
	}

	if len(result.Results) != 2 {
		t.Errorf("expected 2 results (one per criterion), got %d", len(result.Results))
	}

	// Both format and completeness should pass for valid map
	if !result.Verdict.Passed {
		t.Errorf("expected verdict to pass, got Passed=%v, QualityScore=%f",
			result.Verdict.Passed, result.Verdict.QualityScore)
	}
}

func TestEvaluator_FailsWithNilOutput(t *testing.T) {
	evaluator := NewDefaultBattleEvaluator()
	ctx := context.Background()

	battle := &BossBattle{
		ID:      "test-battle",
		QuestID: "test-quest",
		Level:   ReviewAuto,
		Criteria: []ReviewCriterion{
			{Name: "format", Weight: 1.0, Threshold: 0.9},
		},
		Judges: []Judge{
			{ID: "auto", Type: JudgeAutomated},
		},
	}

	quest := &Quest{ID: "test-quest"}

	result, err := evaluator.Evaluate(ctx, battle, quest, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Verdict.Passed {
		t.Error("expected verdict to fail for nil output")
	}

	if result.Verdict.QualityScore != 0.0 {
		t.Errorf("expected quality score 0.0 for nil output, got %f", result.Verdict.QualityScore)
	}
}

func TestEvaluator_MultipleJudges_Aggregates(t *testing.T) {
	evaluator := NewDefaultBattleEvaluator()
	ctx := context.Background()

	battle := &BossBattle{
		ID:      "test-battle",
		QuestID: "test-quest",
		Level:   ReviewStandard,
		Criteria: []ReviewCriterion{
			{Name: "format", Weight: 0.5, Threshold: 0.7},
			{Name: "completeness", Weight: 0.5, Threshold: 0.7},
		},
		Judges: []Judge{
			{ID: "auto", Type: JudgeAutomated},
			{ID: "llm", Type: JudgeLLM},
		},
	}

	quest := &Quest{ID: "test-quest"}
	output := map[string]string{"result": "valid"}

	result, err := evaluator.Evaluate(ctx, battle, quest, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 4 results: 2 criteria × 2 judges
	if len(result.Results) != 4 {
		t.Errorf("expected 4 results, got %d", len(result.Results))
	}

	// Automated gives 1.0, LLM gives 0.75, average = 0.875
	// Both criteria should average above 0.7 threshold
	if !result.Verdict.Passed {
		t.Errorf("expected verdict to pass, got Passed=%v, QualityScore=%f",
			result.Verdict.Passed, result.Verdict.QualityScore)
	}
}

func TestEvaluator_WeightedAverage(t *testing.T) {
	evaluator := NewDefaultBattleEvaluator()
	ctx := context.Background()

	battle := &BossBattle{
		ID:      "test-battle",
		QuestID: "test-quest",
		Level:   ReviewAuto,
		Criteria: []ReviewCriterion{
			{Name: "format", Weight: 0.8, Threshold: 0.9},      // Will pass (1.0)
			{Name: "completeness", Weight: 0.2, Threshold: 0.9}, // Will pass (1.0)
		},
		Judges: []Judge{
			{ID: "auto", Type: JudgeAutomated},
		},
	}

	quest := &Quest{ID: "test-quest"}
	output := map[string]string{"result": "valid"}

	result, err := evaluator.Evaluate(ctx, battle, quest, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both scores are 1.0, so weighted average should be 1.0
	if result.Verdict.QualityScore != 1.0 {
		t.Errorf("expected quality score 1.0, got %f", result.Verdict.QualityScore)
	}
}

func TestEvaluator_HumanJudge_ReturnsPending(t *testing.T) {
	evaluator := NewDefaultBattleEvaluator()
	ctx := context.Background()

	battle := &BossBattle{
		ID:      "test-battle",
		QuestID: "test-quest",
		Level:   ReviewHuman,
		Criteria: []ReviewCriterion{
			{Name: "format", Weight: 0.5, Threshold: 0.8},
		},
		Judges: []Judge{
			{ID: "auto", Type: JudgeAutomated},
			{ID: "human", Type: JudgeHuman},
		},
	}

	quest := &Quest{ID: "test-quest"}
	output := "valid output"

	result, err := evaluator.Evaluate(ctx, battle, quest, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Pending {
		t.Error("expected result to be pending when human judge is involved")
	}

	if result.PendingJudge == nil || *result.PendingJudge != "human" {
		t.Error("expected pending judge to be identified")
	}
}

func TestEvaluator_PartialCriterionFailure(t *testing.T) {
	evaluator := NewDefaultBattleEvaluator()
	ctx := context.Background()

	battle := &BossBattle{
		ID:      "test-battle",
		QuestID: "test-quest",
		Level:   ReviewAuto,
		Criteria: []ReviewCriterion{
			{Name: "format", Weight: 0.5, Threshold: 0.9},   // Will pass (1.0 > 0.9)
			{Name: "non_empty", Weight: 0.5, Threshold: 0.9}, // Will fail (0.5 < 0.9 for short string)
		},
		Judges: []Judge{
			{ID: "auto", Type: JudgeAutomated},
		},
	}

	quest := &Quest{ID: "test-quest"}
	output := "short" // Short string gets 0.5 from non_empty checker

	result, err := evaluator.Evaluate(ctx, battle, quest, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// One criterion failed (non_empty threshold not met)
	if result.Verdict.Passed {
		t.Error("expected verdict to fail when criterion threshold not met")
	}
}

func TestEvaluator_BuildsFeedback(t *testing.T) {
	evaluator := NewDefaultBattleEvaluator()
	ctx := context.Background()

	battle := &BossBattle{
		ID:      "test-battle",
		QuestID: "test-quest",
		Level:   ReviewAuto,
		Criteria: []ReviewCriterion{
			{Name: "format", Weight: 1.0, Threshold: 0.9},
		},
		Judges: []Judge{
			{ID: "auto", Type: JudgeAutomated},
		},
	}

	quest := &Quest{ID: "test-quest"}

	// Test with nil output to trigger failure
	result, err := evaluator.Evaluate(ctx, battle, quest, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Verdict.Feedback == "" {
		t.Error("expected feedback to be populated on failure")
	}
}

func TestEvaluationContext_ToXPContext(t *testing.T) {
	ec := EvaluationContext{
		Quest: Quest{
			ID:       "test-quest",
			BaseXP:   100,
			Attempts: 1,
		},
		Agent: Agent{
			ID:    "test-agent",
			Level: 5,
		},
		Verdict: BattleVerdict{
			Passed:       true,
			QualityScore: 0.9,
		},
		Duration:     60,
		Streak:       3,
		IsGuildQuest: true,
	}

	xpCtx := ec.ToXPContext()

	if xpCtx.Quest.ID != ec.Quest.ID {
		t.Error("Quest not transferred correctly")
	}
	if xpCtx.Agent.ID != ec.Agent.ID {
		t.Error("Agent not transferred correctly")
	}
	if xpCtx.BattleResult.QualityScore != ec.Verdict.QualityScore {
		t.Error("BattleResult not transferred correctly")
	}
	if xpCtx.Streak != ec.Streak {
		t.Errorf("Streak not transferred: got %d, want %d", xpCtx.Streak, ec.Streak)
	}
	if xpCtx.IsGuildQuest != ec.IsGuildQuest {
		t.Error("IsGuildQuest not transferred correctly")
	}
}
