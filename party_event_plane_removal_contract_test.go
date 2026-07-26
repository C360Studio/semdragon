package semdragons_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var removedPartyEventDTOs = []string{
	"PartyFormedPayload",
	"PartyJoinedPayload",
	"PartyDisbandedPayload",
	"PartyQuestDecomposedPayload",
	"PartyTaskAssignedPayload",
	"PartyResultSubmittedPayload",
	"PartyRollupStartedPayload",
	"PartyRollupCompletedPayload",
}

var removedPartyEventSubjects = []string{
	"SubjectPartyFormed",
	"SubjectPartyJoined",
	"SubjectPartyDisbanded",
	"SubjectPartyQuestDecomposed",
	"SubjectPartyTaskAssigned",
	"SubjectPartyResultSubmitted",
	"SubjectPartyRollupStarted",
	"SubjectPartyRollupCompleted",
}

var removedPartyEventPredicates = []string{
	"PredicatePartyFormed",
	"PredicatePartyDisbanded",
	"PredicatePartyJoined",
	"PredicatePartyLeft",
	"PredicatePartyQuestDecomposed",
	"PredicatePartyTaskAssigned",
	"PredicatePartyContextShared",
	"PredicatePartyGuidanceIssued",
	"PredicatePartyProgressReported",
	"PredicatePartyHelpRequested",
	"PredicatePartyResultSubmitted",
	"PredicatePartyRollupStarted",
	"PredicatePartyRollupCompleted",
}

var removedPartyEventPredicateValues = []string{
	"party.formation.formed",
	"party.formation.disbanded",
	"party.membership.joined",
	"party.membership.left",
	"party.coordination.decomposed",
	"party.coordination.assigned",
	"party.coordination.contextshared",
	"party.coordination.guidance",
	"party.coordination.progress",
	"party.coordination.helprequest",
	"party.coordination.resultsubmitted",
	"party.coordination.rollupstarted",
	"party.coordination.rollupcompleted",
}

var removedPartyDeadMethods = []string{
	"DecomposeQuest",
	"SubmitResult",
	"StartRollup",
	"CompleteRollup",
}

var removedPartyShadowMarkers = []string{
	"SharedContext",
	"ContextItem",
	"SubResults",
	"RollupResult",
	"party.strategy",
	"party.context.count",
	"party.results.count",
	"rollupsCompleted",
}

func TestPartyEventPlaneAndShadowStateAreFullyRemoved(t *testing.T) {
	if len(removedPartyEventDTOs) != 8 {
		t.Fatalf("removed DTO inventory has %d entries, want 8", len(removedPartyEventDTOs))
	}
	if len(removedPartyEventSubjects) != 8 {
		t.Fatalf("removed subject inventory has %d entries, want 8", len(removedPartyEventSubjects))
	}
	if len(removedPartyEventPredicates) != 13 || len(removedPartyEventPredicateValues) != 13 {
		t.Fatalf("removed vocabulary inventory = %d symbols/%d values, want 13/13",
			len(removedPartyEventPredicates), len(removedPartyEventPredicateValues))
	}
	if len(removedPartyDeadMethods) != 4 {
		t.Fatalf("removed method inventory has %d entries, want 4", len(removedPartyDeadMethods))
	}

	production := readPartyRemovalProductionGo(t)
	for _, violation := range partyRemovalViolations(production) {
		t.Error(violation)
	}

	if _, err := os.Stat("processor/partycoord/payloads.go"); !os.IsNotExist(err) {
		t.Errorf("processor/partycoord/payloads.go must be deleted; stat error = %v", err)
	}

	assertPartyFileExcludes(t, "processor/partycoord/component.go",
		`Name:        "party-formed"`,
		`Name:        "party-disbanded"`,
	)
	assertPartyFileExcludes(t, "processor/dmpartyformation/component.go",
		`Name:        "party-events"`,
	)
	assertPartyFileExcludes(t, "processor/partycoord/handler_test.go", removedPartyEventDTOs...)
	assertPartyFileExcludes(t, "processor/partycoord/component_test.go",
		"DecomposeQuest(", "SubmitResult(", "StartRollup(", "CompleteRollup(",
		"SharedContext", "SubResults", "RollupResult", ".Strategy",
	)
	assertPartyFileExcludes(t, "graph_relationship_contract_test.go", removedPartyEventDTOs...)
	assertPartyFileExcludes(t, "processor/dmsession/session_test.go",
		"EventPartyFormed", "EventPartyDisbanded",
	)
	assertPartyFileExcludes(t, "processor/questbridge/context.go",
		"party.Strategy", `"Strategy: %s\n"`,
	)

	assertPartyFileContains(t, "processor/partycoord/party.go",
		"type Party struct",
		"SubQuestMap",
		`Predicate: "party.assignment.quest"`,
		`Predicate: "party.membership.lead"`,
		`Predicate: "party.membership.member"`,
		`Predicate: "party.assignment." + string(questID)`,
		`Predicate: "party.lifecycle.formed-at"`,
		`Predicate: "party.lifecycle.disbanded-at"`,
	)
	assertPartyFileContains(t, "processor/partycoord/handler.go",
		"func (c *Component) FormParty(",
		"func (c *Component) JoinParty(",
		"func (c *Component) DisbandParty(",
		"func (c *Component) GetParty(",
		"func (c *Component) ListActiveParties(",
		"func (c *Component) AssignTask(",
		`c.graph.EmitEntity(ctx, party, "party.formed")`,
		`c.graph.EmitEntity(ctx, party, "party.joined")`,
		`c.graph.EmitEntity(ctx, party, "party.disbanded")`,
	)
	assertPartyFileContains(t, "processor/partycoord/component.go",
		`Name:        "party-state"`,
		"component.KVWritePort",
	)
	assertPartyFileContains(t, "graph_relationship_contract_test.go",
		`name: "persisted party"`,
		`"party.assignment.quest":`,
		`"party.membership.lead":`,
		`"party.membership.member":`,
		`"party.assignment." + string(quest2):`,
	)
}

func partyRemovalViolations(files map[string][]byte) []string {
	var violations []string
	for path, data := range files {
		source := string(data)
		for _, marker := range removedPartyEventDTOs {
			if strings.Contains(source, marker) {
				violations = append(violations, path+" retains removed party DTO "+marker)
			}
		}
		for _, marker := range removedPartyEventSubjects {
			if strings.Contains(source, marker) {
				violations = append(violations, path+" retains removed typed party subject "+marker)
			}
		}
		for _, marker := range removedPartyEventPredicates {
			if strings.Contains(source, marker) {
				violations = append(violations, path+" retains removed party vocabulary symbol "+marker)
			}
		}
		for _, literal := range removedPartyEventPredicateValues {
			if strings.Contains(source, `"`+literal+`"`) ||
				strings.Contains(source, "`"+literal+"`") {
				violations = append(violations, path+" retains removed party vocabulary literal "+literal)
			}
		}
	}

	if data := files["processor/partycoord/handler.go"]; data != nil {
		source := string(data)
		for _, method := range removedPartyDeadMethods {
			if strings.Contains(source, "func (c *Component) "+method+"(") {
				violations = append(violations, "processor/partycoord/handler.go retains dead method "+method)
			}
		}
	}
	for _, path := range []string{
		"processor/partycoord/party.go",
		"processor/partycoord/handler.go",
		"processor/partycoord/component.go",
	} {
		source := string(files[path])
		for _, marker := range removedPartyShadowMarkers {
			if strings.Contains(source, marker) {
				violations = append(violations, path+" retains shadow Party marker "+marker)
			}
		}
	}
	if source := string(files["processor/partycoord/party.go"]); strings.Contains(source, "Strategy ") {
		violations = append(violations, "processor/partycoord/party.go retains shadow Party Strategy field")
	}
	if source := string(files["processor/questbridge/context.go"]); strings.Contains(source, "party.Strategy") {
		violations = append(violations, "processor/questbridge/context.go retains always-empty Party strategy rendering")
	}
	if source := string(files["domain/dm.go"]); strings.Contains(source, "EventPartyFormed") ||
		strings.Contains(source, "EventPartyDisbanded") {
		violations = append(violations, "domain/dm.go retains phantom party GameEventType values")
	}
	return violations
}

func TestPartyRemovalContractRejectsSyntheticMutations(t *testing.T) {
	baseline := func() map[string][]byte {
		return map[string][]byte{
			"domain/dm.go":                            nil,
			"processor/partycoord/party.go":           nil,
			"processor/partycoord/handler.go":         nil,
			"processor/partycoord/component.go":       nil,
			"processor/questbridge/context.go":        nil,
			"processor/dmpartyformation/component.go": nil,
		}
	}
	assertRejected := func(t *testing.T, path, source string) {
		t.Helper()
		files := baseline()
		files[path] = []byte(source)
		if violations := partyRemovalViolations(files); len(violations) == 0 {
			t.Fatalf("synthetic stale surface %q in %s escaped the removal contract", source, path)
		}
	}

	for _, marker := range removedPartyEventDTOs {
		t.Run("dto/"+marker, func(t *testing.T) {
			assertRejected(t, "processor/partycoord/handler.go", "var _ "+marker)
		})
	}
	for _, marker := range removedPartyEventSubjects {
		t.Run("subject/"+marker, func(t *testing.T) {
			assertRejected(t, "processor/partycoord/handler.go", "var _ = "+marker)
		})
	}
	for _, marker := range removedPartyEventPredicates {
		t.Run("predicate/"+marker, func(t *testing.T) {
			assertRejected(t, "domain/vocab.go", "const _ = "+marker)
		})
	}
	for _, literal := range removedPartyEventPredicateValues {
		t.Run("literal/"+literal, func(t *testing.T) {
			assertRejected(t, "domain/vocab.go", `const _ = "`+literal+`"`)
		})
	}
	for _, method := range removedPartyDeadMethods {
		t.Run("method/"+method, func(t *testing.T) {
			assertRejected(t, "processor/partycoord/handler.go", "func (c *Component) "+method+"() {}")
		})
	}
	for _, marker := range removedPartyShadowMarkers {
		t.Run("shadow/"+marker, func(t *testing.T) {
			assertRejected(t, "processor/partycoord/party.go", marker)
		})
	}
	t.Run("shadow/StrategyField", func(t *testing.T) {
		assertRejected(t, "processor/partycoord/party.go", "Strategy string")
	})
	t.Run("rendering/PartyStrategy", func(t *testing.T) {
		assertRejected(t, "processor/questbridge/context.go", "party.Strategy")
	})
	for _, eventType := range []string{"EventPartyFormed", "EventPartyDisbanded"} {
		t.Run("game-event/"+eventType, func(t *testing.T) {
			assertRejected(t, "domain/dm.go", eventType)
		})
	}
}

func readPartyRemovalProductionGo(t *testing.T) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	roots := []string{
		"domain",
		"processor/partycoord",
		"processor/dmpartyformation",
		"processor/dmsession",
		"processor/questbridge",
	}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			files[filepath.ToSlash(path)] = data
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return files
}

func assertPartyFileContains(t *testing.T, path string, markers ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, marker := range markers {
		if !strings.Contains(string(data), marker) {
			t.Errorf("%s missing retained authoritative marker %q", path, marker)
		}
	}
}

func assertPartyFileExcludes(t *testing.T, path string, markers ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, marker := range markers {
		if strings.Contains(string(data), marker) {
			t.Errorf("%s retains deleted party surface %q", path, marker)
		}
	}
}
