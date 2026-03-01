package autonomy

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
// AUTONOMY PAYLOADS
// =============================================================================
// Event payloads for autonomy heartbeat operations. Each implements
// graph.Graphable for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations.
var (
	_ graph.Graphable = (*EvaluatedPayload)(nil)
	_ graph.Graphable = (*IdlePayload)(nil)
)

// Typed subjects for autonomy events.
var (
	SubjectAutonomyEvaluated = natsclient.NewSubject[EvaluatedPayload](domain.PredicateAutonomyEvaluated)
	SubjectAutonomyIdle      = natsclient.NewSubject[IdlePayload](domain.PredicateAutonomyIdle)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// AUTONOMY EVALUATED PAYLOAD
// =============================================================================

// EvaluatedPayload is emitted on every heartbeat evaluation.
type EvaluatedPayload struct {
	AgentID     domain.AgentID     `json:"agent_id"`
	AgentStatus domain.AgentStatus `json:"agent_status"`
	ActionTaken string             `json:"action_taken"` // "none", "cooldown_expired", or action name
	Interval    time.Duration      `json:"interval"`     // Current heartbeat interval
	Timestamp   time.Time          `json:"timestamp"`
	Trace       TraceInfo          `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *EvaluatedPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *EvaluatedPayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.autonomy.status", Object: string(p.AgentStatus), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.action", Object: p.ActionTaken, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.interval_ms", Object: p.Interval.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *EvaluatedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.evaluated", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *EvaluatedPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// AUTONOMY IDLE PAYLOAD
// =============================================================================

// IdlePayload is emitted when an idle agent has nothing actionable.
type IdlePayload struct {
	AgentID       domain.AgentID `json:"agent_id"`
	IdleDuration  time.Duration  `json:"idle_duration"`
	HasSuggestion bool           `json:"has_suggestion"`
	BackoffMs     int64          `json:"backoff_ms"` // Next interval after backoff
	Timestamp     time.Time      `json:"timestamp"`
	Trace         TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *IdlePayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *IdlePayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.autonomy.idle_duration_ms", Object: p.IdleDuration.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.has_suggestion", Object: p.HasSuggestion, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.backoff_ms", Object: p.BackoffMs, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *IdlePayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.idle", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *IdlePayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
