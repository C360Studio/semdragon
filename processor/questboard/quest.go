package questboard

import (
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// QUEST - A unit of work posted to the quest board
// =============================================================================
// Quest is the core entity owned by the questboard processor.
// It implements graph.Graphable for persistence in the semstreams graph system.
// =============================================================================

// Quest represents a unit of work on the quest board.
type Quest struct {
	ID          domain.QuestID         `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Status      domain.QuestStatus     `json:"status"`
	Difficulty  domain.QuestDifficulty `json:"difficulty"`

	// Requirements
	RequiredSkills []domain.SkillTag `json:"required_skills"`
	RequiredTools  []string          `json:"required_tools"` // Tool IDs
	MinTier        domain.TrustTier  `json:"min_tier"`
	PartyRequired  bool              `json:"party_required"` // Too big for solo
	MinPartySize   int               `json:"min_party_size"`

	// Rewards
	BaseXP  int64 `json:"base_xp"`
	BonusXP int64 `json:"bonus_xp"` // For exceptional quality
	GuildXP int64 `json:"guild_xp"` // XP toward guild reputation

	// Execution context - the actual work
	Input        any              `json:"input"`  // Quest payload
	Output       any              `json:"output"` // Result when completed
	Constraints  QuestConstraints `json:"constraints"`
	AllowedTools []string         `json:"allowed_tools,omitempty"` // Tool whitelist for execution

	// Quest chain / decomposition
	ParentQuest  *domain.QuestID  `json:"parent_quest,omitempty"`  // If this is a sub-quest
	SubQuests    []domain.QuestID `json:"sub_quests,omitempty"`    // If decomposed
	DecomposedBy *domain.AgentID  `json:"decomposed_by,omitempty"` // Party lead who broke it down

	// Assignment
	ClaimedBy     *domain.AgentID `json:"claimed_by,omitempty"`
	PartyID       *domain.PartyID `json:"party_id,omitempty"`
	GuildPriority *domain.GuildID `json:"guild_priority,omitempty"` // Guild gets first dibs

	// Lifecycle
	PostedAt    time.Time  `json:"posted_at"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`

	// Failure tracking
	Attempts      int         `json:"attempts"`
	MaxAttempts   int         `json:"max_attempts"`
	Escalated     bool        `json:"escalated"`
	FailureReason string      `json:"failure_reason,omitempty"`
	FailureType   FailureType `json:"failure_type,omitempty"`

	// Verdict (set on completion after boss battle)
	Verdict *BattleVerdict `json:"verdict,omitempty"`

	// Duration of quest execution (from start to completion)
	Duration time.Duration `json:"duration,omitempty"`

	// Observability - links back to semstreams
	TrajectoryID string `json:"trajectory_id"`
}

// QuestConstraints defines limits and requirements for quest execution.
type QuestConstraints struct {
	MaxDuration   time.Duration      `json:"max_duration"`
	MaxCost       float64            `json:"max_cost"`
	MaxTokens     int                `json:"max_tokens"`
	RequireReview bool               `json:"require_review"`
	ReviewLevel   domain.ReviewLevel `json:"review_level"`
}

// =============================================================================
// GRAPHABLE IMPLEMENTATION
// =============================================================================

// EntityID returns the 6-part entity ID for this quest.
func (q *Quest) EntityID() string {
	return string(q.ID)
}

// Triples returns all semantic facts about this quest.
func (q *Quest) Triples() []message.Triple {
	now := time.Now()
	source := "questboard"
	entityID := q.EntityID()

	triples := []message.Triple{
		// Identity
		{Subject: entityID, Predicate: "quest.identity.title", Object: q.Title, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.identity.description", Object: q.Description, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "quest.status.state", Object: string(q.Status), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.difficulty.level", Object: int(q.Difficulty), Source: source, Timestamp: now, Confidence: 1.0},

		// Requirements
		{Subject: entityID, Predicate: "quest.tier.minimum", Object: int(q.MinTier), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.party.required", Object: q.PartyRequired, Source: source, Timestamp: now, Confidence: 1.0},

		// Rewards
		{Subject: entityID, Predicate: "quest.xp.base", Object: q.BaseXP, Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "quest.attempts.current", Object: q.Attempts, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.attempts.max", Object: q.MaxAttempts, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.lifecycle.posted_at", Object: q.PostedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Add skills as separate triples
	for _, skill := range q.RequiredSkills {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.skill.required", Object: string(skill),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Add tools as separate triples
	for _, tool := range q.RequiredTools {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.tool.required", Object: tool,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Optional relationships
	if q.ClaimedBy != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.assignment.agent", Object: string(*q.ClaimedBy),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.PartyID != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.assignment.party", Object: string(*q.PartyID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.GuildPriority != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.priority.guild", Object: string(*q.GuildPriority),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.ParentQuest != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.parent.quest", Object: string(*q.ParentQuest),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.ClaimedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.lifecycle.claimed_at", Object: q.ClaimedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.StartedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.lifecycle.started_at", Object: q.StartedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.CompletedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.lifecycle.completed_at", Object: q.CompletedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.TrajectoryID != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.observability.trajectory_id", Object: q.TrajectoryID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Review
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "quest.review.level", Object: int(q.Constraints.ReviewLevel),
		Source: source, Timestamp: now, Confidence: 1.0,
	})
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "quest.review.needs_review", Object: q.Constraints.RequireReview,
		Source: source, Timestamp: now, Confidence: 1.0,
	})

	// Verdict (set on completion after boss battle)
	if q.Verdict != nil {
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: "quest.verdict.passed", Object: q.Verdict.Passed, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "quest.verdict.score", Object: q.Verdict.QualityScore, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "quest.verdict.xp_awarded", Object: q.Verdict.XPAwarded, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "quest.verdict.feedback", Object: q.Verdict.Feedback, Source: source, Timestamp: now, Confidence: 1.0},
		)
	}

	// Failure info
	if q.FailureReason != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.failure.reason", Object: q.FailureReason,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.FailureType != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.failure.type", Object: string(q.FailureType),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Duration
	if q.Duration > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.duration", Object: q.Duration.String(),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}
