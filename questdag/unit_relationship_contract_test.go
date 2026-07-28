package questdag

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
)

func TestUnitPredicatesMatchSemStreamsBeta158Contract(t *testing.T) {
	want := map[string]string{
		"claim":        "gateddag.unit.claim",
		"completed":    "gateddag.unit.completed",
		"failed":       "gateddag.unit.failed",
		"dirtied":      "gateddag.unit.dirtied",
		"depends-on":   "gateddag.unit.depends-on",
		"node-id":      "questdag.unit.node-id",
		"parent-quest": "questdag.unit.parent-quest",
		"execution-id": "questdag.unit.execution-id",
		"sub-quest":    "questdag.unit.sub-quest",
		"plan-digest":  "questdag.unit.plan-digest",
	}
	got := map[string]string{
		"claim":        frameworkGatedDAGDefaults().ClaimPredicate,
		"completed":    frameworkGatedDAGDefaults().CompletedPredicate,
		"failed":       frameworkGatedDAGDefaults().FailedPredicate,
		"dirtied":      frameworkGatedDAGDefaults().DirtiedPredicate,
		"depends-on":   frameworkGatedDAGDefaults().DependsOnPredicate,
		"node-id":      PredUnitNodeID,
		"parent-quest": PredUnitParentQuest,
		"execution-id": PredUnitExecutionID,
		"sub-quest":    PredUnitSubQuest,
		"plan-digest":  PredUnitPlanDigest,
	}
	for name, wantPredicate := range want {
		if gotPredicate := got[name]; gotPredicate != wantPredicate {
			t.Errorf("%s predicate = %q, want %q", name, gotPredicate, wantPredicate)
		}
	}
}

func TestUnitRelationshipContract(t *testing.T) {
	const (
		unitID       = "c360.local.game.board.dag-unit.unit-1"
		parentQuest  = "c360.local.game.board.quest.parent-quest"
		subQuest     = "c360.local.game.board.quest.sub-quest"
		dependencyID = "c360.local.game.board.dag-unit.dependency-1"
	)

	triples := baseUnitTriples(
		unitID,
		"node-1",
		parentQuest,
		"execution-1",
		"digest-1",
		subQuest,
	)
	triples = append(triples, referenceTriple(unitID, dependencyID))

	expected := map[string]string{
		PredUnitParentQuest: parentQuest,
		PredUnitSubQuest:    subQuest,
		frameworkGatedDAGDefaults().DependsOnPredicate: dependencyID,
	}
	observed := make(map[string]int, len(expected))

	for _, triple := range triples {
		wantObject, relationship := expected[triple.Predicate]
		if relationship {
			observed[triple.Predicate]++
			if triple.Subject != unitID {
				t.Errorf("%s subject = %q, want %q", triple.Predicate, triple.Subject, unitID)
			}
			if triple.Datatype != "@id" {
				t.Errorf("%s datatype = %q, want @id", triple.Predicate, triple.Datatype)
			}
			object, ok := triple.Object.(string)
			if !ok {
				t.Errorf("%s object type = %T, want string", triple.Predicate, triple.Object)
				continue
			}
			if object != wantObject {
				t.Errorf("%s object = %q, want %q", triple.Predicate, object, wantObject)
			}
			if !domain.IsValidEntityID(object) {
				t.Errorf("%s object %q is not a full entity ID", triple.Predicate, object)
			}
			continue
		}

		if triple.Datatype == "@id" {
			t.Errorf("unclassified @id triple %s", triple.Predicate)
		}
		if object, ok := triple.Object.(string); ok && domain.IsValidEntityID(object) {
			t.Errorf(
				"entity-looking scalar %s=%q must be classified as relationship or explicit non-edge",
				triple.Predicate,
				object,
			)
		}
	}

	for predicate := range expected {
		if observed[predicate] != 1 {
			t.Errorf("%s observed %d times, want exactly once", predicate, observed[predicate])
		}
	}
	if len(observed) != 3 {
		t.Fatalf("observed relationship predicates = %d, want 3", len(observed))
	}
}
