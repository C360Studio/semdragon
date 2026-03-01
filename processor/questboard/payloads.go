package questboard

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// QUEST LIFECYCLE PAYLOADS
// =============================================================================
// Event payloads for quest lifecycle events. Each implements graph.Graphable
// for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*QuestPostedPayload)(nil)
	_ graph.Graphable = (*QuestClaimedPayload)(nil)
	_ graph.Graphable = (*QuestStartedPayload)(nil)
	_ graph.Graphable = (*QuestSubmittedPayload)(nil)
	_ graph.Graphable = (*QuestCompletedPayload)(nil)
	_ graph.Graphable = (*QuestFailedPayload)(nil)
	_ graph.Graphable = (*QuestEscalatedPayload)(nil)
	_ graph.Graphable = (*QuestAbandonedPayload)(nil)
)

// --- Typed Subjects ---

var (
	SubjectQuestPosted    = natsclient.NewSubject[QuestPostedPayload](domain.PredicateQuestPosted)
	SubjectQuestClaimed   = natsclient.NewSubject[QuestClaimedPayload](domain.PredicateQuestClaimed)
	SubjectQuestStarted   = natsclient.NewSubject[QuestStartedPayload](domain.PredicateQuestStarted)
	SubjectQuestSubmitted = natsclient.NewSubject[QuestSubmittedPayload](domain.PredicateQuestSubmitted)
	SubjectQuestCompleted = natsclient.NewSubject[QuestCompletedPayload](domain.PredicateQuestCompleted)
	SubjectQuestFailed    = natsclient.NewSubject[QuestFailedPayload](domain.PredicateQuestFailed)
	SubjectQuestEscalated = natsclient.NewSubject[QuestEscalatedPayload](domain.PredicateQuestEscalated)
	SubjectQuestAbandoned = natsclient.NewSubject[QuestAbandonedPayload](domain.PredicateQuestAbandoned)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// --- QuestPostedPayload ---

// QuestPostedPayload contains data for quest.lifecycle.posted events.
type QuestPostedPayload struct {
	Quest    Quest     `json:"quest"`
	PostedAt time.Time `json:"posted_at"`
	PostedBy string    `json:"posted_by,omitempty"`
	Trace    TraceInfo `json:"trace,omitempty"`
}

func (p *QuestPostedPayload) EntityID() string { return string(p.Quest.ID) }

func (p *QuestPostedPayload) Triples() []message.Triple {
	return questToTriples(&p.Quest, p.PostedAt)
}

func (p *QuestPostedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.posted", Version: "v1"}
}

func (p *QuestPostedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.PostedAt.IsZero() {
		return errors.New("posted_at required")
	}
	return nil
}

// --- QuestClaimedPayload ---

// QuestClaimedPayload contains data for quest.lifecycle.claimed events.
type QuestClaimedPayload struct {
	Quest     Quest           `json:"quest"`
	AgentID   domain.AgentID  `json:"agent_id"`
	PartyID   *domain.PartyID `json:"party_id,omitempty"`
	ClaimedAt time.Time       `json:"claimed_at"`
	Trace     TraceInfo       `json:"trace,omitempty"`
}

func (p *QuestClaimedPayload) EntityID() string { return string(p.Quest.ID) }

func (p *QuestClaimedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.ClaimedAt)
	id := string(p.Quest.ID)

	triples = append(triples, message.Triple{
		Subject: id, Predicate: "quest.assignment.claimed_by", Object: string(p.AgentID),
		Timestamp: p.ClaimedAt, Confidence: 1.0, Source: "questboard",
	})

	if p.PartyID != nil {
		triples = append(triples, message.Triple{
			Subject: id, Predicate: "quest.assignment.party_id", Object: string(*p.PartyID),
			Timestamp: p.ClaimedAt, Confidence: 1.0, Source: "questboard",
		})
	}

	return triples
}

func (p *QuestClaimedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.claimed", Version: "v1"}
}

func (p *QuestClaimedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.AgentID == "" && p.PartyID == nil {
		return errors.New("agent_id or party_id required")
	}
	if p.ClaimedAt.IsZero() {
		return errors.New("claimed_at required")
	}
	return nil
}

// --- QuestStartedPayload ---

// QuestStartedPayload contains data for quest.lifecycle.started events.
type QuestStartedPayload struct {
	Quest     Quest           `json:"quest"`
	AgentID   domain.AgentID  `json:"agent_id"`
	PartyID   *domain.PartyID `json:"party_id,omitempty"`
	StartedAt time.Time       `json:"started_at"`
	Trace     TraceInfo       `json:"trace,omitempty"`
}

func (p *QuestStartedPayload) EntityID() string { return string(p.Quest.ID) }

func (p *QuestStartedPayload) Triples() []message.Triple {
	return questToTriples(&p.Quest, p.StartedAt)
}

func (p *QuestStartedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.started", Version: "v1"}
}

func (p *QuestStartedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.StartedAt.IsZero() {
		return errors.New("started_at required")
	}
	return nil
}

// --- QuestSubmittedPayload ---

// QuestSubmittedPayload contains data for quest.lifecycle.submitted events.
type QuestSubmittedPayload struct {
	Quest       Quest              `json:"quest"`
	AgentID     domain.AgentID     `json:"agent_id"`
	Result      any                `json:"result"`
	SubmittedAt time.Time          `json:"submitted_at"`
	NeedsReview bool               `json:"needs_review"`
	ReviewLevel domain.ReviewLevel `json:"review_level,omitempty"`
	Trace       TraceInfo          `json:"trace,omitempty"`
}

func (p *QuestSubmittedPayload) EntityID() string { return string(p.Quest.ID) }

func (p *QuestSubmittedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.SubmittedAt)
	id := string(p.Quest.ID)

	triples = append(triples, message.Triple{
		Subject: id, Predicate: "quest.review.needs_review", Object: p.NeedsReview,
		Timestamp: p.SubmittedAt, Confidence: 1.0, Source: "questboard",
	})

	if p.NeedsReview {
		triples = append(triples, message.Triple{
			Subject: id, Predicate: "quest.review.level", Object: int(p.ReviewLevel),
			Timestamp: p.SubmittedAt, Confidence: 1.0, Source: "questboard",
		})
	}

	return triples
}

func (p *QuestSubmittedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.submitted", Version: "v1"}
}

func (p *QuestSubmittedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.SubmittedAt.IsZero() {
		return errors.New("submitted_at required")
	}
	return nil
}

// --- QuestCompletedPayload ---

// BattleVerdict holds the outcome of a boss battle.
type BattleVerdict struct {
	Passed       bool    `json:"passed"`
	QualityScore float64 `json:"quality_score"`
	XPAwarded    int64   `json:"xp_awarded"`
	XPPenalty    int64   `json:"xp_penalty"`
	Feedback     string  `json:"feedback"`
	LevelChange  int     `json:"level_change"`
}

// AgentContext contains agent state needed for downstream processing.
// Embedded in events so handlers don't need to fetch state (fat events pattern).
type AgentContext struct {
	Level     int               `json:"level"`
	XP        int64             `json:"xp"`
	XPToLevel int64             `json:"xp_to_level"`
	Tier      domain.TrustTier  `json:"tier"`
	Streak    int               `json:"streak"`
	Guilds    []domain.GuildID  `json:"guilds,omitempty"`
	GuildRank domain.GuildRank  `json:"guild_rank,omitempty"` // Rank in priority guild if applicable
	Stats     AgentContextStats `json:"stats"`
}

// AgentContextStats contains stats relevant for XP calculation.
type AgentContextStats struct {
	QuestsCompleted int `json:"quests_completed"`
	QuestsFailed    int `json:"quests_failed"`
}

// QuestCompletedPayload contains data for quest.lifecycle.completed events.
// This is a "fat event" - it includes all context needed for downstream handlers.
type QuestCompletedPayload struct {
	Quest        Quest           `json:"quest"`
	AgentID      domain.AgentID  `json:"agent_id"`
	AgentContext AgentContext    `json:"agent_context"` // Agent state at completion time
	PartyID      *domain.PartyID `json:"party_id,omitempty"`
	Verdict      BattleVerdict   `json:"verdict"`
	CompletedAt  time.Time       `json:"completed_at"`
	Duration     time.Duration   `json:"duration"`
	Trace        TraceInfo       `json:"trace,omitempty"`
}

func (p *QuestCompletedPayload) EntityID() string { return string(p.Quest.ID) }

func (p *QuestCompletedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.CompletedAt)
	id := string(p.Quest.ID)

	triples = append(triples,
		message.Triple{Subject: id, Predicate: "quest.verdict.passed", Object: p.Verdict.Passed, Timestamp: p.CompletedAt, Confidence: 1.0, Source: "questboard"},
		message.Triple{Subject: id, Predicate: "quest.verdict.score", Object: p.Verdict.QualityScore, Timestamp: p.CompletedAt, Confidence: 1.0, Source: "questboard"},
		message.Triple{Subject: id, Predicate: "quest.verdict.xp_awarded", Object: p.Verdict.XPAwarded, Timestamp: p.CompletedAt, Confidence: 1.0, Source: "questboard"},
		message.Triple{Subject: id, Predicate: "quest.duration", Object: p.Duration.String(), Timestamp: p.CompletedAt, Confidence: 1.0, Source: "questboard"},
	)

	return triples
}

func (p *QuestCompletedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.completed", Version: "v1"}
}

func (p *QuestCompletedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.CompletedAt.IsZero() {
		return errors.New("completed_at required")
	}
	return nil
}

// --- QuestFailedPayload ---

// FailureType categorizes quest failures.
type FailureType string

const (
	FailureQuality   FailureType = "quality"
	FailureTimeout   FailureType = "timeout"
	FailureError     FailureType = "error"
	FailureAbandoned FailureType = "abandoned"
)

// QuestFailedPayload contains data for quest.lifecycle.failed events.
// This is a "fat event" - includes agent context for penalty calculation.
type QuestFailedPayload struct {
	Quest        Quest           `json:"quest"`
	AgentID      domain.AgentID  `json:"agent_id"`
	AgentContext AgentContext    `json:"agent_context"` // Agent state at failure time
	PartyID      *domain.PartyID `json:"party_id,omitempty"`
	Reason       string          `json:"reason"`
	FailType     FailureType     `json:"fail_type"`
	FailedAt     time.Time       `json:"failed_at"`
	Attempt      int             `json:"attempt"`
	Reposted     bool            `json:"reposted"`
	Trace        TraceInfo       `json:"trace,omitempty"`
}

func (p *QuestFailedPayload) EntityID() string { return string(p.Quest.ID) }

func (p *QuestFailedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.FailedAt)
	id := string(p.Quest.ID)

	triples = append(triples,
		message.Triple{Subject: id, Predicate: "quest.failure.reason", Object: p.Reason, Timestamp: p.FailedAt, Confidence: 1.0, Source: "questboard"},
		message.Triple{Subject: id, Predicate: "quest.failure.type", Object: string(p.FailType), Timestamp: p.FailedAt, Confidence: 1.0, Source: "questboard"},
	)

	return triples
}

func (p *QuestFailedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.failed", Version: "v1"}
}

func (p *QuestFailedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.FailedAt.IsZero() {
		return errors.New("failed_at required")
	}
	return nil
}

// --- QuestEscalatedPayload ---

// QuestEscalatedPayload contains data for quest.lifecycle.escalated events.
type QuestEscalatedPayload struct {
	Quest       Quest           `json:"quest"`
	AgentID     domain.AgentID  `json:"agent_id"`
	PartyID     *domain.PartyID `json:"party_id,omitempty"`
	Reason      string          `json:"reason"`
	EscalatedAt time.Time       `json:"escalated_at"`
	Attempts    int             `json:"attempts"`
	Trace       TraceInfo       `json:"trace,omitempty"`
}

func (p *QuestEscalatedPayload) EntityID() string { return string(p.Quest.ID) }

func (p *QuestEscalatedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.EscalatedAt)
	id := string(p.Quest.ID)

	triples = append(triples,
		message.Triple{Subject: id, Predicate: "quest.escalation.reason", Object: p.Reason, Timestamp: p.EscalatedAt, Confidence: 1.0, Source: "questboard"},
	)

	return triples
}

func (p *QuestEscalatedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.escalated", Version: "v1"}
}

func (p *QuestEscalatedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.EscalatedAt.IsZero() {
		return errors.New("escalated_at required")
	}
	return nil
}

// --- QuestAbandonedPayload ---

// QuestAbandonedPayload contains data for quest.lifecycle.abandoned events.
type QuestAbandonedPayload struct {
	Quest       Quest           `json:"quest"`
	AgentID     domain.AgentID  `json:"agent_id"`
	PartyID     *domain.PartyID `json:"party_id,omitempty"`
	Reason      string          `json:"reason"`
	AbandonedAt time.Time       `json:"abandoned_at"`
	Trace       TraceInfo       `json:"trace,omitempty"`
}

func (p *QuestAbandonedPayload) EntityID() string { return string(p.Quest.ID) }

func (p *QuestAbandonedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.AbandonedAt)
	id := string(p.Quest.ID)

	triples = append(triples,
		message.Triple{Subject: id, Predicate: "quest.abandonment.reason", Object: p.Reason, Timestamp: p.AbandonedAt, Confidence: 1.0, Source: "questboard"},
	)

	return triples
}

func (p *QuestAbandonedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.abandoned", Version: "v1"}
}

func (p *QuestAbandonedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.AbandonedAt.IsZero() {
		return errors.New("abandoned_at required")
	}
	return nil
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// questToTriples converts a Quest to triples at a given timestamp.
func questToTriples(q *Quest, ts time.Time) []message.Triple {
	source := "questboard"
	entityID := string(q.ID)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "quest.identity.title", Object: q.Title, Source: source, Timestamp: ts, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.identity.description", Object: q.Description, Source: source, Timestamp: ts, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.status.state", Object: string(q.Status), Source: source, Timestamp: ts, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.difficulty.level", Object: int(q.Difficulty), Source: source, Timestamp: ts, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.tier.minimum", Object: int(q.MinTier), Source: source, Timestamp: ts, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.party.required", Object: q.PartyRequired, Source: source, Timestamp: ts, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.xp.base", Object: q.BaseXP, Source: source, Timestamp: ts, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.attempts.current", Object: q.Attempts, Source: source, Timestamp: ts, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.attempts.max", Object: q.MaxAttempts, Source: source, Timestamp: ts, Confidence: 1.0},
	}

	for _, skill := range q.RequiredSkills {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.skill.required", Object: string(skill),
			Source: source, Timestamp: ts, Confidence: 1.0,
		})
	}

	return triples
}
