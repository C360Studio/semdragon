package executor

import (
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/vocabulary"

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

	want := map[string]any{
		"source.doc.content":    "content",
		"source.doc.summary":    "content",
		"source.doc.url":        "https://example.com/page",
		"source.doc.mime-type":  "text/html",
		"source.doc.scope":      "all",
		vocabulary.DCTermsTitle: "Example",
		"source.doc.fetched-by": agentID,
		"source.doc.quest-id":   questID,
	}
	if len(entity.Triples()) != len(want)+1 {
		t.Fatalf("web content triple count = %d, want %d", len(entity.Triples()), len(want)+1)
	}
	seen := make(map[string]int, len(want))
	referenceCount := 0
	for _, triple := range entity.Triples() {
		object, scalarString := triple.Object.(string)
		if scalarString && domain.IsValidEntityID(object) && triple.Datatype != "@id" {
			t.Errorf("entity-ID-shaped scalar %q on %s lacks @id", object, triple.Predicate)
		}
		if triple.Datatype == "@id" && (!scalarString || !domain.IsValidEntityID(object)) {
			t.Errorf("typed relationship %s has non-entity object %#v", triple.Predicate, triple.Object)
		}
		if triple.Predicate == "dc.terms.modified" {
			if !scalarString {
				t.Errorf("dc.terms.modified object = %#v, want RFC3339 string", triple.Object)
			} else if _, err := time.Parse(time.RFC3339, object); err != nil {
				t.Errorf("dc.terms.modified object = %q, want RFC3339: %v", object, err)
			}
			if triple.Datatype != "" {
				t.Errorf("dc.terms.modified datatype = %q, want scalar", triple.Datatype)
			}
			continue
		}
		if object, ok := want[triple.Predicate]; ok {
			seen[triple.Predicate]++
			if !reflect.DeepEqual(triple.Object, object) {
				t.Errorf("%s object = %#v, want %#v", triple.Predicate, triple.Object, object)
			}
			wantReference := triple.Predicate == "source.doc.fetched-by" || triple.Predicate == "source.doc.quest-id"
			if wantReference {
				referenceCount++
				if triple.Datatype != "@id" {
					t.Errorf("%s datatype = %q, want @id", triple.Predicate, triple.Datatype)
				}
			} else if triple.Datatype != "" {
				t.Errorf("%s datatype = %q, want scalar", triple.Predicate, triple.Datatype)
			}
		} else if triple.Datatype == "@id" {
			t.Errorf("non-edge %s unexpectedly carries @id", triple.Predicate)
		} else {
			t.Errorf("unexpected web content predicate %q", triple.Predicate)
		}
	}
	for predicate := range want {
		if seen[predicate] != 1 {
			t.Errorf("%s cardinality = %d, want 1", predicate, seen[predicate])
		}
	}
	if referenceCount != 2 {
		t.Errorf("web content @id relationships = %d, want 2", referenceCount)
	}
}
