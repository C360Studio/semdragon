package bossbattle

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
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
	Verdict     *domain.BattleVerdict    `json:"verdict,omitempty"`
	Judges      []domain.Judge            `json:"judges"`
	StartedAt   time.Time                `json:"started_at"`
	CompletedAt *time.Time               `json:"completed_at,omitempty"`
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
		// Relationships — must match reconstruction predicates in BattleFromEntityState
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

// =============================================================================
// RECONSTRUCTION
// =============================================================================

// BattleFromEntityState reconstructs a BossBattle from graph EntityState.
func BattleFromEntityState(entity *graph.EntityState) *BossBattle {
	if entity == nil {
		return nil
	}

	b := &BossBattle{
		ID: domain.BattleID(entity.ID),
	}

	var judgeIDs []string

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		// Relationships
		case "battle.assignment.quest":
			b.QuestID = domain.QuestID(domain.AsString(triple.Object))
		case "battle.assignment.agent":
			b.AgentID = domain.AgentID(domain.AsString(triple.Object))

		// Status
		case "battle.status.state":
			b.Status = domain.BattleStatus(domain.AsString(triple.Object))
		case "battle.review.level":
			b.Level = domain.ReviewLevel(domain.AsInt(triple.Object))

		// Lifecycle
		case "battle.lifecycle.started_at":
			b.StartedAt = domain.AsTime(triple.Object)
		case "battle.lifecycle.completed_at":
			t := domain.AsTime(triple.Object)
			b.CompletedAt = &t

		// Verdict
		case "battle.verdict.passed":
			if b.Verdict == nil {
				b.Verdict = &domain.BattleVerdict{}
			}
			b.Verdict.Passed = domain.AsBool(triple.Object)
		case "battle.verdict.score", "battle.verdict.quality_score":
			if b.Verdict == nil {
				b.Verdict = &domain.BattleVerdict{}
			}
			b.Verdict.QualityScore = domain.AsFloat64(triple.Object)
		case "battle.verdict.xp_awarded":
			if b.Verdict == nil {
				b.Verdict = &domain.BattleVerdict{}
			}
			b.Verdict.XPAwarded = domain.AsInt64(triple.Object)
		case "battle.verdict.xp_penalty":
			if b.Verdict == nil {
				b.Verdict = &domain.BattleVerdict{}
			}
			b.Verdict.XPPenalty = domain.AsInt64(triple.Object)
		case "battle.verdict.feedback":
			if b.Verdict == nil {
				b.Verdict = &domain.BattleVerdict{}
			}
			b.Verdict.Feedback = domain.AsString(triple.Object)

		// Judges (legacy predicate — kept for backward compatibility)
		case "battle.judge.id":
			judgeIDs = append(judgeIDs, domain.AsString(triple.Object))
		}
	}

	// Reconstruct judges (we only store IDs in triples)
	for _, id := range judgeIDs {
		b.Judges = append(b.Judges, domain.Judge{ID: id})
	}

	return b
}
