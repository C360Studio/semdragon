package executor

import (
	"testing"

	"github.com/c360studio/semstreams/agentic"

	"github.com/c360studio/semdragons/domain"
)

func TestBuildWebContentEntityTypesSourceRelationships(t *testing.T) {
	agentID := "c360.local.game.board.agent.agent-1"
	questID := "c360.local.game.board.quest.quest-1"
	entity := buildWebContentEntity(
		&domain.BoardConfig{Org: "c360", Platform: "local"},
		"https://example.com/page",
		"Example",
		"content",
		agentic.ToolCall{Metadata: map[string]any{"agent_id": agentID, "quest_id": questID}},
	)

	want := map[string]string{
		"source.doc.fetched_by": agentID,
		"source.doc.quest_id":   questID,
	}
	seen := make(map[string]int, len(want))
	for _, triple := range entity.Triples() {
		object, scalarString := triple.Object.(string)
		if scalarString && domain.IsValidEntityID(object) && triple.Datatype != "@id" {
			t.Errorf("entity-ID-shaped scalar %q on %s lacks @id", object, triple.Predicate)
		}
		if triple.Datatype == "@id" && (!scalarString || !domain.IsValidEntityID(object)) {
			t.Errorf("typed relationship %s has non-entity object %#v", triple.Predicate, triple.Object)
		}
		if object, ok := want[triple.Predicate]; ok {
			seen[triple.Predicate]++
			if triple.Object != object {
				t.Errorf("%s object = %#v, want %q", triple.Predicate, triple.Object, object)
			}
			if triple.Datatype != "@id" {
				t.Errorf("%s datatype = %q, want @id", triple.Predicate, triple.Datatype)
			}
		} else if triple.Datatype == "@id" {
			t.Errorf("non-edge %s unexpectedly carries @id", triple.Predicate)
		}
	}
	for predicate := range want {
		if seen[predicate] != 1 {
			t.Errorf("%s cardinality = %d, want 1", predicate, seen[predicate])
		}
	}
}
