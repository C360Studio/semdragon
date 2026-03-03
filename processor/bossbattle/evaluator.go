package bossbattle

import (
	"context"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// BATTLE EVALUATOR - Runs judges and produces verdicts
// =============================================================================

// BattleEvaluator runs evaluation judges on quest outputs.
type BattleEvaluator interface {
	// Evaluate runs all judges and produces a verdict.
	Evaluate(ctx context.Context, battle *BossBattle, quest *domain.Quest, output any) (*EvaluationResult, error)
}

// EvaluationResult holds the outcome of an evaluation.
type EvaluationResult struct {
	Results      []domain.ReviewResult `json:"results"`
	Verdict      domain.BattleVerdict  `json:"verdict"`
	Pending      bool                  `json:"pending"`
	PendingJudge string                `json:"pending_judge,omitempty"`
}

// DefaultBattleEvaluator provides a simple evaluator implementation.
type DefaultBattleEvaluator struct{}

// NewDefaultBattleEvaluator creates a new default evaluator.
func NewDefaultBattleEvaluator() *DefaultBattleEvaluator {
	return &DefaultBattleEvaluator{}
}

// Evaluate runs evaluation judges on the output.
func (e *DefaultBattleEvaluator) Evaluate(ctx context.Context, battle *BossBattle, _ *domain.Quest, output any) (*EvaluationResult, error) {
	// Check for cancellation - will be used for LLM judge calls when implemented
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simple implementation - auto-pass for now
	// TODO: Implement actual judge evaluation logic

	results := make([]domain.ReviewResult, len(battle.Criteria))
	totalScore := 0.0
	allPassed := true

	for i, criterion := range battle.Criteria {
		// Simple heuristic scoring based on output presence
		score := 0.0
		if output != nil {
			score = 0.8 // Output exists = 0.8 base score
		}

		passed := score >= criterion.Threshold
		results[i] = domain.ReviewResult{
			CriterionName: criterion.Name,
			Score:         score,
			Passed:        passed,
			Reasoning:     "Automated evaluation",
			JudgeID:       "judge-auto",
		}

		totalScore += score * criterion.Weight
		if !passed {
			allPassed = false
		}
	}

	// Check for human judge requirement
	for _, judge := range battle.Judges {
		if judge.Type == domain.JudgeHuman {
			return &EvaluationResult{
				Results:      results,
				Pending:      true,
				PendingJudge: judge.ID,
			}, nil
		}
	}

	verdict := domain.BattleVerdict{
		Passed:       allPassed,
		QualityScore: totalScore,
		Feedback:     "Automated evaluation complete",
	}

	return &EvaluationResult{
		Results: results,
		Verdict: verdict,
	}, nil
}

// Ensure DefaultBattleEvaluator implements BattleEvaluator.
var _ BattleEvaluator = (*DefaultBattleEvaluator)(nil)
