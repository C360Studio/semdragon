package partycoord

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/internal/semcompat"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org != "default" || cfg.Platform != "local" || cfg.Board != "main" {
		t.Fatalf("unexpected default board identity: %#v", cfg)
	}
	if cfg.DefaultMaxPartySize != 5 {
		t.Errorf("DefaultMaxPartySize = %d, want 5", cfg.DefaultMaxPartySize)
	}
	if cfg.FormationTimeout != 10*time.Minute {
		t.Errorf("FormationTimeout = %v, want 10m", cfg.FormationTimeout)
	}
	if cfg.RollupTimeout != 5*time.Minute {
		t.Errorf("RollupTimeout = %v, want 5m", cfg.RollupTimeout)
	}
	if !cfg.AutoFormParties || cfg.MinMembersForParty != 2 || !cfg.RequireLeadApproval {
		t.Errorf("unexpected party defaults: %#v", cfg)
	}
}

func TestConfigToBoardConfigReturnsIndependentValues(t *testing.T) {
	cfg := Config{Org: "org", Platform: "platform", Board: "board"}
	first := cfg.ToBoardConfig()
	second := cfg.ToBoardConfig()

	if first == second {
		t.Fatal("ToBoardConfig returned the same pointer twice")
	}
	if first.Org != cfg.Org || first.Platform != cfg.Platform || first.Board != cfg.Board {
		t.Fatalf("ToBoardConfig() = %#v, want identity from %#v", first, cfg)
	}
	first.Org = "changed"
	if second.Org == first.Org {
		t.Fatal("mutating one BoardConfig affected another")
	}
}

func TestPartyTriplesProjectAuthoritativeRelationships(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	disbandedAt := now.Add(time.Hour)
	partyID := domain.PartyID("c360.prod.game.board.party.p001")
	questID := domain.QuestID("c360.prod.game.board.quest.q001")
	subQuestID := domain.QuestID("c360.prod.game.board.quest.sq001")
	leadID := domain.AgentID("c360.prod.game.board.agent.lead")
	memberID := domain.AgentID("c360.prod.game.board.agent.member")

	party := &Party{
		ID:      partyID,
		Name:    "Iron Wolves",
		Status:  domain.PartyDisbanded,
		QuestID: questID,
		Lead:    leadID,
		Members: []PartyMember{
			{AgentID: leadID, Role: domain.RoleLead, JoinedAt: now},
			{AgentID: memberID, Role: domain.RoleExecutor, JoinedAt: now},
		},
		SubQuestMap: map[domain.QuestID]domain.AgentID{subQuestID: memberID},
		FormedAt:    now,
		DisbandedAt: &disbandedAt,
	}

	triples := party.Triples()
	assertPartyTriple(t, triples, partyID, "party.identity.name", party.Name, "")
	assertPartyTriple(t, triples, partyID, "party.status.state", string(domain.PartyDisbanded), "")
	assertPartyTriple(t, triples, partyID, "party.quest", string(questID), semcompat.EntityReferenceDatatype)
	assertPartyTriple(t, triples, partyID, "party.lead", string(leadID), semcompat.EntityReferenceDatatype)
	assertPartyTriple(t, triples, partyID, "party.membership.count", 2, "")
	assertPartyTriple(t, triples, partyID, "party.lifecycle.formed_at", now.Format(time.RFC3339), "")
	assertPartyTriple(t, triples, partyID, "party.lifecycle.disbanded_at", disbandedAt.Format(time.RFC3339), "")
	assertPartyTriple(t, triples, partyID, "party.assignment."+string(subQuestID), string(memberID), semcompat.EntityReferenceDatatype)

	memberTriples := triplesForPredicate(triples, "party.membership.member")
	if len(memberTriples) != 2 {
		t.Fatalf("party.membership.member triples = %d, want 2", len(memberTriples))
	}
	for _, triple := range memberTriples {
		if triple.Datatype != semcompat.EntityReferenceDatatype {
			t.Errorf("member triple datatype = %q, want %q", triple.Datatype, semcompat.EntityReferenceDatatype)
		}
	}
}

func TestPartyTriplesOmitDisbandedTimeWhenActive(t *testing.T) {
	party := &Party{
		ID:          "c360.prod.game.board.party.active",
		SubQuestMap: map[domain.QuestID]domain.AgentID{},
		FormedAt:    time.Now(),
	}

	if got := triplesForPredicate(party.Triples(), "party.lifecycle.disbanded_at"); len(got) != 0 {
		t.Fatalf("active party emitted %d disbanded_at triples, want 0", len(got))
	}
}

func TestPartyFromEntityStateReconstructsAuthoritativeParty(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	disbandedAt := now.Add(time.Hour)
	partyID := domain.PartyID("c360.prod.game.board.party.p001")
	subQuestID := domain.QuestID("c360.prod.game.board.quest.sub001")
	leadID := domain.AgentID("c360.prod.game.board.agent.lead")
	memberID := domain.AgentID("c360.prod.game.board.agent.member")

	entity := &graph.EntityState{
		ID: string(partyID),
		Triples: []message.Triple{
			{Predicate: "party.identity.name", Object: "Iron Wolves"},
			{Predicate: "party.status.state", Object: string(domain.PartyDisbanded)},
			{Predicate: "party.quest", Object: "c360.prod.game.board.quest.q001"},
			{Predicate: "party.lead", Object: string(leadID)},
			{Predicate: "party.membership.member", Object: string(leadID)},
			{Predicate: "party.member." + string(leadID) + ".role", Object: string(domain.RoleLead)},
			{Predicate: "party.membership.member", Object: string(memberID)},
			{Predicate: "party.member." + string(memberID) + ".role", Object: string(domain.RoleExecutor)},
			{Predicate: "party.assignment." + string(subQuestID), Object: string(memberID)},
			{Predicate: "party.lifecycle.formed_at", Object: now.Format(time.RFC3339)},
			{Predicate: "party.lifecycle.disbanded_at", Object: disbandedAt.Format(time.RFC3339)},
		},
	}

	party := PartyFromEntityState(entity)
	if party == nil {
		t.Fatal("PartyFromEntityState returned nil")
	}
	if party.ID != partyID || party.Name != "Iron Wolves" || party.Lead != leadID {
		t.Fatalf("unexpected reconstructed party: %#v", party)
	}
	if party.Status != domain.PartyDisbanded || !party.FormedAt.Equal(now) {
		t.Fatalf("unexpected lifecycle reconstruction: %#v", party)
	}
	if party.DisbandedAt == nil || !party.DisbandedAt.Equal(disbandedAt) {
		t.Fatalf("DisbandedAt = %v, want %v", party.DisbandedAt, disbandedAt)
	}
	if len(party.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(party.Members))
	}
	if party.SubQuestMap[subQuestID] != memberID {
		t.Fatalf("SubQuestMap[%s] = %q, want %q", subQuestID, party.SubQuestMap[subQuestID], memberID)
	}
}

func TestPartyFromEntityStateNil(t *testing.T) {
	if party := PartyFromEntityState(nil); party != nil {
		t.Fatalf("PartyFromEntityState(nil) = %#v, want nil", party)
	}
}

func TestGetPartyReturnsDeepClone(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board.party.clone")
	questID := domain.QuestID("c360.prod.game.board.quest.sub")
	agentID := domain.AgentID("c360.prod.game.board.agent.member")
	original := &Party{
		ID: partyID,
		Members: []PartyMember{{
			AgentID: agentID,
			Skills:  []domain.SkillTag{domain.SkillAnalysis},
		}},
		SubQuestMap: map[domain.QuestID]domain.AgentID{questID: agentID},
	}
	comp := &Component{}
	comp.activeParties.Store(partyID, original)

	got, ok := comp.GetParty(partyID)
	if !ok {
		t.Fatal("GetParty did not find seeded party")
	}
	got.Members[0].AgentID = "mutated"
	got.Members[0].Skills[0] = domain.SkillCodeGen
	got.Members = append(got.Members, PartyMember{AgentID: "extra"})
	got.SubQuestMap[questID] = "mutated"

	again, ok := comp.GetParty(partyID)
	if !ok {
		t.Fatal("GetParty lost seeded party")
	}
	if len(again.Members) != 1 || again.Members[0].AgentID != agentID {
		t.Fatalf("stored members mutated through returned pointer: %#v", again.Members)
	}
	if again.Members[0].Skills[0] != domain.SkillAnalysis {
		t.Fatalf("stored nested skills mutated through returned pointer: %#v", again.Members[0].Skills)
	}
	if again.SubQuestMap[questID] != agentID {
		t.Fatalf("stored assignment mutated through returned map: %#v", again.SubQuestMap)
	}
}

func TestListActivePartiesReturnsDeepClones(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board.party.list-clone")
	questID := domain.QuestID("c360.prod.game.board.quest.sub")
	agentID := domain.AgentID("c360.prod.game.board.agent.member")
	comp := &Component{}
	comp.activeParties.Store(partyID, &Party{
		ID:          partyID,
		Members:     []PartyMember{{AgentID: agentID}},
		SubQuestMap: map[domain.QuestID]domain.AgentID{questID: agentID},
	})

	parties := comp.ListActiveParties()
	if len(parties) != 1 {
		t.Fatalf("ListActiveParties() = %d parties, want 1", len(parties))
	}
	parties[0].Members[0].AgentID = "mutated"
	parties[0].SubQuestMap[questID] = "mutated"

	again := comp.ListActiveParties()
	if len(again) != 1 || again[0].Members[0].AgentID != agentID ||
		again[0].SubQuestMap[questID] != agentID {
		t.Fatalf("stored party mutated through list result: %#v", again)
	}
}

func TestMaybeFormPartyGuardConditions(t *testing.T) {
	claimedBy := domain.AgentID("c360.prod.game.board.agent.lead")
	assignedParty := domain.PartyID("c360.prod.game.board.party.assigned")

	tests := []struct {
		name string
		prev *domain.Quest
		curr *domain.Quest
	}{
		{
			name: "party not required",
			curr: &domain.Quest{ID: "q1", Status: domain.QuestClaimed, PartyRequired: false, ClaimedBy: &claimedBy},
		},
		{
			name: "status unchanged",
			prev: &domain.Quest{ID: "q2", Status: domain.QuestClaimed, PartyRequired: true},
			curr: &domain.Quest{ID: "q2", Status: domain.QuestClaimed, PartyRequired: true, ClaimedBy: &claimedBy},
		},
		{
			name: "not claimed",
			prev: &domain.Quest{ID: "q3", Status: domain.QuestPosted, PartyRequired: true},
			curr: &domain.Quest{ID: "q3", Status: domain.QuestInProgress, PartyRequired: true, ClaimedBy: &claimedBy},
		},
		{
			name: "claim has no agent",
			prev: &domain.Quest{ID: "q4", Status: domain.QuestPosted, PartyRequired: true},
			curr: &domain.Quest{ID: "q4", Status: domain.QuestClaimed, PartyRequired: true},
		},
		{
			name: "party already assigned",
			prev: &domain.Quest{ID: "q5", Status: domain.QuestPosted, PartyRequired: true},
			curr: &domain.Quest{
				ID: "q5", Status: domain.QuestClaimed, PartyRequired: true,
				ClaimedBy: &claimedBy, PartyID: &assignedParty,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &Component{}
			comp.running.Store(true)
			comp.maybeFormParty(tt.prev, tt.curr)
			if parties := comp.ListActiveParties(); len(parties) != 0 {
				t.Fatalf("guard path formed %d parties, want 0", len(parties))
			}
		})
	}
}

func assertPartyTriple(
	t *testing.T,
	triples []message.Triple,
	subject domain.PartyID,
	predicate string,
	object any,
	datatype string,
) {
	t.Helper()
	matches := triplesForPredicate(triples, predicate)
	if len(matches) != 1 {
		t.Fatalf("%s triples = %d, want 1", predicate, len(matches))
	}
	triple := matches[0]
	if triple.Subject != string(subject) || triple.Object != object || triple.Datatype != datatype {
		t.Errorf("%s triple = %#v, want subject=%q object=%v datatype=%q", predicate, triple, subject, object, datatype)
	}
	if triple.Source != "partycoord" || triple.Confidence != 1 {
		t.Errorf("%s provenance = source %q confidence %v", predicate, triple.Source, triple.Confidence)
	}
}

func triplesForPredicate(triples []message.Triple, predicate string) []message.Triple {
	var matches []message.Triple
	for _, triple := range triples {
		if triple.Predicate == predicate {
			matches = append(matches, triple)
		}
	}
	return matches
}
