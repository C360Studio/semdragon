package semsource

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// Compile-time interface compliance — mirrors the checks in payload.go but lives
// in the test file so the compiler confirms them independently of the production
// build tag.
var (
	_ graph.Graphable = (*EntityPayload)(nil)
	_ message.Payload = (*EntityPayload)(nil)
)

// TestEntityPayloadGraphable verifies EntityPayload satisfies graph.Graphable.
func TestEntityPayloadGraphable(t *testing.T) {
	t.Parallel()

	triples := []message.Triple{
		{Subject: "c360.prod.game.board1.quest.abc123", Predicate: "quest.lifecycle.posted", Object: "true"},
		{Subject: "c360.prod.game.board1.quest.abc123", Predicate: "quest.data.title", Object: "Fix the dragon"},
	}

	p := &EntityPayload{
		ID:         "c360.prod.game.board1.quest.abc123",
		TripleData: triples,
		UpdatedAt:  time.Now().UTC(),
	}

	t.Run("EntityID returns ID", func(t *testing.T) {
		t.Parallel()
		got := p.EntityID()
		if got != p.ID {
			t.Errorf("EntityID() = %q, want %q", got, p.ID)
		}
	})

	t.Run("Triples returns TripleData", func(t *testing.T) {
		t.Parallel()
		got := p.Triples()
		if len(got) != len(triples) {
			t.Fatalf("Triples() returned %d triples, want %d", len(got), len(triples))
		}
		for i, triple := range got {
			if triple.Subject != triples[i].Subject {
				t.Errorf("triple[%d].Subject = %q, want %q", i, triple.Subject, triples[i].Subject)
			}
			if triple.Predicate != triples[i].Predicate {
				t.Errorf("triple[%d].Predicate = %q, want %q", i, triple.Predicate, triples[i].Predicate)
			}
		}
	})

	t.Run("Triples returns nil for empty payload", func(t *testing.T) {
		t.Parallel()
		empty := &EntityPayload{ID: "c360.prod.game.board1.agent.dragon"}
		if got := empty.Triples(); got != nil {
			t.Errorf("Triples() for empty payload = %v, want nil", got)
		}
	})
}

// TestEntityPayloadSchema verifies Schema returns the correct message.Type.
func TestEntityPayloadSchema(t *testing.T) {
	t.Parallel()

	p := &EntityPayload{}
	schema := p.Schema()

	if schema.Domain != "semsource" {
		t.Errorf("Schema().Domain = %q, want %q", schema.Domain, "semsource")
	}
	if schema.Category != "entity" {
		t.Errorf("Schema().Category = %q, want %q", schema.Category, "entity")
	}
	if schema.Version != "v1" {
		t.Errorf("Schema().Version = %q, want %q", schema.Version, "v1")
	}
}

// TestEntityPayloadValidate verifies Validate returns nil for valid payloads and
// an error when ID is empty.
func TestEntityPayloadValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid payload passes", func(t *testing.T) {
		t.Parallel()
		p := &EntityPayload{
			ID:        "c360.prod.game.board1.agent.dragon",
			UpdatedAt: time.Now().UTC(),
		}
		if err := p.Validate(); err != nil {
			t.Errorf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("empty ID fails validation", func(t *testing.T) {
		t.Parallel()
		p := &EntityPayload{}
		err := p.Validate()
		if err == nil {
			t.Fatal("Validate() expected error for empty ID, got nil")
		}
		// Error message should identify the problem clearly.
		want := "id is required"
		if msg := err.Error(); len(msg) == 0 {
			t.Error("Validate() error message is empty")
		} else if !contains(msg, want) {
			t.Errorf("Validate() error = %q, want it to mention %q", msg, want)
		}
	})
}

// TestEntityPayloadJSONRoundTrip verifies that marshalling then unmarshalling an
// EntityPayload preserves all fields exactly.
func TestEntityPayloadJSONRoundTrip(t *testing.T) {
	t.Parallel()

	// Use a fixed timestamp truncated to seconds for deterministic comparison —
	// JSON RFC3339 format does not preserve sub-second precision in all encoders.
	ts := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	original := &EntityPayload{
		ID: "c360.prod.game.board1.guild.datawranglers",
		TripleData: []message.Triple{
			{
				Subject:   "c360.prod.game.board1.guild.datawranglers",
				Predicate: "guild.membership.joined",
				Object:    "c360.prod.game.board1.agent.dragon",
			},
			{
				Subject:   "c360.prod.game.board1.guild.datawranglers",
				Predicate: "guild.stats.member_count",
				Object:    float64(3),
			},
		},
		UpdatedAt: ts,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored EntityPayload
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID: got %q, want %q", restored.ID, original.ID)
	}
	if !restored.UpdatedAt.Equal(original.UpdatedAt) {
		t.Errorf("UpdatedAt: got %v, want %v", restored.UpdatedAt, original.UpdatedAt)
	}
	if len(restored.TripleData) != len(original.TripleData) {
		t.Fatalf("TripleData length: got %d, want %d", len(restored.TripleData), len(original.TripleData))
	}
	for i, orig := range original.TripleData {
		got := restored.TripleData[i]
		if got.Subject != orig.Subject {
			t.Errorf("triple[%d].Subject: got %q, want %q", i, got.Subject, orig.Subject)
		}
		if got.Predicate != orig.Predicate {
			t.Errorf("triple[%d].Predicate: got %q, want %q", i, got.Predicate, orig.Predicate)
		}
		// Object is unmarshalled as float64 for numeric JSON values — check via
		// JSON re-encoding for a stable comparison.
		origObj, _ := json.Marshal(orig.Object)
		gotObj, _ := json.Marshal(got.Object)
		if string(origObj) != string(gotObj) {
			t.Errorf("triple[%d].Object: got %s, want %s", i, gotObj, origObj)
		}
	}
}

// TestEntityPayloadRegistration verifies that RegisterPayloads registers the
// semsource entity payload (and its factory) with a fresh registry.
func TestEntityPayloadRegistration(t *testing.T) {
	t.Parallel()

	reg := payloadregistry.New()
	if err := RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads: %v", err)
	}

	// The registry key format used internally is "domain.category.version".
	const expectedKey = "semsource.entity.v1"
	registration, ok := reg.GetRegistration(expectedKey)
	if !ok {
		t.Fatalf("payload %q not found after RegisterPayloads", expectedKey)
	}

	if registration.Domain != "semsource" {
		t.Errorf("registration.Domain = %q, want %q", registration.Domain, "semsource")
	}
	if registration.Category != "entity" {
		t.Errorf("registration.Category = %q, want %q", registration.Category, "entity")
	}
	if registration.Version != "v1" {
		t.Errorf("registration.Version = %q, want %q", registration.Version, "v1")
	}
	// GetRegistration intentionally strips the Factory field for safety.
	// Use Registry.Create to verify the factory produces the correct type.
	instance := reg.Create("semsource", "entity", "v1")
	if instance == nil {
		t.Fatal("Registry.Create returned nil; factory not registered correctly")
	}
	if _, ok := instance.(*EntityPayload); !ok {
		t.Errorf("Registry.Create returned %T, want *EntityPayload", instance)
	}
}

// contains is a small helper to avoid importing strings in test assertions.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
