package guildformation

import (
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// DefaultConfig
// =============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org != "default" {
		t.Errorf("Org = %q; want %q", cfg.Org, "default")
	}
	if cfg.Platform != "local" {
		t.Errorf("Platform = %q; want %q", cfg.Platform, "local")
	}
	if cfg.Board != "main" {
		t.Errorf("Board = %q; want %q", cfg.Board, "main")
	}
	if cfg.MinMembersForFormation != 3 {
		t.Errorf("MinMembersForFormation = %d; want 3", cfg.MinMembersForFormation)
	}
	if cfg.MaxGuildSize != 20 {
		t.Errorf("MaxGuildSize = %d; want 20", cfg.MaxGuildSize)
	}
	if cfg.EnableAutoFormation {
		t.Errorf("EnableAutoFormation = true; want false (agents should opt-in to guilds)")
	}
}

func TestDefaultConfig_ToBoardConfig(t *testing.T) {
	cfg := DefaultConfig()
	bc := cfg.ToBoardConfig()

	if bc == nil {
		t.Fatal("ToBoardConfig() returned nil")
	}
	if bc.Org != cfg.Org {
		t.Errorf("BoardConfig.Org = %q; want %q", bc.Org, cfg.Org)
	}
	if bc.Platform != cfg.Platform {
		t.Errorf("BoardConfig.Platform = %q; want %q", bc.Platform, cfg.Platform)
	}
	if bc.Board != cfg.Board {
		t.Errorf("BoardConfig.Board = %q; want %q", bc.Board, cfg.Board)
	}
}

// =============================================================================
// generateGuildName
// =============================================================================

func TestGenerateGuildName(t *testing.T) {
	tests := []struct {
		name        string
		founder     *agentprogression.Agent
		wantContain string
	}{
		{
			name: "uses DisplayName when set",
			founder: &agentprogression.Agent{
				ID:          "agent-1",
				Name:        "alpha",
				DisplayName: "Shadowweaver",
			},
			wantContain: "Shadowweaver's Guild",
		},
		{
			name: "falls back to Name when DisplayName is empty",
			founder: &agentprogression.Agent{
				ID:          "agent-2",
				Name:        "coder-bot",
				DisplayName: "",
			},
			wantContain: "coder-bot's Guild",
		},
		{
			name: "DisplayName takes priority over Name when both set",
			founder: &agentprogression.Agent{
				ID:          "agent-3",
				Name:        "system-name",
				DisplayName: "TrueTitle",
			},
			wantContain: "TrueTitle's Guild",
		},
		{
			name: "apostrophe-s suffix appended",
			founder: &agentprogression.Agent{
				ID:          "agent-4",
				Name:        "Ironclad",
				DisplayName: "",
			},
			wantContain: "Ironclad's Guild",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateGuildName(tt.founder)
			if got != tt.wantContain {
				t.Errorf("generateGuildName() = %q; want %q", got, tt.wantContain)
			}
		})
	}
}

// =============================================================================
// selectDiverseCandidates
// =============================================================================

func agentWithSkills(id domain.AgentID, skills ...domain.SkillTag) *agentprogression.Agent {
	profs := make(map[domain.SkillTag]domain.SkillProficiency, len(skills))
	for _, s := range skills {
		profs[s] = domain.SkillProficiency{Level: domain.ProficiencyNovice}
	}
	return &agentprogression.Agent{
		ID:                 id,
		Name:               string(id),
		SkillProficiencies: profs,
	}
}

func containsAgent(selected []*agentprogression.Agent, id domain.AgentID) bool {
	for _, a := range selected {
		if a.ID == id {
			return true
		}
	}
	return false
}

func TestSelectDiverseCandidates_EmptyInput(t *testing.T) {
	result := selectDiverseCandidates(nil, 3, "founder-1")
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d agents", len(result))
	}
}

func TestSelectDiverseCandidates_OnlyFounder(t *testing.T) {
	founder := agentWithSkills("founder-1", domain.SkillCodeGen)
	candidates := []*agentprogression.Agent{founder}

	result := selectDiverseCandidates(candidates, 3, "founder-1")

	// Only the founder is in the list; result should contain just them.
	if len(result) != 1 {
		t.Fatalf("expected 1 result (just founder), got %d", len(result))
	}
	if result[0].ID != "founder-1" {
		t.Errorf("result[0].ID = %q; want %q", result[0].ID, "founder-1")
	}
}

func TestSelectDiverseCandidates_FounderAlwaysFirst(t *testing.T) {
	// Founder appears last in the input slice — must still appear first in output.
	a1 := agentWithSkills("agent-1", domain.SkillAnalysis)
	a2 := agentWithSkills("agent-2", domain.SkillResearch)
	founder := agentWithSkills("founder-1", domain.SkillCodeGen)
	candidates := []*agentprogression.Agent{a1, a2, founder}

	result := selectDiverseCandidates(candidates, 3, "founder-1")

	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
	if result[0].ID != "founder-1" {
		t.Errorf("expected founder first, got %q", result[0].ID)
	}
}

func TestSelectDiverseCandidates_DistinctSkillsIncluded(t *testing.T) {
	founder := agentWithSkills("founder-1", domain.SkillCodeGen)
	// agent-2 has a distinct skill (Analysis) — should be included.
	a2 := agentWithSkills("agent-2", domain.SkillAnalysis)
	// agent-3 also distinct (Research) — should be included.
	a3 := agentWithSkills("agent-3", domain.SkillResearch)
	candidates := []*agentprogression.Agent{founder, a2, a3}

	result := selectDiverseCandidates(candidates, 3, "founder-1")

	if len(result) != 3 {
		t.Fatalf("expected 3 candidates selected, got %d", len(result))
	}
	if !containsAgent(result, "founder-1") {
		t.Error("founder not in result")
	}
	if !containsAgent(result, "agent-2") {
		t.Error("agent-2 (distinct skill) not in result")
	}
	if !containsAgent(result, "agent-3") {
		t.Error("agent-3 (distinct skill) not in result")
	}
}

func TestSelectDiverseCandidates_OverlappingSkillsExcluded(t *testing.T) {
	founder := agentWithSkills("founder-1", domain.SkillCodeGen)
	// agent-2 shares CodeGen with the founder — no new skill contributed.
	a2 := agentWithSkills("agent-2", domain.SkillCodeGen)
	// agent-3 is distinct.
	a3 := agentWithSkills("agent-3", domain.SkillAnalysis)
	candidates := []*agentprogression.Agent{founder, a2, a3}

	result := selectDiverseCandidates(candidates, 3, "founder-1")

	// a2 overlaps completely with the founder and should not be selected
	// when a distinct candidate (a3) fills the remaining slot.
	if containsAgent(result, "agent-2") && !containsAgent(result, "agent-3") {
		t.Error("overlapping candidate selected over a diverse one")
	}
	// founder must always be present.
	if !containsAgent(result, "founder-1") {
		t.Error("founder missing from result")
	}
}

func TestSelectDiverseCandidates_CountBoundary_Zero(t *testing.T) {
	founder := agentWithSkills("founder-1", domain.SkillCodeGen)
	a2 := agentWithSkills("agent-2", domain.SkillAnalysis)
	candidates := []*agentprogression.Agent{founder, a2}

	// count=0: loop stops immediately after adding founder (len >= 0), so
	// founder is added before the break condition is checked at iteration start.
	// In practice, the function still returns the founder since it is inserted
	// unconditionally before the count check.
	result := selectDiverseCandidates(candidates, 0, "founder-1")
	// The loop exits immediately for non-founder candidates, but the founder
	// is appended before the loop. Result length must be <= 1.
	if len(result) > 1 {
		t.Errorf("expected at most 1 result for count=0, got %d", len(result))
	}
}

func TestSelectDiverseCandidates_CountBoundary_ExactMatch(t *testing.T) {
	founder := agentWithSkills("founder-1", domain.SkillCodeGen)
	a2 := agentWithSkills("agent-2", domain.SkillAnalysis)
	a3 := agentWithSkills("agent-3", domain.SkillResearch)
	// 4 candidates, count=3 — should stop at 3.
	a4 := agentWithSkills("agent-4", domain.SkillPlanning)
	candidates := []*agentprogression.Agent{founder, a2, a3, a4}

	result := selectDiverseCandidates(candidates, 3, "founder-1")

	if len(result) > 3 {
		t.Errorf("expected at most 3 candidates, got %d", len(result))
	}
}

func TestSelectDiverseCandidates_CountLargerThanCandidates(t *testing.T) {
	founder := agentWithSkills("founder-1", domain.SkillCodeGen)
	a2 := agentWithSkills("agent-2", domain.SkillAnalysis)
	candidates := []*agentprogression.Agent{founder, a2}

	// count=10 but only 2 candidates exist.
	result := selectDiverseCandidates(candidates, 10, "founder-1")

	if len(result) > len(candidates) {
		t.Errorf("result length %d exceeds number of candidates %d", len(result), len(candidates))
	}
}

func TestSelectDiverseCandidates_FounderNotInCandidateList(t *testing.T) {
	// founderID is not present in candidates — function should not panic.
	a1 := agentWithSkills("agent-1", domain.SkillCodeGen)
	a2 := agentWithSkills("agent-2", domain.SkillAnalysis)
	candidates := []*agentprogression.Agent{a1, a2}

	// Should not panic; founder is simply absent from the result.
	result := selectDiverseCandidates(candidates, 3, "missing-founder")

	// Founder not found — the initial founder-insertion loop finds nothing;
	// the second pass proceeds normally without a pre-seeded founder.
	if len(result) > len(candidates) {
		t.Errorf("result length %d exceeds candidate count %d", len(result), len(candidates))
	}
}

// =============================================================================
// GuildCreatedPayload.Validate
// =============================================================================

func TestGuildCreatedPayload_Validate_Valid(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild:     domain.Guild{ID: "guild-1"},
		FounderID: "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildCreatedPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild:     domain.Guild{ID: ""},
		FounderID: "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

func TestGuildCreatedPayload_Validate_MissingFounderID(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild:     domain.Guild{ID: "guild-1"},
		FounderID: "",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing founder_id, got nil")
	}
}

func TestGuildCreatedPayload_Validate_ZeroTimestamp(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild:     domain.Guild{ID: "guild-1"},
		FounderID: "agent-1",
		Timestamp: time.Time{},
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for zero timestamp, got nil")
	}
}

// =============================================================================
// GuildJoinedPayload.Validate
// =============================================================================

func TestGuildJoinedPayload_Validate_Valid(t *testing.T) {
	p := &GuildJoinedPayload{
		GuildID:   "guild-1",
		AgentID:   "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildJoinedPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildJoinedPayload{
		GuildID:   "",
		AgentID:   "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

func TestGuildJoinedPayload_Validate_MissingAgentID(t *testing.T) {
	p := &GuildJoinedPayload{
		GuildID:   "guild-1",
		AgentID:   "",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing agent_id, got nil")
	}
}

// =============================================================================
// GuildLeftPayload.Validate
// =============================================================================

func TestGuildLeftPayload_Validate_Valid(t *testing.T) {
	p := &GuildLeftPayload{
		GuildID:   "guild-1",
		AgentID:   "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildLeftPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildLeftPayload{
		GuildID: "",
		AgentID: "agent-1",
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

func TestGuildLeftPayload_Validate_MissingAgentID(t *testing.T) {
	p := &GuildLeftPayload{
		GuildID: "guild-1",
		AgentID: "",
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing agent_id, got nil")
	}
}

// =============================================================================
// GuildPromotedPayload.Validate
// =============================================================================

func TestGuildPromotedPayload_Validate_Valid(t *testing.T) {
	p := &GuildPromotedPayload{
		GuildID:   "guild-1",
		AgentID:   "agent-1",
		OldRank:   domain.GuildRankInitiate,
		NewRank:   domain.GuildRankMember,
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildPromotedPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildPromotedPayload{
		GuildID: "",
		AgentID: "agent-1",
		NewRank: domain.GuildRankMember,
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

func TestGuildPromotedPayload_Validate_MissingAgentID(t *testing.T) {
	p := &GuildPromotedPayload{
		GuildID: "guild-1",
		AgentID: "",
		NewRank: domain.GuildRankMember,
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing agent_id, got nil")
	}
}

// =============================================================================
// GuildDisbandedPayload.Validate
// =============================================================================

func TestGuildDisbandedPayload_Validate_Valid(t *testing.T) {
	p := &GuildDisbandedPayload{
		GuildID:   "guild-1",
		Reason:    "quest objectives met",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildDisbandedPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildDisbandedPayload{
		GuildID:   "",
		Reason:    "some reason",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

// GuildDisbandedPayload.Validate only requires GuildID — Reason and Timestamp
// are optional fields, so a missing reason must not be an error.
func TestGuildDisbandedPayload_Validate_EmptyReasonOK(t *testing.T) {
	p := &GuildDisbandedPayload{
		GuildID:   "guild-1",
		Reason:    "",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error for empty reason: %v", err)
	}
}

// =============================================================================
// Triples — GuildCreatedPayload
// =============================================================================

func TestGuildCreatedPayload_Triples(t *testing.T) {
	now := time.Now()
	guild := domain.Guild{
		ID:     "guild-abc",
		Name:   "Ironclad's Guild",
		Status: domain.GuildActive,
	}
	p := &GuildCreatedPayload{
		Guild:     guild,
		FounderID: "agent-founder",
		Timestamp: now,
	}

	triples := p.Triples()

	if len(triples) == 0 {
		t.Fatal("Triples() returned empty slice")
	}

	// Verify the "created_by" triple is included.
	var foundCreatedBy bool
	for _, tr := range triples {
		if tr.Predicate == "guild.event.created_by" {
			foundCreatedBy = true
			if tr.Subject != "guild-abc" {
				t.Errorf("created_by triple subject = %q; want %q", tr.Subject, "guild-abc")
			}
			if tr.Object != "agent-founder" {
				t.Errorf("created_by triple object = %q; want %q", tr.Object, "agent-founder")
			}
			if tr.Source != "guildformation" {
				t.Errorf("created_by triple source = %q; want %q", tr.Source, "guildformation")
			}
		}
	}
	if !foundCreatedBy {
		t.Error("Triples() missing guild.event.created_by triple")
	}
}

func TestGuildCreatedPayload_EntityID(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild: domain.Guild{ID: "guild-xyz"},
	}
	if got := p.EntityID(); got != "guild-xyz" {
		t.Errorf("EntityID() = %q; want %q", got, "guild-xyz")
	}
}

// =============================================================================
// Triples — GuildJoinedPayload
// =============================================================================

func TestGuildJoinedPayload_Triples(t *testing.T) {
	now := time.Now()
	p := &GuildJoinedPayload{
		GuildID:   "guild-1",
		GuildName: "Ironclad's Guild",
		AgentID:   "agent-42",
		Rank:      domain.GuildRankInitiate,
		Timestamp: now,
	}

	triples := p.Triples()

	if len(triples) != 2 {
		t.Fatalf("Triples() returned %d triples; want 2", len(triples))
	}

	// First triple: membership fact.
	memberTriple := findTriple(triples, "guild.membership.member")
	if memberTriple == nil {
		t.Fatal("missing guild.membership.member triple")
	}
	if memberTriple.Object != "agent-42" {
		t.Errorf("member triple object = %q; want %q", memberTriple.Object, "agent-42")
	}

	// Second triple: rank fact.
	rankPredicate := "guild.member.agent-42.rank"
	rankTriple := findTriple(triples, rankPredicate)
	if rankTriple == nil {
		t.Fatalf("missing rank triple with predicate %q", rankPredicate)
	}
	if rankTriple.Object != string(domain.GuildRankInitiate) {
		t.Errorf("rank triple object = %q; want %q", rankTriple.Object, domain.GuildRankInitiate)
	}
}

func TestGuildJoinedPayload_EntityID(t *testing.T) {
	p := &GuildJoinedPayload{GuildID: "guild-9"}
	if got := p.EntityID(); got != "guild-9" {
		t.Errorf("EntityID() = %q; want %q", got, "guild-9")
	}
}

// =============================================================================
// Triples — GuildLeftPayload
// =============================================================================

func TestGuildLeftPayload_Triples(t *testing.T) {
	now := time.Now()
	p := &GuildLeftPayload{
		GuildID:   "guild-1",
		GuildName: "Ironclad's Guild",
		AgentID:   "agent-99",
		Reason:    "retired",
		Timestamp: now,
	}

	triples := p.Triples()

	if len(triples) != 1 {
		t.Fatalf("Triples() returned %d triples; want 1", len(triples))
	}

	tr := triples[0]
	if tr.Predicate != "guild.membership.left" {
		t.Errorf("Predicate = %q; want %q", tr.Predicate, "guild.membership.left")
	}
	if tr.Subject != "guild-1" {
		t.Errorf("Subject = %q; want %q", tr.Subject, "guild-1")
	}
	if tr.Object != "agent-99" {
		t.Errorf("Object = %q; want %q", tr.Object, "agent-99")
	}
}

func TestGuildLeftPayload_EntityID(t *testing.T) {
	p := &GuildLeftPayload{GuildID: "guild-left-1"}
	if got := p.EntityID(); got != "guild-left-1" {
		t.Errorf("EntityID() = %q; want %q", got, "guild-left-1")
	}
}

// =============================================================================
// Triples — GuildPromotedPayload
// =============================================================================

func TestGuildPromotedPayload_Triples(t *testing.T) {
	now := time.Now()
	p := &GuildPromotedPayload{
		GuildID:   "guild-2",
		GuildName: "Silver Swords",
		AgentID:   "agent-7",
		OldRank:   domain.GuildRankInitiate,
		NewRank:   domain.GuildRankVeteran,
		Timestamp: now,
	}

	triples := p.Triples()

	if len(triples) != 1 {
		t.Fatalf("Triples() returned %d triples; want 1", len(triples))
	}

	tr := triples[0]
	expectedPredicate := "guild.member.agent-7.rank"
	if tr.Predicate != expectedPredicate {
		t.Errorf("Predicate = %q; want %q", tr.Predicate, expectedPredicate)
	}
	if tr.Object != string(domain.GuildRankVeteran) {
		t.Errorf("Object = %q; want %q", tr.Object, domain.GuildRankVeteran)
	}
	if tr.Subject != "guild-2" {
		t.Errorf("Subject = %q; want %q", tr.Subject, "guild-2")
	}
}

func TestGuildPromotedPayload_EntityID(t *testing.T) {
	p := &GuildPromotedPayload{GuildID: "guild-promo"}
	if got := p.EntityID(); got != "guild-promo" {
		t.Errorf("EntityID() = %q; want %q", got, "guild-promo")
	}
}

// =============================================================================
// Triples — GuildDisbandedPayload
// =============================================================================

func TestGuildDisbandedPayload_Triples(t *testing.T) {
	now := time.Now()
	p := &GuildDisbandedPayload{
		GuildID:          "guild-3",
		GuildName:        "Fallen Order",
		Reason:           "leadership vacuum",
		FinalMemberCount: 5,
		Timestamp:        now,
	}

	triples := p.Triples()

	if len(triples) != 2 {
		t.Fatalf("Triples() returned %d triples; want 2", len(triples))
	}

	// State triple: guild.status.state -> inactive.
	stateTriple := findTriple(triples, "guild.status.state")
	if stateTriple == nil {
		t.Fatal("missing guild.status.state triple")
	}
	if stateTriple.Object != string(domain.GuildInactive) {
		t.Errorf("state triple object = %q; want %q", stateTriple.Object, domain.GuildInactive)
	}

	// Disbanded-at triple.
	disbandedTriple := findTriple(triples, "guild.lifecycle.disbanded_at")
	if disbandedTriple == nil {
		t.Fatal("missing guild.lifecycle.disbanded_at triple")
	}
	// The value should be the RFC3339 formatted timestamp.
	wantTime := now.Format(time.RFC3339)
	if disbandedTriple.Object != wantTime {
		t.Errorf("disbanded_at object = %q; want %q", disbandedTriple.Object, wantTime)
	}
}

func TestGuildDisbandedPayload_EntityID(t *testing.T) {
	p := &GuildDisbandedPayload{GuildID: "guild-disband"}
	if got := p.EntityID(); got != "guild-disband" {
		t.Errorf("EntityID() = %q; want %q", got, "guild-disband")
	}
}

// =============================================================================
// Payload Schema methods — sanity check domain/category/version
// =============================================================================

func TestPayload_Schema(t *testing.T) {
	tests := []struct {
		name   string
		schema func() interface {
			Schema() interface{ GetDomain() string }
		}
		wantDomain   string
		wantCategory string
		wantVersion  string
	}{
		// We test Schema() values directly through the concrete types.
	}
	_ = tests // reserved for future expansion via table-driven schema check

	t.Run("GuildCreatedPayload schema", func(t *testing.T) {
		p := &GuildCreatedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q; want semdragons", s.Domain)
		}
		if s.Category != "guild.created" {
			t.Errorf("Category = %q; want guild.created", s.Category)
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q; want v1", s.Version)
		}
	})

	t.Run("GuildJoinedPayload schema", func(t *testing.T) {
		p := &GuildJoinedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q; want semdragons", s.Domain)
		}
		if s.Category != "guild.joined" {
			t.Errorf("Category = %q; want guild.joined", s.Category)
		}
	})

	t.Run("GuildLeftPayload schema", func(t *testing.T) {
		p := &GuildLeftPayload{}
		s := p.Schema()
		if s.Category != "guild.left" {
			t.Errorf("Category = %q; want guild.left", s.Category)
		}
	})

	t.Run("GuildPromotedPayload schema", func(t *testing.T) {
		p := &GuildPromotedPayload{}
		s := p.Schema()
		if s.Category != "guild.promoted" {
			t.Errorf("Category = %q; want guild.promoted", s.Category)
		}
	})

	t.Run("GuildDisbandedPayload schema", func(t *testing.T) {
		p := &GuildDisbandedPayload{}
		s := p.Schema()
		if s.Category != "guild.disbanded" {
			t.Errorf("Category = %q; want guild.disbanded", s.Category)
		}
	})
}

// =============================================================================
// Triples source field verification
// =============================================================================

func TestTriples_SourceIsGuildformation(t *testing.T) {
	now := time.Now()

	payloads := []interface {
		Triples() []message.Triple
	}{
		&GuildJoinedPayload{
			GuildID: "g1", AgentID: "a1", Rank: domain.GuildRankInitiate, Timestamp: now,
		},
		&GuildLeftPayload{
			GuildID: "g1", AgentID: "a1", Timestamp: now,
		},
		&GuildPromotedPayload{
			GuildID: "g1", AgentID: "a1", NewRank: domain.GuildRankMember, Timestamp: now,
		},
		&GuildDisbandedPayload{
			GuildID: "g1", Timestamp: now,
		},
	}

	for _, p := range payloads {
		for _, tr := range p.Triples() {
			if tr.Source != "guildformation" {
				t.Errorf("triple source = %q; want guildformation (predicate: %s)", tr.Source, tr.Predicate)
			}
		}
	}
}

func TestTriples_ConfidenceIsOne(t *testing.T) {
	now := time.Now()

	payloads := []interface {
		Triples() []message.Triple
	}{
		&GuildJoinedPayload{
			GuildID: "g1", AgentID: "a1", Rank: domain.GuildRankInitiate, Timestamp: now,
		},
		&GuildLeftPayload{
			GuildID: "g1", AgentID: "a1", Timestamp: now,
		},
		&GuildPromotedPayload{
			GuildID: "g1", AgentID: "a1", NewRank: domain.GuildRankMember, Timestamp: now,
		},
		&GuildDisbandedPayload{
			GuildID: "g1", Timestamp: now,
		},
	}

	for _, p := range payloads {
		for _, tr := range p.Triples() {
			if tr.Confidence != 1.0 {
				t.Errorf("triple confidence = %v; want 1.0 (predicate: %s)", tr.Confidence, tr.Predicate)
			}
		}
	}
}

// =============================================================================
// isMember and getMember helpers (internal, tested via white-box access)
// =============================================================================

func TestIsMember(t *testing.T) {
	guild := &domain.Guild{
		Members: []domain.GuildMember{
			{AgentID: "agent-1", Rank: domain.GuildRankMaster},
			{AgentID: "agent-2", Rank: domain.GuildRankInitiate},
		},
	}

	if !isMember(guild, "agent-1") {
		t.Error("isMember: expected true for agent-1, got false")
	}
	if !isMember(guild, "agent-2") {
		t.Error("isMember: expected true for agent-2, got false")
	}
	if isMember(guild, "agent-3") {
		t.Error("isMember: expected false for agent-3, got true")
	}
}

func TestIsMember_EmptyMembers(t *testing.T) {
	guild := &domain.Guild{Members: nil}
	if isMember(guild, "agent-1") {
		t.Error("isMember: expected false for empty members slice, got true")
	}
}

func TestGetMember_ReturnsPointerToSliceElement(t *testing.T) {
	guild := &domain.Guild{
		Members: []domain.GuildMember{
			{AgentID: "agent-1", Rank: domain.GuildRankInitiate},
		},
	}

	m := getMember(guild, "agent-1")
	if m == nil {
		t.Fatal("getMember: expected non-nil pointer, got nil")
	}
	// Mutating through the pointer must affect the original slice.
	m.Rank = domain.GuildRankVeteran
	if guild.Members[0].Rank != domain.GuildRankVeteran {
		t.Error("getMember: pointer mutation did not propagate to slice")
	}
}

func TestGetMember_NilForAbsentAgent(t *testing.T) {
	guild := &domain.Guild{
		Members: []domain.GuildMember{
			{AgentID: "agent-1"},
		},
	}
	m := getMember(guild, "agent-99")
	if m != nil {
		t.Errorf("getMember: expected nil for absent agent, got %+v", m)
	}
}

// =============================================================================
// Test helper
// =============================================================================

// findTriple returns the first triple whose Predicate matches, or nil.
func findTriple(triples []message.Triple, predicate string) *message.Triple {
	for i := range triples {
		if triples[i].Predicate == predicate {
			return &triples[i]
		}
	}
	return nil
}
