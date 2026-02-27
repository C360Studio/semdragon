package semdragons

import (
	"context"
	"fmt"
	"time"
)

// =============================================================================
// BATTLE EVALUATOR - Orchestrates judges for boss battles
// =============================================================================
// The BattleEvaluator coordinates multiple judges to evaluate quest output
// and produce a final verdict. It:
// 1. Runs each judge against each criterion
// 2. Aggregates scores with weighted averages
// 3. Determines pass/fail based on thresholds
// 4. Produces a BattleVerdict for XP calculation
// =============================================================================

// BattleEvaluator evaluates quest output through a boss battle.
type BattleEvaluator interface {
	// Evaluate runs all judges and produces a verdict.
	Evaluate(ctx context.Context, battle *BossBattle, quest *Quest, output any) (*EvaluationResult, error)
}

// EvaluationResult holds the outcome of a boss battle evaluation.
type EvaluationResult struct {
	Verdict      BattleVerdict  `json:"verdict"`
	Results      []ReviewResult `json:"results"`
	Pending      bool           `json:"pending"`       // True if awaiting human review
	PendingJudge *string        `json:"pending_judge"` // ID of judge awaiting input
}

// DefaultBattleEvaluator is the standard evaluator implementation.
type DefaultBattleEvaluator struct {
	registry *JudgeRegistry
}

// NewDefaultBattleEvaluator creates an evaluator with the default judge registry.
func NewDefaultBattleEvaluator() *DefaultBattleEvaluator {
	return &DefaultBattleEvaluator{
		registry: NewJudgeRegistry(),
	}
}

// NewBattleEvaluatorWithRegistry creates an evaluator with a custom registry.
func NewBattleEvaluatorWithRegistry(registry *JudgeRegistry) *DefaultBattleEvaluator {
	return &DefaultBattleEvaluator{
		registry: registry,
	}
}

// Evaluate runs all judges against all criteria and aggregates results.
func (e *DefaultBattleEvaluator) Evaluate(ctx context.Context, battle *BossBattle, quest *Quest, output any) (*EvaluationResult, error) {
	var allResults []ReviewResult
	criterionScores := make(map[string][]float64)

	// Run each criterion against each judge
	for _, criterion := range battle.Criteria {
		for _, judge := range battle.Judges {
			// Check for context cancellation before each judge evaluation
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("evaluation cancelled: %w", err)
			}

			evaluator, ok := e.registry.Get(judge.Type)
			if !ok {
				continue // Skip unknown judge types
			}

			input := JudgeInput{
				Judge:     judge,
				Quest:     *quest,
				Output:    output,
				Criterion: criterion,
			}

			judgeOutput, err := evaluator.Evaluate(ctx, input)
			if err != nil {
				// Record failure but continue
				allResults = append(allResults, ReviewResult{
					CriterionName: criterion.Name,
					Score:         0.0,
					Passed:        false,
					Reasoning:     fmt.Sprintf("Judge error: %v", err),
					JudgeID:       judge.ID,
				})
				continue
			}

			// Check for pending human review
			if judgeOutput.Pending {
				return &EvaluationResult{
					Results:      allResults,
					Pending:      true,
					PendingJudge: &judge.ID,
				}, nil
			}

			result := ReviewResult{
				CriterionName: criterion.Name,
				Score:         judgeOutput.Score,
				Passed:        judgeOutput.Passed,
				Reasoning:     judgeOutput.Reasoning,
				JudgeID:       judge.ID,
			}
			allResults = append(allResults, result)

			// Accumulate scores for this criterion
			criterionScores[criterion.Name] = append(criterionScores[criterion.Name], judgeOutput.Score)
		}
	}

	// Compute verdict
	verdict := e.computeVerdict(battle.Criteria, criterionScores, allResults)

	return &EvaluationResult{
		Verdict: verdict,
		Results: allResults,
		Pending: false,
	}, nil
}

// computeVerdict aggregates scores and determines pass/fail.
func (e *DefaultBattleEvaluator) computeVerdict(criteria []ReviewCriterion, scores map[string][]float64, results []ReviewResult) BattleVerdict {
	// Compute weighted average of criterion scores
	var totalWeight float64
	var weightedSum float64
	allPassed := true

	for _, criterion := range criteria {
		criterionScoreList, ok := scores[criterion.Name]
		if !ok || len(criterionScoreList) == 0 {
			continue
		}

		// Average scores from all judges for this criterion
		var sum float64
		for _, s := range criterionScoreList {
			sum += s
		}
		avgScore := sum / float64(len(criterionScoreList))

		// Check if criterion passed
		if avgScore < criterion.Threshold {
			allPassed = false
		}

		// Accumulate weighted score
		weightedSum += avgScore * criterion.Weight
		totalWeight += criterion.Weight
	}

	// Final quality score
	var qualityScore float64
	if totalWeight > 0 {
		qualityScore = weightedSum / totalWeight
	}

	// Build feedback from results
	feedback := e.buildFeedback(results)

	// Determine if battle passed
	// Must pass all criteria AND have overall quality >= 0.5
	passed := allPassed && qualityScore >= 0.5

	return BattleVerdict{
		Passed:       passed,
		QualityScore: qualityScore,
		Feedback:     feedback,
	}
}

// buildFeedback aggregates reasoning from all judge results.
func (e *DefaultBattleEvaluator) buildFeedback(results []ReviewResult) string {
	if len(results) == 0 {
		return "No evaluation results"
	}

	// Collect failing criteria
	var failures []string
	for _, r := range results {
		if !r.Passed {
			failures = append(failures, fmt.Sprintf("%s: %s", r.CriterionName, r.Reasoning))
		}
	}

	if len(failures) == 0 {
		return "All criteria passed"
	}

	feedback := "Areas for improvement:\n"
	for _, f := range failures {
		feedback += "- " + f + "\n"
	}
	return feedback
}

// --- Evaluation Context for Progression ---

// EvaluationContext holds everything needed for progression after evaluation.
type EvaluationContext struct {
	Quest      Quest           `json:"quest"`
	Agent      Agent           `json:"agent"`
	Battle     BossBattle      `json:"battle"`
	Verdict    BattleVerdict   `json:"verdict"`
	Duration   time.Duration   `json:"duration"`
	Streak     int             `json:"streak"`
	IsGuildQuest bool          `json:"is_guild_quest"`
}

// ToXPContext converts evaluation context to XP context for the XP engine.
func (ec *EvaluationContext) ToXPContext() XPContext {
	return XPContext{
		Quest:        ec.Quest,
		Agent:        ec.Agent,
		BattleResult: ec.Verdict,
		Duration:     ec.Duration,
		Streak:       ec.Streak,
		IsGuildQuest: ec.IsGuildQuest,
		Attempt:      ec.Quest.Attempts,
	}
}
