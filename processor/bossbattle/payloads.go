package bossbattle

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/questboard"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// BOSS BATTLE PAYLOADS
// =============================================================================
// Event payloads for boss battle lifecycle events. Each implements graph.Graphable
// for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*BattleStartedPayload)(nil)
	_ graph.Graphable = (*BattleVerdictPayload)(nil)
	_ graph.Graphable = (*BattleVictoryPayload)(nil)
	_ graph.Graphable = (*BattleDefeatPayload)(nil)
	_ graph.Graphable = (*BattleRetreatPayload)(nil)
)

// --- Typed Subjects ---

var (
	SubjectBattleStarted = natsclient.NewSubject[BattleStartedPayload](domain.PredicateBattleStarted)
	SubjectBattleVerdict = natsclient.NewSubject[BattleVerdictPayload](domain.PredicateBattleVerdict)
	SubjectBattleVictory = natsclient.NewSubject[BattleVictoryPayload](domain.PredicateBattleVictory)
	SubjectBattleDefeat  = natsclient.NewSubject[BattleDefeatPayload](domain.PredicateBattleDefeat)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// BATTLE STARTED PAYLOAD
// =============================================================================

// BattleStartedPayload contains data for battle.review.started events.
type BattleStartedPayload struct {
	Battle    BossBattle       `json:"battle"`
	Quest     questboard.Quest `json:"quest"`
	StartedAt time.Time        `json:"started_at"`
	Trace     TraceInfo        `json:"trace,omitempty"`
}

func (p *BattleStartedPayload) EntityID() string { return string(p.Battle.ID) }

func (p *BattleStartedPayload) Triples() []message.Triple {
	triples := p.Battle.Triples()
	source := "bossbattle"
	entityID := string(p.Battle.ID)

	// Add relationship to quest
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "battle.quest", Object: string(p.Quest.ID),
		Source: source, Timestamp: p.StartedAt, Confidence: 1.0,
	})

	return triples
}

func (p *BattleStartedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "battle.started", Version: "v1"}
}

func (p *BattleStartedPayload) Validate() error {
	if p.Battle.ID == "" {
		return errors.New("battle_id required")
	}
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.StartedAt.IsZero() {
		return errors.New("started_at required")
	}
	return nil
}

// =============================================================================
// BATTLE VERDICT PAYLOAD
// =============================================================================

// BattleVerdictPayload contains data for battle.review.verdict events.
type BattleVerdictPayload struct {
	Battle  BossBattle       `json:"battle"`
	Quest   questboard.Quest `json:"quest"`
	Verdict BattleVerdict    `json:"verdict"`
	EndedAt time.Time        `json:"ended_at"`
	Trace   TraceInfo        `json:"trace,omitempty"`
}

func (p *BattleVerdictPayload) EntityID() string { return string(p.Battle.ID) }

func (p *BattleVerdictPayload) Triples() []message.Triple {
	triples := p.Battle.Triples()
	source := "bossbattle"
	entityID := string(p.Battle.ID)

	// Add verdict details
	triples = append(triples,
		message.Triple{Subject: entityID, Predicate: "battle.verdict.final.passed", Object: p.Verdict.Passed, Source: source, Timestamp: p.EndedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.verdict.final.score", Object: p.Verdict.QualityScore, Source: source, Timestamp: p.EndedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.verdict.final.xp_awarded", Object: p.Verdict.XPAwarded, Source: source, Timestamp: p.EndedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.verdict.final.xp_penalty", Object: p.Verdict.XPPenalty, Source: source, Timestamp: p.EndedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.lifecycle.ended_at", Object: p.EndedAt.Format(time.RFC3339), Source: source, Timestamp: p.EndedAt, Confidence: 1.0},
	)

	return triples
}

func (p *BattleVerdictPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "battle.verdict", Version: "v1"}
}

func (p *BattleVerdictPayload) Validate() error {
	if p.Battle.ID == "" {
		return errors.New("battle_id required")
	}
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.EndedAt.IsZero() {
		return errors.New("ended_at required")
	}
	return nil
}

// =============================================================================
// BATTLE VICTORY PAYLOAD
// =============================================================================

// BattleVictoryPayload contains data for battle.review.victory events.
type BattleVictoryPayload struct {
	Battle       BossBattle       `json:"battle"`
	Quest        questboard.Quest `json:"quest"`
	Verdict      BattleVerdict    `json:"verdict"`
	XPAwarded    int64            `json:"xp_awarded"`
	LevelChange  int              `json:"level_change"`
	CompletedAt  time.Time        `json:"completed_at"`
	Trace        TraceInfo        `json:"trace,omitempty"`
}

func (p *BattleVictoryPayload) EntityID() string { return string(p.Battle.ID) }

func (p *BattleVictoryPayload) Triples() []message.Triple {
	triples := p.Battle.Triples()
	source := "bossbattle"
	entityID := string(p.Battle.ID)

	triples = append(triples,
		message.Triple{Subject: entityID, Predicate: "battle.outcome", Object: "victory", Source: source, Timestamp: p.CompletedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.xp_awarded", Object: p.XPAwarded, Source: source, Timestamp: p.CompletedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.level_change", Object: p.LevelChange, Source: source, Timestamp: p.CompletedAt, Confidence: 1.0},
	)

	return triples
}

func (p *BattleVictoryPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "battle.victory", Version: "v1"}
}

func (p *BattleVictoryPayload) Validate() error {
	if p.Battle.ID == "" {
		return errors.New("battle_id required")
	}
	if p.CompletedAt.IsZero() {
		return errors.New("completed_at required")
	}
	return nil
}

// =============================================================================
// BATTLE DEFEAT PAYLOAD
// =============================================================================

// BattleDefeatPayload contains data for battle.review.defeat events.
type BattleDefeatPayload struct {
	Battle      BossBattle       `json:"battle"`
	Quest       questboard.Quest `json:"quest"`
	Verdict     BattleVerdict    `json:"verdict"`
	XPPenalty   int64            `json:"xp_penalty"`
	Reason      string           `json:"reason"`
	CanRetry    bool             `json:"can_retry"`
	CompletedAt time.Time        `json:"completed_at"`
	Trace       TraceInfo        `json:"trace,omitempty"`
}

func (p *BattleDefeatPayload) EntityID() string { return string(p.Battle.ID) }

func (p *BattleDefeatPayload) Triples() []message.Triple {
	triples := p.Battle.Triples()
	source := "bossbattle"
	entityID := string(p.Battle.ID)

	triples = append(triples,
		message.Triple{Subject: entityID, Predicate: "battle.outcome", Object: "defeat", Source: source, Timestamp: p.CompletedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.xp_penalty", Object: p.XPPenalty, Source: source, Timestamp: p.CompletedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.defeat.reason", Object: p.Reason, Source: source, Timestamp: p.CompletedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.defeat.can_retry", Object: p.CanRetry, Source: source, Timestamp: p.CompletedAt, Confidence: 1.0},
	)

	return triples
}

func (p *BattleDefeatPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "battle.defeat", Version: "v1"}
}

func (p *BattleDefeatPayload) Validate() error {
	if p.Battle.ID == "" {
		return errors.New("battle_id required")
	}
	if p.CompletedAt.IsZero() {
		return errors.New("completed_at required")
	}
	return nil
}

// =============================================================================
// BATTLE RETREAT PAYLOAD
// =============================================================================

// BattleRetreatPayload contains data for agent retreat events.
type BattleRetreatPayload struct {
	Battle      BossBattle       `json:"battle"`
	Quest       questboard.Quest `json:"quest"`
	Reason      string           `json:"reason"`
	RetreatedAt time.Time        `json:"retreated_at"`
	Trace       TraceInfo        `json:"trace,omitempty"`
}

func (p *BattleRetreatPayload) EntityID() string { return string(p.Battle.ID) }

func (p *BattleRetreatPayload) Triples() []message.Triple {
	triples := p.Battle.Triples()
	source := "bossbattle"
	entityID := string(p.Battle.ID)

	triples = append(triples,
		message.Triple{Subject: entityID, Predicate: "battle.outcome", Object: "retreat", Source: source, Timestamp: p.RetreatedAt, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: "battle.retreat.reason", Object: p.Reason, Source: source, Timestamp: p.RetreatedAt, Confidence: 1.0},
	)

	return triples
}

func (p *BattleRetreatPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "battle.retreat", Version: "v1"}
}

func (p *BattleRetreatPayload) Validate() error {
	if p.Battle.ID == "" {
		return errors.New("battle_id required")
	}
	if p.RetreatedAt.IsZero() {
		return errors.New("retreated_at required")
	}
	return nil
}
