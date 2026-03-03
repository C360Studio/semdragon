package bossbattle

import (
	"errors"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/types"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// BOSS BATTLE PAYLOADS
// =============================================================================
// Event payloads for boss battle lifecycle events. Each implements graph.Graphable
// for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*BattleRetreatPayload)(nil)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// BATTLE RETREAT PAYLOAD
// =============================================================================

// BattleRetreatPayload contains data for agent retreat events.
type BattleRetreatPayload struct {
	Battle      BossBattle   `json:"battle"`
	Quest       domain.Quest `json:"quest"`
	Reason      string       `json:"reason"`
	RetreatedAt time.Time    `json:"retreated_at"`
	Trace       TraceInfo    `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *BattleRetreatPayload) EntityID() string { return string(p.Battle.ID) }

// Triples returns semantic triples for this event.
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

// Schema returns the type schema for this payload.
func (p *BattleRetreatPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "battle.retreat", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *BattleRetreatPayload) Validate() error {
	if p.Battle.ID == "" {
		return errors.New("battle_id required")
	}
	if p.RetreatedAt.IsZero() {
		return errors.New("retreated_at required")
	}
	return nil
}
