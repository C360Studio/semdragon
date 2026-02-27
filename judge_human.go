package semdragons

import (
	"context"
)

// =============================================================================
// HUMAN JUDGE - Pending human review
// =============================================================================
// The human judge indicates that evaluation requires human input.
// It returns a pending status that keeps the quest in review until
// a human provides the verdict.
// =============================================================================

// HumanJudge represents evaluation that requires human review.
type HumanJudge struct{}

// NewHumanJudge creates a new human judge.
func NewHumanJudge() *HumanJudge {
	return &HumanJudge{}
}

// Type returns the judge type.
func (j *HumanJudge) Type() JudgeType {
	return JudgeHuman
}

// Evaluate returns a pending status for human review.
// The actual verdict must be provided by a human reviewer.
func (j *HumanJudge) Evaluate(_ context.Context, _ JudgeInput) (*JudgeOutput, error) {
	return &JudgeOutput{
		Score:     0.0,
		Passed:    false,
		Reasoning: "Awaiting human review",
		Pending:   true,
	}, nil
}

// HumanVerdict represents a human reviewer's evaluation.
type HumanVerdict struct {
	ReviewerID  string  `json:"reviewer_id"`
	CriterionID string  `json:"criterion_id"`
	Score       float64 `json:"score"`
	Passed      bool    `json:"passed"`
	Reasoning   string  `json:"reasoning"`
}

// ApplyHumanVerdict converts a human verdict to a JudgeOutput.
func ApplyHumanVerdict(verdict HumanVerdict) *JudgeOutput {
	return &JudgeOutput{
		Score:     verdict.Score,
		Passed:    verdict.Passed,
		Reasoning: verdict.Reasoning,
		Pending:   false,
	}
}
