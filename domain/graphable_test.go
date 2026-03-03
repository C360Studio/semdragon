package domain

import (
	"testing"
	"time"
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
