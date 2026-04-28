// Package semsource registers the semsource entity payload type so that
// graph-ingest can deserialize entities streamed from a semsource instance.
//
// Call RegisterPayloads from the binary's bootstrap (alongside
// payloadbuiltins.Register) to register the entity + status payloads with the
// per-binary payload registry. semstreams beta.18 retired the package-level
// payloadregistry singleton, so registration is now explicit and DI-driven.
package semsource

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// Compile-time interface compliance checks.
var (
	_ graph.Graphable  = (*EntityPayload)(nil)
	_ message.Payload  = (*EntityPayload)(nil)
)

// RegisterPayloads registers the semsource entity and status payload types
// with the supplied registry. Called from the binary's bootstrap.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	if err := reg.Register(&payloadregistry.Registration{
		Domain:      "semsource",
		Category:    "entity",
		Version:     "v1",
		Description: "Entity streamed from a semsource ingestion instance",
		Factory: func() any {
			return &EntityPayload{}
		},
		Example: map[string]any{
			"id":         "org.platform.domain.system.type.instance",
			"triples":    []any{},
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		},
	}); err != nil {
		return err
	}

	// Register semsource status heartbeat so message-logger can parse it.
	// The payload is informational only — no component consumes it.
	return reg.Register(&payloadregistry.Registration{
		Domain:      "semsource",
		Category:    "status",
		Version:     "v1",
		Description: "Semsource instance status heartbeat",
		Factory: func() any {
			return &StatusPayload{}
		},
	})
}

// EntityPayload carries a graph entity received from semsource.
// It implements graph.Graphable so graph-ingest can persist it directly,
// and message.Payload so the component framework can deserialize it from wire.
type EntityPayload struct {
	// ID is the six-part federated entity identifier: org.platform.domain.system.type.instance
	ID string `json:"id"`

	// TripleData are the semantic facts that make up this entity's current state.
	TripleData []message.Triple `json:"triples"`

	// UpdatedAt records when the entity was last modified in semsource.
	UpdatedAt time.Time `json:"updated_at"`
}

// EntityID satisfies graph.Graphable — returns the federated entity ID.
func (p *EntityPayload) EntityID() string {
	return p.ID
}

// Triples satisfies graph.Graphable — returns the entity's semantic triples.
func (p *EntityPayload) Triples() []message.Triple {
	return p.TripleData
}

// Schema satisfies message.Payload — identifies this payload type in the registry.
func (p *EntityPayload) Schema() message.Type {
	return message.Type{Domain: "semsource", Category: "entity", Version: "v1"}
}

// Validate satisfies message.Payload — checks required fields are present.
func (p *EntityPayload) Validate() error {
	if p.ID == "" {
		return errors.New("semsource entity payload: id is required")
	}
	return nil
}

// MarshalJSON satisfies json.Marshaler for deterministic serialization.
func (p *EntityPayload) MarshalJSON() ([]byte, error) {
	type Alias EntityPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON satisfies json.Unmarshaler.
func (p *EntityPayload) UnmarshalJSON(data []byte) error {
	type Alias EntityPayload
	return json.Unmarshal(data, (*Alias)(p))
}

// StatusPayload is the semsource heartbeat message. Registered so the
// message-logger can parse it without "unregistered payload type" warnings.
type StatusPayload struct {
	Status    string `json:"status"`
	Sources   int    `json:"sources"`
	Entities  int    `json:"entities"`
	Uptime    string `json:"uptime,omitempty"`
}

// Schema returns the message type for semsource status heartbeats.
func (p *StatusPayload) Schema() message.Type {
	return message.Type{Domain: "semsource", Category: "status", Version: "v1"}
}

// Validate checks the payload for correctness.
func (p *StatusPayload) Validate() error { return nil }
