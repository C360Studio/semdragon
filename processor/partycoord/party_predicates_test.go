package partycoord

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
)

var partyScalarPredicateRenames = map[string]string{
	"party.quest":                  "party.assignment.quest",
	"party.lead":                   "party.membership.lead",
	"party.lifecycle.formed_at":    "party.lifecycle.formed-at",
	"party.lifecycle.disbanded_at": "party.lifecycle.disbanded-at",
}

func TestPartyPredicateMappingAndDynamicAssignmentExclusionAreExact(t *testing.T) {
	t.Parallel()

	if len(partyScalarPredicateRenames) != 4 {
		t.Fatalf("predicate rename contract has %d entries, want 4", len(partyScalarPredicateRenames))
	}

	source, err := os.ReadFile("party.go")
	if err != nil {
		t.Fatalf("read party.go: %v", err)
	}
	text := string(source)
	canonicalOccurrences := map[string]int{
		"party.assignment.quest":       3, // producer, reconstruction, dynamic-parser exclusion
		"party.membership.lead":        2, // producer and reconstruction
		"party.lifecycle.formed-at":    2, // producer and reconstruction
		"party.lifecycle.disbanded-at": 2, // producer and reconstruction
	}
	for stale, canonical := range partyScalarPredicateRenames {
		if strings.Contains(text, `"`+stale+`"`) {
			t.Errorf("party.go retains stale producer or reconstruction predicate %q", stale)
		}
		if got := strings.Count(text, `"`+canonical+`"`); got != canonicalOccurrences[canonical] {
			t.Errorf("%q occurrences = %d, want %d", canonical, got, canonicalOccurrences[canonical])
		}
	}
	if got := strings.Count(text, `triple.Predicate != "party.assignment.quest"`); got != 1 {
		t.Errorf("canonical quest exclusion guards = %d, want exactly 1", got)
	}
}

func TestPartyCanonicalLifecycleTimestampsAndOptionalOmission(t *testing.T) {
	t.Parallel()

	party := canonicalPredicateParty()
	triples := party.Triples()
	for _, tt := range []struct {
		predicate string
		want      string
	}{
		{predicate: "party.lifecycle.formed-at", want: party.FormedAt.Format(time.RFC3339)},
		{predicate: "party.lifecycle.disbanded-at", want: party.DisbandedAt.Format(time.RFC3339)},
	} {
		triple := requireSinglePartyPredicate(t, triples, tt.predicate)
		got, ok := triple.Object.(string)
		if !ok {
			t.Errorf("%s object type = %T, want string", tt.predicate, triple.Object)
		} else if got != tt.want {
			t.Errorf("%s object = %q, want %q", tt.predicate, got, tt.want)
		}
		if triple.Datatype != "" {
			t.Errorf("%s datatype = %q, want untyped scalar", tt.predicate, triple.Datatype)
		}
	}

	party.DisbandedAt = nil
	if got := triplesForPredicate(party.Triples(), "party.lifecycle.disbanded-at"); len(got) != 0 {
		t.Errorf("active party emitted %d canonical disbanded-at triples, want 0", len(got))
	}
}

func TestPartyTriplesRestrictEntityReferenceDatatypeToCanonicalRelationships(t *testing.T) {
	t.Parallel()

	party := canonicalPredicateParty()
	dynamicAssignment := "party.assignment.c360.prod.game.board.quest.sub001"
	expectedReferences := map[string]map[string]int{
		"party.assignment.quest": {
			string(party.QuestID): 1,
		},
		"party.membership.lead": {
			string(party.Lead): 1,
		},
		"party.membership.member": {
			string(party.Members[0].AgentID): 1,
			string(party.Members[1].AgentID): 1,
		},
		dynamicAssignment: {
			string(party.SubQuestMap["c360.prod.game.board.quest.sub001"]): 1,
		},
	}
	observed := make(map[string]map[string]int, len(expectedReferences))
	for _, triple := range party.Triples() {
		expectedTargets, relationship := expectedReferences[triple.Predicate]
		if !relationship {
			if triple.Datatype != "" {
				t.Errorf("non-relationship %q datatype = %q, want empty", triple.Predicate, triple.Datatype)
			}
			continue
		}

		if triple.Datatype != message.EntityReferenceDatatype {
			t.Errorf("%s datatype = %q, want %q", triple.Predicate, triple.Datatype, message.EntityReferenceDatatype)
		}
		target, ok := triple.Object.(string)
		if !ok {
			t.Errorf("%s object type = %T, want canonical entity ID string", triple.Predicate, triple.Object)
			continue
		}
		if expectedTargets[target] == 0 {
			t.Errorf("%s unexpected entity target %q", triple.Predicate, target)
			continue
		}
		if observed[triple.Predicate] == nil {
			observed[triple.Predicate] = make(map[string]int)
		}
		observed[triple.Predicate][target]++
	}
	for predicate, targets := range expectedReferences {
		for target, wantCount := range targets {
			if got := observed[predicate][target]; got != wantCount {
				t.Errorf("%s target %q observed %d times, want %d", predicate, target, got, wantCount)
			}
		}
	}
}

func TestPartyCanonicalTriplesRoundTrip(t *testing.T) {
	t.Parallel()

	want := canonicalPredicateParty()
	got := PartyFromEntityState(&graph.EntityState{
		ID:      want.EntityID(),
		Triples: want.Triples(),
	})

	if got.ID != want.ID ||
		got.Name != want.Name ||
		got.Status != want.Status ||
		got.QuestID != want.QuestID ||
		got.Lead != want.Lead ||
		!got.FormedAt.Equal(want.FormedAt) {
		t.Errorf("canonical scalar round trip = %#v, want %#v", got, want)
	}
	if got.DisbandedAt == nil || !got.DisbandedAt.Equal(*want.DisbandedAt) {
		t.Errorf("DisbandedAt = %v, want %v", got.DisbandedAt, want.DisbandedAt)
	}
	if len(got.Members) != len(want.Members) {
		t.Fatalf("members = %#v, want %#v", got.Members, want.Members)
	}
	memberRoles := make(map[domain.AgentID]domain.PartyRole, len(got.Members))
	for _, member := range got.Members {
		memberRoles[member.AgentID] = member.Role
	}
	for _, member := range want.Members {
		if gotRole := memberRoles[member.AgentID]; gotRole != member.Role {
			t.Errorf("member %q role = %q, want %q", member.AgentID, gotRole, member.Role)
		}
	}
	if len(got.SubQuestMap) != 1 {
		t.Fatalf("SubQuestMap = %#v, want one entry", got.SubQuestMap)
	}
	for questID, agentID := range want.SubQuestMap {
		if got.SubQuestMap[questID] != agentID {
			t.Errorf("SubQuestMap[%q] = %q, want %q", questID, got.SubQuestMap[questID], agentID)
		}
	}
}

func TestPartyFromEntityStateRejectsAllFourLegacyScalarReaders(t *testing.T) {
	t.Parallel()

	const entityID = "c360.prod.game.board.party.legacy"
	got := PartyFromEntityState(&graph.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{Subject: entityID, Predicate: "party.quest", Object: "c360.prod.game.board.quest.legacy"},
			{Subject: entityID, Predicate: "party.lead", Object: "c360.prod.game.board.agent.legacy"},
			{Subject: entityID, Predicate: "party.lifecycle.formed_at", Object: "2026-07-25T12:00:00Z"},
			{Subject: entityID, Predicate: "party.lifecycle.disbanded_at", Object: "2026-07-25T13:00:00Z"},
		},
	})

	if got.QuestID != "" ||
		got.Lead != "" ||
		!got.FormedAt.IsZero() ||
		got.DisbandedAt != nil {
		t.Errorf("legacy scalar predicates changed canonical-only reconstruction: %#v", got)
	}
}

func TestPartyCanonicalQuestEdgeDoesNotCollideWithDynamicAssignmentParser(t *testing.T) {
	t.Parallel()

	const (
		entityID      = "c360.prod.game.board.party.collision"
		questID       = "c360.prod.game.board.quest.primary"
		subQuestID    = "c360.prod.game.board.quest.sub001"
		assignedAgent = "c360.prod.game.board.agent.member"
	)
	got := PartyFromEntityState(&graph.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{Subject: entityID, Predicate: "party.assignment.quest", Object: questID, Datatype: message.EntityReferenceDatatype},
			{Subject: entityID, Predicate: "party.assignment." + subQuestID, Object: assignedAgent, Datatype: message.EntityReferenceDatatype},
		},
	})

	if got.QuestID != questID {
		t.Errorf("QuestID = %q, want %q", got.QuestID, questID)
	}
	if len(got.SubQuestMap) != 1 {
		t.Fatalf("SubQuestMap = %#v, want exactly one dynamic assignment", got.SubQuestMap)
	}
	if got.SubQuestMap[subQuestID] != assignedAgent {
		t.Errorf("SubQuestMap[%q] = %q, want %q", subQuestID, got.SubQuestMap[subQuestID], assignedAgent)
	}
	if _, collided := got.SubQuestMap["quest"]; collided {
		t.Errorf("canonical party.assignment.quest leaked into SubQuestMap: %#v", got.SubQuestMap)
	}
}

func TestPartyDynamicFullIDPredicateFamiliesRemainBlockedByIssues4And313(t *testing.T) {
	t.Parallel()

	// Semdragon #4 and SemStreams #313 block replacement of runtime full-ID predicate families.
	// Keep both producer and reconstruction behavior unchanged until framework
	// primitives can represent these role and sub-quest assignment collections.
	party := canonicalPredicateParty()
	memberRolePredicate := "party.member." + string(party.Members[1].AgentID) + ".role"
	subQuestID := domain.QuestID("c360.prod.game.board.quest.sub001")
	assignmentPredicate := "party.assignment." + string(subQuestID)
	triples := party.Triples()

	roleTriple := requireSinglePartyPredicate(t, triples, memberRolePredicate)
	if roleTriple.Object != string(party.Members[1].Role) || roleTriple.Datatype != "" {
		t.Errorf("dynamic member role triple = %#v", roleTriple)
	}
	assignmentTriple := requireSinglePartyPredicate(t, triples, assignmentPredicate)
	if assignmentTriple.Object != string(party.SubQuestMap[subQuestID]) ||
		assignmentTriple.Datatype != message.EntityReferenceDatatype {
		t.Errorf("dynamic assignment triple = %#v", assignmentTriple)
	}

	reconstructed := PartyFromEntityState(&graph.EntityState{
		ID:      party.EntityID(),
		Triples: []message.Triple{roleTriple, assignmentTriple},
	})
	if len(reconstructed.Members) != 1 ||
		reconstructed.Members[0].AgentID != party.Members[1].AgentID ||
		reconstructed.Members[0].Role != party.Members[1].Role {
		t.Errorf("dynamic role reconstruction = %#v", reconstructed.Members)
	}
	if len(reconstructed.SubQuestMap) != 1 ||
		reconstructed.SubQuestMap[subQuestID] != party.SubQuestMap[subQuestID] {
		t.Errorf("dynamic assignment reconstruction = %#v", reconstructed.SubQuestMap)
	}
}

func canonicalPredicateParty() *Party {
	formedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	disbandedAt := formedAt.Add(time.Hour)
	leadID := domain.AgentID("c360.prod.game.board.agent.lead")
	memberID := domain.AgentID("c360.prod.game.board.agent.member")
	subQuestID := domain.QuestID("c360.prod.game.board.quest.sub001")
	return &Party{
		ID:      "c360.prod.game.board.party.canonical",
		Name:    "Canonical Party",
		Status:  domain.PartyDisbanded,
		QuestID: "c360.prod.game.board.quest.primary",
		Lead:    leadID,
		Members: []PartyMember{
			{AgentID: leadID, Role: domain.RoleLead},
			{AgentID: memberID, Role: domain.RoleExecutor},
		},
		SubQuestMap: map[domain.QuestID]domain.AgentID{subQuestID: memberID},
		FormedAt:    formedAt,
		DisbandedAt: &disbandedAt,
	}
}

func requireSinglePartyPredicate(t *testing.T, triples []message.Triple, predicate string) message.Triple {
	t.Helper()
	matches := triplesForPredicate(triples, predicate)
	if len(matches) != 1 {
		t.Fatalf("%s triples = %d, want exactly one", predicate, len(matches))
	}
	return matches[0]
}
