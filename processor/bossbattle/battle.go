package bossbattle

import (
	"fmt"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// BOSS BATTLE - Quality gate review session for quest submissions
// =============================================================================
// BossBattle is the core entity owned by the bossbattle processor.
// It implements graph.Graphable for persistence in the semstreams graph system.
// =============================================================================

// BossBattle represents a quality gate review session.
type BossBattle struct {
	ID      domain.BattleID     `json:"id"`
	QuestID domain.QuestID      `json:"quest_id"`
	AgentID domain.AgentID      `json:"agent_id"`
	Level   domain.ReviewLevel  `json:"level"`
	Status  domain.BattleStatus `json:"status"`

	Criteria    []domain.ReviewCriterion `json:"criteria"`
	Results     []domain.ReviewResult    `json:"results,omitempty"`
	Verdict     *BattleVerdict           `json:"verdict,omitempty"`
	Judges      []Judge                  `json:"judges"`
	StartedAt   time.Time                `json:"started_at"`
	CompletedAt *time.Time               `json:"completed_at,omitempty"`
}

// BattleVerdict holds the outcome of a boss battle.
type BattleVerdict struct {
	Passed       bool    `json:"passed"`
	QualityScore float64 `json:"quality_score"`
	XPAwarded    int64   `json:"xp_awarded"`
	XPPenalty    int64   `json:"xp_penalty"`
	Feedback     string  `json:"feedback"`
	LevelChange  int     `json:"level_change"`
}

// Judge represents an evaluator for boss battles.
type Judge struct {
	ID     string           `json:"id"`
	Type   domain.JudgeType `json:"type"`
	Config map[string]any   `json:"config"`
}

// =============================================================================
// GRAPHABLE IMPLEMENTATION
// =============================================================================

// EntityID returns the 6-part entity ID for this battle.
func (b *BossBattle) EntityID() string {
	return string(b.ID)
}

// Triples returns all semantic facts about this boss battle.
func (b *BossBattle) Triples() []message.Triple {
	now := time.Now()
	source := "bossbattle"
	entityID := b.EntityID()

	triples := []message.Triple{
		// Relationships — must match reconstruction predicates in reconstruction.go
		{Subject: entityID, Predicate: "battle.assignment.quest", Object: string(b.QuestID), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "battle.assignment.agent", Object: string(b.AgentID), Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "battle.status.state", Object: string(b.Status), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "battle.review.level", Object: int(b.Level), Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "battle.lifecycle.started_at", Object: b.StartedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Criteria count
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "battle.criteria.count", Object: len(b.Criteria),
		Source: source, Timestamp: now, Confidence: 1.0,
	})

	// Judge information
	for i, judge := range b.Judges {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("battle.judge.%d.id", i), Object: judge.ID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("battle.judge.%d.type", i), Object: string(judge.Type),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Verdict if present
	if b.Verdict != nil {
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: "battle.verdict.passed", Object: b.Verdict.Passed, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "battle.verdict.quality_score", Object: b.Verdict.QualityScore, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "battle.verdict.xp_awarded", Object: b.Verdict.XPAwarded, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "battle.verdict.xp_penalty", Object: b.Verdict.XPPenalty, Source: source, Timestamp: now, Confidence: 1.0},
		)

		if b.Verdict.Feedback != "" {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: "battle.verdict.feedback", Object: b.Verdict.Feedback,
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
	}

	// Completed time if set
	if b.CompletedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "battle.lifecycle.completed_at", Object: b.CompletedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Results summary
	if len(b.Results) > 0 {
		passedCount := 0
		for _, r := range b.Results {
			if r.Passed {
				passedCount++
			}
		}
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: "battle.results.count", Object: len(b.Results), Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "battle.results.passed", Object: passedCount, Source: source, Timestamp: now, Confidence: 1.0},
		)
	}

	return triples
}
