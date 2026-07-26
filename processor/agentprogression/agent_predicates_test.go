package agentprogression

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/internal/semcompat"
)

var agentScalarPredicateRenames = map[string]string{
	"agent.identity.display_name":   "agent.identity.display-name",
	"agent.progression.xp.current":  "agent.progression.xp-current",
	"agent.progression.xp.to_level": "agent.progression.xp-to-level",
	"agent.progression.death_count": "agent.progression.death-count",
	"agent.stats.quests_completed":  "agent.stats.quests-completed",
	"agent.stats.quests_failed":     "agent.stats.quests-failed",
	"agent.stats.bosses_defeated":   "agent.stats.bosses-defeated",
	"agent.stats.total_xp_earned":   "agent.stats.total-xp-earned",
	"agent.lifecycle.created_at":    "agent.lifecycle.created-at",
	"agent.lifecycle.updated_at":    "agent.lifecycle.updated-at",
	"agent.status.cooldown_until":   "agent.status.cooldown-until",
	"agent.inventory.total_spent":   "agent.inventory.total-spent",
	"agent.reputation.peer_avg":     "agent.reputation.peer-avg",
	"agent.reputation.peer_q1_avg":  "agent.reputation.peer-q1-avg",
	"agent.reputation.peer_q2_avg":  "agent.reputation.peer-q2-avg",
	"agent.reputation.peer_q3_avg":  "agent.reputation.peer-q3-avg",
	"agent.reputation.peer_count":   "agent.reputation.peer-count",
}

func TestAgentTriplesEmitCanonicalScalarAndBoundedSkillPredicates(t *testing.T) {
	t.Parallel()

	agent := canonicalPredicateAgent()
	triples := agent.Triples()
	predicates := make([]string, 0, len(triples))
	for _, triple := range triples {
		predicates = append(predicates, triple.Predicate)
	}

	if len(agentScalarPredicateRenames) != 17 {
		t.Fatalf("scalar rename contract has %d entries, want 17", len(agentScalarPredicateRenames))
	}
	for stale, canonical := range agentScalarPredicateRenames {
		if slices.Contains(predicates, stale) {
			t.Errorf("emitted stale predicate %q", stale)
		}
		if !slices.Contains(predicates, canonical) {
			t.Errorf("missing canonical predicate %q", canonical)
		}
	}

	skills := map[string]map[string]any{}
	for _, triple := range triples {
		if strings.HasPrefix(triple.Predicate, "agent.skill.") &&
			triple.Predicate != "agent.skill.proficiency" {
			t.Errorf("runtime skill data escaped into predicate %q", triple.Predicate)
		}
		if triple.Predicate != "agent.skill.proficiency" {
			continue
		}
		value, ok := triple.Object.(map[string]any)
		if !ok {
			t.Fatalf("in-process skill object type = %T, want map[string]any", triple.Object)
		}
		tag := domain.AsString(value["tag"])
		skills[tag] = value
	}
	if len(skills) != 2 {
		t.Fatalf("bounded skill objects = %#v, want two", skills)
	}
	assertSkillObject(t, skills["coding"], domain.ProficiencyExpert, 4200)
	assertSkillObject(t, skills["testing"], domain.ProficiencyJourneyman, 2300)

	for _, predicate := range []string{
		"agent.membership.guild",
		"agent.assignment.quest",
		"agent.membership.party",
	} {
		triple, ok := findAgentTriple(triples, predicate)
		if !ok {
			t.Fatalf("missing relationship %q", predicate)
		}
		if triple.Datatype != semcompat.EntityReferenceDatatype {
			t.Errorf("%s datatype = %q, want %q", predicate, triple.Datatype, semcompat.EntityReferenceDatatype)
		}
	}
}

func TestAgentCanonicalTriplesRoundTripInProcessAndJSONDecoded(t *testing.T) {
	t.Parallel()

	want := canonicalPredicateAgent()
	triples := want.Triples()
	slices.Reverse(triples)

	tests := []struct {
		name    string
		triples []message.Triple
	}{
		{name: "in-process maps", triples: triples},
		{name: "graph JSON decoded maps", triples: jsonRoundTripTriples(t, triples)},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := AgentFromEntityState(&graph.EntityState{ID: string(want.ID), Triples: tt.triples})
			assertCanonicalAgentProjection(t, got, want)
		})
	}
}

func TestAgentGoContainsNoStaleScalarOrDynamicSkillPredicates(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("agent.go")
	if err != nil {
		t.Fatalf("read agent.go: %v", err)
	}
	text := string(source)
	for stale := range agentScalarPredicateRenames {
		if strings.Contains(text, `"`+stale+`"`) {
			t.Errorf("agent.go retains stale predicate %q", stale)
		}
	}
	for _, fragment := range []string{
		`fmt.Sprintf("agent.skill.`,
		`"agent.skill."`,
	} {
		if strings.Contains(text, fragment) {
			t.Errorf("agent.go retains dynamic skill predicate fragment %q", fragment)
		}
	}
}

func canonicalPredicateAgent() *Agent {
	cooldown := time.Date(2026, time.July, 25, 20, 30, 0, 0, time.UTC)
	questID := domain.QuestID("local.test.game.board1.quest.canonical")
	partyID := domain.PartyID("local.test.game.board1.party.canonical")
	return &Agent{
		ID:            domain.AgentID("local.test.game.board1.agent.canonical"),
		Name:          "canonical",
		DisplayName:   "Canonical Agent",
		Status:        domain.AgentCooldown,
		Level:         7,
		XP:            7100,
		XPToLevel:     8000,
		DeathCount:    2,
		Archetype:     domain.AgentArchetype("developer"),
		Tier:          domain.TrustTier(3),
		Guild:         domain.GuildID("local.test.game.board1.guild.canonical"),
		CurrentQuest:  &questID,
		CurrentParty:  &partyID,
		CooldownUntil: &cooldown,
		TotalSpent:    1900,
		IsNPC:         true,
		CreatedAt:     time.Date(2026, time.July, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, time.July, 25, 20, 0, 0, 0, time.UTC),
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"coding":  {Level: domain.ProficiencyExpert, TotalXP: 4200},
			"testing": {Level: domain.ProficiencyJourneyman, TotalXP: 2300},
		},
		Stats: AgentStats{
			QuestsCompleted: 21,
			QuestsFailed:    3,
			BossesDefeated:  5,
			TotalXPEarned:   12345,
			PeerReviewAvg:   4.5,
			PeerReviewQ1Avg: 4.1,
			PeerReviewQ2Avg: 4.4,
			PeerReviewQ3Avg: 4.8,
			PeerReviewCount: 9,
		},
	}
}

func jsonRoundTripTriples(t *testing.T, triples []message.Triple) []message.Triple {
	t.Helper()
	data, err := json.Marshal(triples)
	if err != nil {
		t.Fatalf("marshal triples: %v", err)
	}
	var decoded []message.Triple
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal triples: %v", err)
	}
	return decoded
}

func assertSkillObject(
	t *testing.T,
	object map[string]any,
	wantLevel domain.ProficiencyLevel,
	wantXP int64,
) {
	t.Helper()
	if got := domain.ProficiencyLevel(domain.AsInt(object["level"])); got != wantLevel {
		t.Errorf("skill level = %d, want %d", got, wantLevel)
	}
	if got := domain.AsInt64(object["total-xp"]); got != wantXP {
		t.Errorf("skill total-xp = %d, want %d", got, wantXP)
	}
}

func findAgentTriple(triples []message.Triple, predicate string) (message.Triple, bool) {
	for _, triple := range triples {
		if triple.Predicate == predicate {
			return triple, true
		}
	}
	return message.Triple{}, false
}

func assertCanonicalAgentProjection(t *testing.T, got, want *Agent) {
	t.Helper()
	if got == nil {
		t.Fatal("reconstructed agent is nil")
	}
	if got.ID != want.ID ||
		got.Name != want.Name ||
		got.DisplayName != want.DisplayName ||
		got.Status != want.Status ||
		got.Level != want.Level ||
		got.XP != want.XP ||
		got.XPToLevel != want.XPToLevel ||
		got.DeathCount != want.DeathCount ||
		got.Archetype != want.Archetype ||
		got.Tier != want.Tier ||
		got.Guild != want.Guild ||
		got.TotalSpent != want.TotalSpent ||
		got.IsNPC != want.IsNPC {
		t.Errorf("scalar projection mismatch:\n got=%#v\nwant=%#v", got, want)
	}
	if !reflect.DeepEqual(got.CurrentQuest, want.CurrentQuest) ||
		!reflect.DeepEqual(got.CurrentParty, want.CurrentParty) ||
		!reflect.DeepEqual(got.CooldownUntil, want.CooldownUntil) {
		t.Errorf("relationship/state projection mismatch:\n got=%#v\nwant=%#v", got, want)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("lifecycle timestamps got=(%s,%s) want=(%s,%s)",
			got.CreatedAt, got.UpdatedAt, want.CreatedAt, want.UpdatedAt)
	}
	if !reflect.DeepEqual(got.Stats, want.Stats) {
		t.Errorf("stats mismatch:\n got=%#v\nwant=%#v", got.Stats, want.Stats)
	}
	if !reflect.DeepEqual(got.SkillProficiencies, want.SkillProficiencies) {
		t.Errorf("skills mismatch:\n got=%#v\nwant=%#v", got.SkillProficiencies, want.SkillProficiencies)
	}
}
