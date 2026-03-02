package semdragons

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
)

// =============================================================================
// ReviewRatings unit tests
// =============================================================================

func TestReviewRatingsAverage(t *testing.T) {
	tests := []struct {
		name string
		r    ReviewRatings
		want float64
	}{
		{
			name: "all fives",
			r:    ReviewRatings{Q1: 5, Q2: 5, Q3: 5},
			want: 5.0,
		},
		{
			name: "all ones",
			r:    ReviewRatings{Q1: 1, Q2: 1, Q3: 1},
			want: 1.0,
		},
		{
			name: "mixed",
			r:    ReviewRatings{Q1: 3, Q2: 4, Q3: 5},
			want: 4.0,
		},
		{
			name: "non-integer average",
			r:    ReviewRatings{Q1: 1, Q2: 2, Q3: 3},
			want: 2.0,
		},
		{
			name: "fractional average",
			r:    ReviewRatings{Q1: 1, Q2: 2, Q3: 2},
			want: float64(1+2+2) / 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Average()
			if got != tt.want {
				t.Errorf("Average() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewRatingsValidate_ValidRange(t *testing.T) {
	tests := []struct {
		name        string
		r           ReviewRatings
		explanation string
	}{
		{
			name:        "all fives no explanation needed",
			r:           ReviewRatings{Q1: 5, Q2: 5, Q3: 5},
			explanation: "",
		},
		{
			name:        "average exactly 3 no explanation needed",
			r:           ReviewRatings{Q1: 3, Q2: 3, Q3: 3},
			explanation: "",
		},
		{
			name:        "low average with explanation",
			r:           ReviewRatings{Q1: 1, Q2: 2, Q3: 1},
			explanation: "Agent failed to communicate blockers in time",
		},
		{
			name:        "boundary values",
			r:           ReviewRatings{Q1: 1, Q2: 5, Q3: 3},
			explanation: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.r.Validate(tt.explanation); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestReviewRatingsValidate_OutOfRange(t *testing.T) {
	tests := []struct {
		name        string
		r           ReviewRatings
		explanation string
	}{
		{
			name:        "Q1 zero",
			r:           ReviewRatings{Q1: 0, Q2: 3, Q3: 3},
			explanation: "some explanation",
		},
		{
			name:        "Q2 six",
			r:           ReviewRatings{Q1: 3, Q2: 6, Q3: 3},
			explanation: "",
		},
		{
			name:        "Q3 negative",
			r:           ReviewRatings{Q1: 3, Q2: 3, Q3: -1},
			explanation: "",
		},
		{
			name:        "all zeros",
			r:           ReviewRatings{Q1: 0, Q2: 0, Q3: 0},
			explanation: "explanation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.r.Validate(tt.explanation); err == nil {
				t.Error("Validate() expected error for out-of-range rating, got nil")
			}
		})
	}
}

func TestReviewRatingsValidate_LowAvgNoExplanation(t *testing.T) {
	// Average below 3.0 without explanation must fail.
	r := ReviewRatings{Q1: 1, Q2: 2, Q3: 2} // avg = 1.67
	if err := r.Validate(""); err == nil {
		t.Error("Validate() expected error when avg < 3.0 and no explanation, got nil")
	}
}

func TestReviewRatingsValidate_LowAvgWithExplanation(t *testing.T) {
	// Average below 3.0 WITH explanation must pass.
	r := ReviewRatings{Q1: 1, Q2: 2, Q3: 2} // avg = 1.67
	if err := r.Validate("Deliverable was incomplete and missed the deadline"); err != nil {
		t.Errorf("Validate() unexpected error with explanation: %v", err)
	}
}

// =============================================================================
// PeerReview Triples tests
// =============================================================================

func TestPeerReviewTriples_Pending(t *testing.T) {
	pr := &PeerReview{
		ID:        PeerReviewID("test.dev.game.board1.peerreview.pr1"),
		Status:    PeerReviewPending,
		QuestID:   QuestID("test.dev.game.board1.quest.q1"),
		LeaderID:  AgentID("test.dev.game.board1.agent.leader"),
		MemberID:  AgentID("test.dev.game.board1.agent.member"),
		IsSoloTask: false,
		CreatedAt: time.Now(),
	}

	triples := pr.Triples()

	expected := map[string]bool{
		"review.status.state":       false,
		"review.assignment.quest":   false,
		"review.assignment.leader":  false,
		"review.assignment.member":  false,
		"review.config.solo_task":   false,
		"review.lifecycle.created_at": false,
	}

	for _, triple := range triples {
		if _, ok := expected[triple.Predicate]; ok {
			expected[triple.Predicate] = true
		}
	}

	for pred, found := range expected {
		if !found {
			t.Errorf("expected predicate %q not found in PeerReview triples", pred)
		}
	}

	// Verify no result triples for pending review.
	for _, triple := range triples {
		if triple.Predicate == "review.result.leader_avg" || triple.Predicate == "review.result.member_avg" {
			t.Errorf("pending review should not emit result triple %q", triple.Predicate)
		}
	}
}

func TestPeerReviewTriples_Completed(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	completedAt := now.Add(time.Hour)
	partyID := PartyID("test.dev.game.board1.party.p1")

	pr := &PeerReview{
		ID:       PeerReviewID("test.dev.game.board1.peerreview.pr1"),
		Status:   PeerReviewCompleted,
		QuestID:  QuestID("test.dev.game.board1.quest.q1"),
		PartyID:  &partyID,
		LeaderID: AgentID("test.dev.game.board1.agent.leader"),
		MemberID: AgentID("test.dev.game.board1.agent.member"),
		LeaderReview: &ReviewSubmission{
			Ratings:     ReviewRatings{Q1: 5, Q2: 4, Q3: 5},
			Explanation: "Outstanding work",
			SubmittedAt: now,
		},
		MemberReview: &ReviewSubmission{
			Ratings:     ReviewRatings{Q1: 4, Q2: 5, Q3: 4},
			Explanation: "",
			SubmittedAt: now,
		},
		LeaderAvgRating: 4.67,
		MemberAvgRating: 4.33,
		CreatedAt:       now,
		CompletedAt:     &completedAt,
	}

	triples := pr.Triples()

	expected := map[string]bool{
		"review.status.state":            false,
		"review.assignment.quest":        false,
		"review.assignment.leader":       false,
		"review.assignment.member":       false,
		"review.assignment.party":        false,
		"review.config.solo_task":        false,
		"review.lifecycle.created_at":    false,
		"review.lifecycle.completed_at":  false,
		"review.leader.q1":               false,
		"review.leader.q2":               false,
		"review.leader.q3":               false,
		"review.leader.submitted_at":     false,
		"review.leader.explanation":      false,
		"review.member.q1":               false,
		"review.member.q2":               false,
		"review.member.q3":               false,
		"review.member.submitted_at":     false,
		"review.result.leader_avg":       false,
		"review.result.member_avg":       false,
	}

	for _, triple := range triples {
		if _, ok := expected[triple.Predicate]; ok {
			expected[triple.Predicate] = true
		}
	}

	for pred, found := range expected {
		if !found {
			t.Errorf("expected predicate %q not found in completed PeerReview triples", pred)
		}
	}

	// Member review has no explanation — verify it's absent.
	for _, triple := range triples {
		if triple.Predicate == "review.member.explanation" {
			t.Error("member review with empty explanation should not emit review.member.explanation triple")
		}
	}
}

// =============================================================================
// PeerReviewFromEntityState round-trip tests
// =============================================================================

func TestPeerReviewFromEntityState_RoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	completedAt := now.Add(time.Hour)
	partyID := PartyID("test.dev.game.board1.party.p1")

	original := &PeerReview{
		ID:       PeerReviewID("test.dev.game.board1.peerreview.pr1"),
		Status:   PeerReviewCompleted,
		QuestID:  QuestID("test.dev.game.board1.quest.q1"),
		PartyID:  &partyID,
		LeaderID: AgentID("test.dev.game.board1.agent.leader"),
		MemberID: AgentID("test.dev.game.board1.agent.member"),
		IsSoloTask: false,
		LeaderReview: &ReviewSubmission{
			Ratings:     ReviewRatings{Q1: 5, Q2: 4, Q3: 5},
			Explanation: "Excellent deliverable",
			SubmittedAt: now,
		},
		MemberReview: &ReviewSubmission{
			Ratings:     ReviewRatings{Q1: 4, Q2: 5, Q3: 3},
			Explanation: "Good support but late context",
			SubmittedAt: now,
		},
		LeaderAvgRating: 4.67,
		MemberAvgRating: 4.0,
		CreatedAt:       now,
		CompletedAt:     &completedAt,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := PeerReviewFromEntityState(entity)

	if r.ID != original.ID {
		t.Errorf("ID = %q, want %q", r.ID, original.ID)
	}
	if r.Status != original.Status {
		t.Errorf("Status = %q, want %q", r.Status, original.Status)
	}
	if r.QuestID != original.QuestID {
		t.Errorf("QuestID = %q, want %q", r.QuestID, original.QuestID)
	}
	if r.PartyID == nil || *r.PartyID != partyID {
		t.Errorf("PartyID = %v, want %v", r.PartyID, &partyID)
	}
	if r.LeaderID != original.LeaderID {
		t.Errorf("LeaderID = %q, want %q", r.LeaderID, original.LeaderID)
	}
	if r.MemberID != original.MemberID {
		t.Errorf("MemberID = %q, want %q", r.MemberID, original.MemberID)
	}
	if r.IsSoloTask != original.IsSoloTask {
		t.Errorf("IsSoloTask = %v, want %v", r.IsSoloTask, original.IsSoloTask)
	}

	// Leader review
	if r.LeaderReview == nil {
		t.Fatal("LeaderReview is nil after reconstruction")
	}
	if r.LeaderReview.Ratings.Q1 != 5 {
		t.Errorf("LeaderReview.Q1 = %d, want 5", r.LeaderReview.Ratings.Q1)
	}
	if r.LeaderReview.Ratings.Q2 != 4 {
		t.Errorf("LeaderReview.Q2 = %d, want 4", r.LeaderReview.Ratings.Q2)
	}
	if r.LeaderReview.Ratings.Q3 != 5 {
		t.Errorf("LeaderReview.Q3 = %d, want 5", r.LeaderReview.Ratings.Q3)
	}
	if r.LeaderReview.Explanation != "Excellent deliverable" {
		t.Errorf("LeaderReview.Explanation = %q, want %q", r.LeaderReview.Explanation, "Excellent deliverable")
	}

	// Member review
	if r.MemberReview == nil {
		t.Fatal("MemberReview is nil after reconstruction")
	}
	if r.MemberReview.Ratings.Q1 != 4 {
		t.Errorf("MemberReview.Q1 = %d, want 4", r.MemberReview.Ratings.Q1)
	}
	if r.MemberReview.Explanation != "Good support but late context" {
		t.Errorf("MemberReview.Explanation = %q, want %q", r.MemberReview.Explanation, "Good support but late context")
	}

	// Averages
	if r.LeaderAvgRating != original.LeaderAvgRating {
		t.Errorf("LeaderAvgRating = %v, want %v", r.LeaderAvgRating, original.LeaderAvgRating)
	}
	if r.MemberAvgRating != original.MemberAvgRating {
		t.Errorf("MemberAvgRating = %v, want %v", r.MemberAvgRating, original.MemberAvgRating)
	}

	// Timestamps
	if !r.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", r.CreatedAt, now)
	}
	if r.CompletedAt == nil || !r.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt = %v, want %v", r.CompletedAt, &completedAt)
	}
}

func TestPeerReviewFromEntityState_PartialSubmission(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	// Only the leader has submitted so far.
	original := &PeerReview{
		ID:       PeerReviewID("test.dev.game.board1.peerreview.pr2"),
		Status:   PeerReviewPartial,
		QuestID:  QuestID("test.dev.game.board1.quest.q2"),
		LeaderID: AgentID("test.dev.game.board1.agent.lead2"),
		MemberID: AgentID("test.dev.game.board1.agent.mem2"),
		LeaderReview: &ReviewSubmission{
			Ratings:     ReviewRatings{Q1: 4, Q2: 4, Q3: 4},
			SubmittedAt: now,
		},
		CreatedAt: now,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := PeerReviewFromEntityState(entity)

	if r.Status != PeerReviewPartial {
		t.Errorf("Status = %q, want %q", r.Status, PeerReviewPartial)
	}
	if r.LeaderReview == nil {
		t.Fatal("LeaderReview should not be nil")
	}
	if r.LeaderReview.Ratings.Q1 != 4 {
		t.Errorf("LeaderReview.Q1 = %d, want 4", r.LeaderReview.Ratings.Q1)
	}
	if r.MemberReview != nil {
		t.Error("MemberReview should be nil when member has not submitted yet")
	}
	if r.CompletedAt != nil {
		t.Error("CompletedAt should be nil for partial review")
	}
}

func TestPeerReviewFromEntityState_SoloTask(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	original := &PeerReview{
		ID:         PeerReviewID("test.dev.game.board1.peerreview.pr3"),
		Status:     PeerReviewPending,
		QuestID:    QuestID("test.dev.game.board1.quest.q3"),
		LeaderID:   AgentID("test.dev.game.board1.agent.dm"),
		MemberID:   AgentID("test.dev.game.board1.agent.solo"),
		IsSoloTask: true,
		CreatedAt:  now,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := PeerReviewFromEntityState(entity)

	if !r.IsSoloTask {
		t.Error("IsSoloTask = false, want true")
	}
	if r.PartyID != nil {
		t.Errorf("PartyID should be nil for solo task, got %v", r.PartyID)
	}
}

func TestPeerReviewFromEntityState_NilReturnsNil(t *testing.T) {
	if got := PeerReviewFromEntityState(nil); got != nil {
		t.Errorf("PeerReviewFromEntityState(nil) = %v, want nil", got)
	}
}

// =============================================================================
// Agent reputation triples tests
// =============================================================================

func TestAgentReputationTriples(t *testing.T) {
	tests := []struct {
		name           string
		peerReviewAvg  float64
		peerReviewCount int
		expectTriples  bool
	}{
		{
			name:            "no peer reviews — no triples emitted",
			peerReviewAvg:   0,
			peerReviewCount: 0,
			expectTriples:   false,
		},
		{
			name:            "has peer reviews — triples emitted",
			peerReviewAvg:   4.25,
			peerReviewCount: 5,
			expectTriples:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{
				ID:     AgentID("test.dev.game.board1.agent.a1"),
				Name:   "rep-agent",
				Status: AgentIdle,
				Level:  10,
				Stats: AgentStats{
					PeerReviewAvg:   tt.peerReviewAvg,
					PeerReviewCount: tt.peerReviewCount,
				},
			}

			triples := a.Triples()

			var foundAvg, foundCount bool
			for _, triple := range triples {
				switch triple.Predicate {
				case "agent.reputation.peer_avg":
					foundAvg = true
					if got := triple.Object.(float64); got != tt.peerReviewAvg {
						t.Errorf("peer_avg = %v, want %v", got, tt.peerReviewAvg)
					}
				case "agent.reputation.peer_count":
					foundCount = true
					if got := triple.Object.(int); got != tt.peerReviewCount {
						t.Errorf("peer_count = %d, want %d", got, tt.peerReviewCount)
					}
				}
			}

			if tt.expectTriples {
				if !foundAvg {
					t.Error("agent.reputation.peer_avg triple not found")
				}
				if !foundCount {
					t.Error("agent.reputation.peer_count triple not found")
				}
			} else {
				if foundAvg {
					t.Error("agent.reputation.peer_avg should not be emitted when count is 0")
				}
				if foundCount {
					t.Error("agent.reputation.peer_count should not be emitted when count is 0")
				}
			}
		})
	}
}

func TestAgentReputationReconstruction(t *testing.T) {
	original := &Agent{
		ID:     AgentID("test.dev.game.board1.agent.a1"),
		Name:   "rep-agent",
		Status: AgentIdle,
		Level:  12,
		Tier:   TierExpert,
		Stats: AgentStats{
			QuestsCompleted: 20,
			PeerReviewAvg:   4.5,
			PeerReviewCount: 8,
		},
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := AgentFromEntityState(entity)

	if r.Stats.PeerReviewAvg != original.Stats.PeerReviewAvg {
		t.Errorf("PeerReviewAvg = %v, want %v", r.Stats.PeerReviewAvg, original.Stats.PeerReviewAvg)
	}
	if r.Stats.PeerReviewCount != original.Stats.PeerReviewCount {
		t.Errorf("PeerReviewCount = %d, want %d", r.Stats.PeerReviewCount, original.Stats.PeerReviewCount)
	}
}
