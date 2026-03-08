package domain

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
)

func TestQuestTriples_ReviewPredicates(t *testing.T) {
	tests := []struct {
		name            string
		requireReview   bool
		reviewLevel     ReviewLevel
		wantNeedsReview bool
		wantLevel       int
	}{
		{
			name:            "review required with standard level",
			requireReview:   true,
			reviewLevel:     ReviewStandard,
			wantNeedsReview: true,
			wantLevel:       int(ReviewStandard),
		},
		{
			name:            "review not required",
			requireReview:   false,
			reviewLevel:     ReviewAuto,
			wantNeedsReview: false,
			wantLevel:       int(ReviewAuto),
		},
		{
			name:            "strict review",
			requireReview:   true,
			reviewLevel:     ReviewStrict,
			wantNeedsReview: true,
			wantLevel:       int(ReviewStrict),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Quest{
				ID:       QuestID("test.dev.game.board1.quest.q1"),
				Title:    "Test Quest",
				Status:   QuestPosted,
				PostedAt: time.Now(),
				Constraints: QuestConstraints{
					RequireReview: tt.requireReview,
					ReviewLevel:   tt.reviewLevel,
				},
			}

			triples := q.Triples()

			var foundNeedsReview, foundLevel bool
			for _, triple := range triples {
				switch triple.Predicate {
				case "quest.review.needs_review":
					foundNeedsReview = true
					got, ok := triple.Object.(bool)
					if !ok {
						t.Errorf("quest.review.needs_review: expected bool, got %T", triple.Object)
					} else if got != tt.wantNeedsReview {
						t.Errorf("quest.review.needs_review = %v, want %v", got, tt.wantNeedsReview)
					}
				case "quest.review.level":
					foundLevel = true
					got, ok := triple.Object.(int)
					if !ok {
						t.Errorf("quest.review.level: expected int, got %T", triple.Object)
					} else if got != tt.wantLevel {
						t.Errorf("quest.review.level = %v, want %v", got, tt.wantLevel)
					}
				}
			}

			if !foundNeedsReview {
				t.Error("quest.review.needs_review triple not found")
			}
			if !foundLevel {
				t.Error("quest.review.level triple not found")
			}
		})
	}
}

func TestQuestRoundTrip_DMClarifications(t *testing.T) {
	askedAt := time.Now().Truncate(time.Second)
	clarifications := []ClarificationExchange{
		{
			Question: "What is the expected output format?",
			Answer:   "Return a JSON object with keys: result, confidence.",
			AskedAt:  askedAt,
		},
		{
			Question: "Should the agent include reasoning steps?",
			Answer:   "Yes, include a brief chain-of-thought.",
			AskedAt:  askedAt,
		},
	}

	original := &Quest{
		ID:               QuestID("test.dev.game.board1.quest.q1"),
		Title:            "DM Clarification Round-Trip",
		Status:           QuestInProgress,
		PostedAt:         time.Now().Truncate(time.Second),
		DMClarifications: clarifications,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.DMClarifications == nil {
		t.Fatal("DMClarifications is nil, want non-nil")
	}
}

func TestQuestRoundTrip_DMClarificationsNil(t *testing.T) {
	// A quest without DM clarifications should not emit the predicate.
	original := &Quest{
		ID:       QuestID("test.dev.game.board1.quest.q2"),
		Title:    "No DM Clarifications",
		Status:   QuestPosted,
		PostedAt: time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.DMClarifications != nil {
		t.Errorf("DMClarifications = %v, want nil", r.DMClarifications)
	}

	// Also verify no quest.dm.clarifications triple was emitted.
	for _, triple := range entity.Triples {
		if triple.Predicate == "quest.dm.clarifications" {
			t.Errorf("unexpected triple emitted: predicate %q with object %v", triple.Predicate, triple.Object)
		}
	}
}
