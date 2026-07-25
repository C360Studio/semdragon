package questdag

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
)

func TestBuildUnitTriplesTypesGraphEdges(t *testing.T) {
	board := &domain.BoardConfig{Org: "c360", Platform: "local", Board: "board"}
	unitID := board.DagUnitEntityID("unit-b")
	dependencyID := board.DagUnitEntityID("unit-a")
	parentID := board.QuestEntityID("parent")
	subQuestID := board.QuestEntityID("sub-b")
	node := Node{ID: "b", DependsOn: []string{"a"}}
	triples := buildUnitTriples(
		unitID,
		node,
		map[string]string{"a": dependencyID, "b": unitID},
		SeedInput{
			ParentQuestID: parentID,
			ExecutionID:   "execution-1",
			NodeSubQuests: map[string]string{
				"b": subQuestID,
			},
		},
	)

	wantReferences := map[string]string{
		PredUnitParentQuest: parentID,
		PredUnitSubQuest:    subQuestID,
		PredDependsOn:       dependencyID,
	}
	seen := make(map[string]int, len(wantReferences))
	for _, triple := range triples {
		object, scalarString := triple.Object.(string)
		if scalarString && domain.IsValidEntityID(object) && triple.Datatype != "@id" {
			t.Errorf("entity-ID-shaped scalar %q on %s lacks @id", object, triple.Predicate)
		}
		if triple.Datatype == "@id" && (!scalarString || !domain.IsValidEntityID(object)) {
			t.Errorf("typed relationship %s has non-entity object %#v", triple.Predicate, triple.Object)
		}
		if want, ok := wantReferences[triple.Predicate]; ok {
			seen[triple.Predicate]++
			if triple.Object != want {
				t.Errorf("%s object = %#v, want %q", triple.Predicate, triple.Object, want)
			}
			if triple.Datatype != "@id" {
				t.Errorf("%s datatype = %q, want @id", triple.Predicate, triple.Datatype)
			}
			continue
		}
		if triple.Datatype == "@id" {
			t.Errorf("non-edge %s unexpectedly carries @id", triple.Predicate)
		}
	}
	for predicate := range wantReferences {
		if seen[predicate] != 1 {
			t.Errorf("%s cardinality = %d, want 1", predicate, seen[predicate])
		}
	}
}
