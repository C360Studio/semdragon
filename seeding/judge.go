package seeding

import (
	"context"

	"github.com/c360studio/semdragons"
)

// =============================================================================
// ARENA JUDGE - LLM-based evaluation for training
// =============================================================================

// ArenaJudge evaluates training quest results using an LLM.
type ArenaJudge struct {
	config semdragons.AgentConfig
}

// NewArenaJudge creates a new arena judge with the given LLM config.
func NewArenaJudge(config semdragons.AgentConfig) *ArenaJudge {
	return &ArenaJudge{
		config: config,
	}
}

// JudgeResult holds the evaluation outcome.
type JudgeResult struct {
	Passed       bool               `json:"passed"`
	QualityScore float64            `json:"quality_score"`
	Feedback     string             `json:"feedback"`
	Criteria     map[string]bool    `json:"criteria"`
	Scores       map[string]float64 `json:"scores"`
}

// Evaluate judges a quest result against the template criteria.
// The ctx, template, and result parameters will be used when LLM integration is implemented.
func (j *ArenaJudge) Evaluate(_ context.Context, template *QuestTemplate, result any) (*JudgeResult, error) {
	// In a real implementation, this would:
	// 1. Build an evaluation prompt from template.Criteria
	// 2. Call the judge LLM with the quest input, expected output, and actual result
	// 3. Parse the LLM's evaluation into structured scores
	//
	// For now, return a simulated passing result for training
	// TODO: Use template.Criteria and result in LLM evaluation prompt
	_ = template // Reserved for LLM prompt construction
	_ = result   // Reserved for LLM evaluation

	return &JudgeResult{
		Passed:       true,
		QualityScore: 0.8, // Good quality for training purposes
		Feedback:     "Training quest completed successfully.",
		Criteria:     make(map[string]bool),
		Scores:       make(map[string]float64),
	}, nil
}

// EvaluateWithRubric evaluates using detailed scoring rubrics.
// The ctx, quest, and result parameters will be used when LLM integration is implemented.
func (j *ArenaJudge) EvaluateWithRubric(_ context.Context, quest *semdragons.Quest, result any, rubric []semdragons.ReviewCriterion) (*JudgeResult, error) {
	// Detailed evaluation using provided rubric
	// Each criterion is scored and aggregated
	// TODO: Use quest and result in LLM evaluation prompt
	_ = quest  // Reserved for LLM context
	_ = result // Reserved for LLM evaluation

	scores := make(map[string]float64)
	criteria := make(map[string]bool)
	var totalScore float64
	var totalWeight float64

	for _, criterion := range rubric {
		// In production, each criterion would be evaluated by the LLM
		// For now, simulate reasonable scores
		score := 0.75 + 0.20*(float64(len(rubric)-1)/float64(len(rubric)+1)) // ~0.75-0.95
		scores[criterion.Name] = score
		criteria[criterion.Name] = score >= criterion.Threshold
		totalScore += score * criterion.Weight
		totalWeight += criterion.Weight
	}

	avgScore := totalScore / totalWeight
	allPassed := true
	for _, passed := range criteria {
		if !passed {
			allPassed = false
			break
		}
	}

	return &JudgeResult{
		Passed:       allPassed,
		QualityScore: avgScore,
		Feedback:     "Evaluation completed.",
		Criteria:     criteria,
		Scores:       scores,
	}, nil
}

// ToBattleVerdict converts a JudgeResult to a BattleVerdict.
func (r *JudgeResult) ToBattleVerdict(questXP int64) semdragons.BattleVerdict {
	var xpAwarded int64
	var xpPenalty int64

	if r.Passed {
		// Full XP scaled by quality
		xpAwarded = int64(float64(questXP) * r.QualityScore)
	} else {
		// Penalty for failure
		xpPenalty = questXP / 4
	}

	return semdragons.BattleVerdict{
		Passed:       r.Passed,
		QualityScore: r.QualityScore,
		XPAwarded:    xpAwarded,
		XPPenalty:    xpPenalty,
		Feedback:     r.Feedback,
	}
}
