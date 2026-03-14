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
	Name    string              `json:"name,omitempty"`
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
		// Identity
		{Subject: entityID, Predicate: "battle.identity.name", Object: b.Name, Source: source, Timestamp: now, Confidence: 1.0},

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

	// Criteria count + indexed details
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "battle.criteria.count", Object: len(b.Criteria),
		Source: source, Timestamp: now, Confidence: 1.0,
	})
	for i, c := range b.Criteria {
		prefix := fmt.Sprintf("battle.criteria.%d", i)
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: prefix + ".name", Object: c.Name, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".description", Object: c.Description, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".weight", Object: c.Weight, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".threshold", Object: c.Threshold, Source: source, Timestamp: now, Confidence: 1.0},
		)
	}

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
			message.Triple{Subject: entityID, Predicate: "battle.verdict.level_change", Object: b.Verdict.LevelChange, Source: source, Timestamp: now, Confidence: 1.0},
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

	// Results summary + indexed details
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
		for i, r := range b.Results {
			prefix := fmt.Sprintf("battle.result.%d", i)
			triples = append(triples,
				message.Triple{Subject: entityID, Predicate: prefix + ".criterion_name", Object: r.CriterionName, Source: source, Timestamp: now, Confidence: 1.0},
				message.Triple{Subject: entityID, Predicate: prefix + ".judge_id", Object: r.JudgeID, Source: source, Timestamp: now, Confidence: 1.0},
				message.Triple{Subject: entityID, Predicate: prefix + ".score", Object: r.Score, Source: source, Timestamp: now, Confidence: 1.0},
				message.Triple{Subject: entityID, Predicate: prefix + ".passed", Object: r.Passed, Source: source, Timestamp: now, Confidence: 1.0},
				message.Triple{Subject: entityID, Predicate: prefix + ".reasoning", Object: r.Reasoning, Source: source, Timestamp: now, Confidence: 1.0},
			)
		}
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

	// Indexed maps for reconstruction
	judgeByIndex := make(map[int]*domain.Judge)
	criteriaByIndex := make(map[int]*domain.ReviewCriterion)
	resultByIndex := make(map[int]*domain.ReviewResult)

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		// Identity
		case "battle.identity.name":
			b.Name = domain.AsString(triple.Object)

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
		case "battle.verdict.level_change":
			if b.Verdict == nil {
				b.Verdict = &domain.BattleVerdict{}
			}
			b.Verdict.LevelChange = domain.AsInt(triple.Object)
		case "battle.verdict.feedback":
			if b.Verdict == nil {
				b.Verdict = &domain.BattleVerdict{}
			}
			b.Verdict.Feedback = domain.AsString(triple.Object)
		// Execution metadata
		case "battle.execution.loop_id":
			b.LoopID = domain.AsString(triple.Object)

		default:
			// Indexed predicates: battle.judge.N.*, battle.criteria.N.*, battle.result.N.*
			if strings.HasPrefix(triple.Predicate, "battle.judge.") {
				parseIndexedJudge(triple.Predicate, triple.Object, judgeByIndex)
			} else if strings.HasPrefix(triple.Predicate, "battle.criteria.") {
				parseIndexedCriterion(triple.Predicate, triple.Object, criteriaByIndex)
			} else if strings.HasPrefix(triple.Predicate, "battle.result.") {
				parseIndexedResult(triple.Predicate, triple.Object, resultByIndex)
			}
		}
	}

	// Reconstruct indexed collections in order
	b.Judges = collectJudges(judgeByIndex)
	b.Criteria = collectCriteria(criteriaByIndex)
	b.Results = collectResults(resultByIndex)

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

// parseIndexedCriterion extracts criterion fields from predicates like
// "battle.criteria.0.name" or "battle.criteria.1.threshold".
func parseIndexedCriterion(predicate string, object any, criteria map[int]*domain.ReviewCriterion) {
	parts := strings.Split(predicate, ".")
	if len(parts) != 4 {
		return
	}
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	if criteria[idx] == nil {
		criteria[idx] = &domain.ReviewCriterion{}
	}
	switch parts[3] {
	case "name":
		criteria[idx].Name = domain.AsString(object)
	case "description":
		criteria[idx].Description = domain.AsString(object)
	case "weight":
		criteria[idx].Weight = domain.AsFloat64(object)
	case "threshold":
		criteria[idx].Threshold = domain.AsFloat64(object)
	}
}

// parseIndexedResult extracts result fields from predicates like
// "battle.result.0.score" or "battle.result.1.passed".
func parseIndexedResult(predicate string, object any, results map[int]*domain.ReviewResult) {
	parts := strings.Split(predicate, ".")
	if len(parts) != 4 {
		return
	}
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	if results[idx] == nil {
		results[idx] = &domain.ReviewResult{}
	}
	switch parts[3] {
	case "criterion_name":
		results[idx].CriterionName = domain.AsString(object)
	case "judge_id":
		results[idx].JudgeID = domain.AsString(object)
	case "score":
		results[idx].Score = domain.AsFloat64(object)
	case "passed":
		results[idx].Passed = domain.AsBool(object)
	case "reasoning":
		results[idx].Reasoning = domain.AsString(object)
	}
}

// collectCriteria converts the indexed criterion map to a sorted slice.
func collectCriteria(m map[int]*domain.ReviewCriterion) []domain.ReviewCriterion {
	if len(m) == 0 {
		return nil
	}
	maxIdx := 0
	for idx := range m {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	result := make([]domain.ReviewCriterion, 0, len(m))
	for i := 0; i <= maxIdx; i++ {
		if c, ok := m[i]; ok {
			result = append(result, *c)
		}
	}
	return result
}

// collectResults converts the indexed result map to a sorted slice.
func collectResults(m map[int]*domain.ReviewResult) []domain.ReviewResult {
	if len(m) == 0 {
		return nil
	}
	maxIdx := 0
	for idx := range m {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	result := make([]domain.ReviewResult, 0, len(m))
	for i := 0; i <= maxIdx; i++ {
		if r, ok := m[i]; ok {
			result = append(result, *r)
		}
	}
	return result
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
