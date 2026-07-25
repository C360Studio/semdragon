package questdag

import "testing"

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
		"claim":        PredClaim,
		"completed":    PredCompleted,
		"failed":       PredFailed,
		"dirtied":      PredDirtied,
		"depends-on":   PredDependsOn,
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
