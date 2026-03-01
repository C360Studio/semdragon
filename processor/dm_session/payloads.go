package dm_session

import (
	"fmt"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// SESSION EVENT PAYLOADS
// =============================================================================

// Ensure payloads implement Graphable.
var (
	_ graph.Graphable = (*SessionStartPayload)(nil)
	_ graph.Graphable = (*SessionEndPayload)(nil)
)

// SessionStartPayload contains data for dm.session.start events.
type SessionStartPayload struct {
	SessionID string               `json:"session_id"`
	Config    domain.SessionConfig `json:"config"`
	StartedAt time.Time            `json:"started_at"`
}

// EntityID returns the entity ID for this payload.
func (p *SessionStartPayload) EntityID() string {
	return p.SessionID
}

// Triples returns semantic triples for this payload.
func (p *SessionStartPayload) Triples() []message.Triple {
	return []message.Triple{
		{Subject: p.SessionID, Predicate: "dm.session.mode", Object: string(p.Config.Mode)},
		{Subject: p.SessionID, Predicate: "dm.session.name", Object: p.Config.Name},
		{Subject: p.SessionID, Predicate: domain.PredicateSessionStart, Object: p.StartedAt.Format(time.RFC3339)},
	}
}

// Schema returns the schema type.
func (p *SessionStartPayload) Schema() string {
	return "session.start"
}

// Validate checks that required fields are present.
func (p *SessionStartPayload) Validate() error {
	if p.SessionID == "" {
		return fmt.Errorf("session_id required")
	}
	if p.StartedAt.IsZero() {
		return fmt.Errorf("started_at required")
	}
	return nil
}

// SessionEndPayload contains data for dm.session.end events.
type SessionEndPayload struct {
	SessionID string                `json:"session_id"`
	Summary   domain.SessionSummary `json:"summary"`
	EndedAt   time.Time             `json:"ended_at"`
}

// EntityID returns the entity ID for this payload.
func (p *SessionEndPayload) EntityID() string {
	return p.SessionID
}

// Triples returns semantic triples for this payload.
func (p *SessionEndPayload) Triples() []message.Triple {
	return []message.Triple{
		{Subject: p.SessionID, Predicate: domain.PredicateSessionEnd, Object: p.EndedAt.Format(time.RFC3339)},
		{Subject: p.SessionID, Predicate: "dm.session.quests_completed", Object: fmt.Sprintf("%d", p.Summary.QuestsCompleted)},
		{Subject: p.SessionID, Predicate: "dm.session.quests_failed", Object: fmt.Sprintf("%d", p.Summary.QuestsFailed)},
	}
}

// Schema returns the schema type.
func (p *SessionEndPayload) Schema() string {
	return "session.end"
}

// Validate checks that required fields are present.
func (p *SessionEndPayload) Validate() error {
	if p.SessionID == "" {
		return fmt.Errorf("session_id required")
	}
	if p.EndedAt.IsZero() {
		return fmt.Errorf("ended_at required")
	}
	return nil
}

// =============================================================================
// TYPED SUBJECTS
// =============================================================================

// Typed subjects for session events.
var (
	SubjectSessionStart = natsclient.NewSubject[SessionStartPayload](domain.PredicateSessionStart)
	SubjectSessionEnd   = natsclient.NewSubject[SessionEndPayload](domain.PredicateSessionEnd)
)
