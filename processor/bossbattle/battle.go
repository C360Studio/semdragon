package bossbattle

import (
	"fmt"
	"strconv"
	"strings"
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
	LoopID  string              `json:"loop_id,omitempty"`

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

	// Loop ID for trajectory observability (omitted when empty)
	if b.LoopID != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "battle.execution.loop_id", Object: b.LoopID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
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

	// Indexed judge predicates: battle.judge.N.id and battle.judge.N.type
	judgeByIndex := make(map[int]*domain.Judge)

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
		// Execution metadata
		case "battle.execution.loop_id":
			b.LoopID = domain.AsString(triple.Object)

		default:
			// Indexed judge predicates: battle.judge.N.id / battle.judge.N.type
			if strings.HasPrefix(triple.Predicate, "battle.judge.") {
				parseIndexedJudge(triple.Predicate, triple.Object, judgeByIndex)
			}
		}
	}

	// Reconstruct judges in index order
	b.Judges = collectJudges(judgeByIndex)

	return b
}

// parseIndexedJudge extracts judge ID or type from predicates like
// "battle.judge.0.id" or "battle.judge.1.type" into the indexed map.
// Also handles legacy 3-part predicates like "battle.judge.id".
func parseIndexedJudge(predicate string, object any, judges map[int]*domain.Judge) {
	// Expected: battle.judge.<N>.<field> (4 parts) or battle.judge.<field> (3 parts, legacy)
	parts := strings.Split(predicate, ".")
	if len(parts) == 3 {
		// Legacy unindexed predicate: battle.judge.id (type was never emitted in legacy format)
		if parts[2] == "id" {
			nextIdx := len(judges)
			judges[nextIdx] = &domain.Judge{ID: domain.AsString(object)}
		}
		return
	}
	if len(parts) != 4 {
		return
	}
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	if judges[idx] == nil {
		judges[idx] = &domain.Judge{}
	}
	switch parts[3] {
	case "id":
		judges[idx].ID = domain.AsString(object)
	case "type":
		judges[idx].Type = domain.JudgeType(domain.AsString(object))
	}
}

// collectJudges converts the indexed judge map to a sorted slice.
func collectJudges(m map[int]*domain.Judge) []domain.Judge {
	if len(m) == 0 {
		return nil
	}
	// Find max index to size the slice
	maxIdx := 0
	for idx := range m {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	judges := make([]domain.Judge, 0, len(m))
	for i := 0; i <= maxIdx; i++ {
		if j, ok := m[i]; ok {
			judges = append(judges, *j)
		}
	}
	return judges
}
