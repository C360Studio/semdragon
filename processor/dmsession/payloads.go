package dmsession

import (
	"fmt"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// SESSION EVENT PAYLOADS
// =============================================================================

// SessionStartPayload contains data for dm.session.start events.
type SessionStartPayload struct {
	SessionID string               `json:"session_id"`
	Config    domain.SessionConfig `json:"config"`
	StartedAt time.Time            `json:"started_at"`
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
