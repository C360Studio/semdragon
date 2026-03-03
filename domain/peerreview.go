package domain

import (
	"fmt"
	"time"
)

// =============================================================================
// PEER REVIEW
// =============================================================================

// ReviewRatings holds the 3 peer review ratings (1-5 scale).
type ReviewRatings struct {
	Q1 int `json:"q1"` // Quality/Clarity
	Q2 int `json:"q2"` // Communication/Support
	Q3 int `json:"q3"` // Autonomy/Fairness
}

// Average returns the mean of the three ratings.
func (r ReviewRatings) Average() float64 {
	return float64(r.Q1+r.Q2+r.Q3) / 3.0
}

// Validate checks that ratings are in range and explanation is provided when required.
func (r ReviewRatings) Validate(explanation string) error {
	for i, v := range []int{r.Q1, r.Q2, r.Q3} {
		if v < 1 || v > 5 {
			return fmt.Errorf("rating Q%d must be between 1 and 5, got %d", i+1, v)
		}
	}
	if r.Average() < 3.0 && explanation == "" {
		return fmt.Errorf("explanation required when average rating is below 3.0")
	}
	return nil
}

// ReviewSubmission represents one party's submitted review.
type ReviewSubmission struct {
	ReviewerID  AgentID         `json:"reviewer_id"`
	RevieweeID  AgentID         `json:"reviewee_id"`
	Direction   ReviewDirection `json:"direction"`
	Ratings     ReviewRatings   `json:"ratings"`
	Explanation string          `json:"explanation,omitempty"`
	SubmittedAt time.Time       `json:"submitted_at"`
}

// PeerReview is the entity tracking bidirectional review between two agents.
type PeerReview struct {
	ID              PeerReviewID      `json:"id"`
	Status          PeerReviewStatus  `json:"status"`
	QuestID         QuestID           `json:"quest_id"`
	PartyID         *PartyID          `json:"party_id,omitempty"`
	LeaderID        AgentID           `json:"leader_id"`
	MemberID        AgentID           `json:"member_id"`
	IsSoloTask      bool              `json:"is_solo_task"`
	LeaderReview    *ReviewSubmission `json:"leader_review,omitempty"`
	MemberReview    *ReviewSubmission `json:"member_review,omitempty"`
	LeaderAvgRating float64           `json:"leader_avg_rating"`
	MemberAvgRating float64           `json:"member_avg_rating"`
	CreatedAt       time.Time         `json:"created_at"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
}

// LeaderToMemberQuestions are the review questions for leader reviewing member.
var LeaderToMemberQuestions = [3]string{
	"Task quality — did the deliverable meet acceptance criteria?",
	"Communication — were blockers surfaced promptly?",
	"Autonomy — did they work independently without excessive hand-holding?",
}

// MemberToLeaderQuestions are the review questions for member reviewing leader.
var MemberToLeaderQuestions = [3]string{
	"Clarity — was the task well-defined with clear acceptance criteria?",
	"Support — were blockers unblocked promptly?",
	"Fairness — was the task appropriate for my level/skills?",
}
