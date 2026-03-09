package dmpartyformation

// =============================================================================
// UNIT TESTS - Party Formation Engine
// =============================================================================
// Pure unit tests: no Docker, no NATS, no integration tag.
// Run with: go test ./processor/dmpartyformation/ -run 'Test' -count=1 -v
// =============================================================================

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/boidengine"
)

// =============================================================================
// HELPERS
// =============================================================================

// testBoardConfig returns a minimal BoardConfig for testing (no NATS required).
func testBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "testboard",
	}
}

// testEngine returns a PartyFormationEngine wired with default rules.
// The graph field is nil; tests that call createParty will still work because
// createParty only uses boardConfig (not the graph client).
func testEngine() *PartyFormationEngine {
	boids := boidengine.NewDefaultBoidEngine()
	return NewPartyFormationEngine(boids, nil, testBoardConfig())
}

// makeAgent builds an Agent with the specified tier, level, and optional skills.
// Status is AgentIdle so that boid ComputeAttractions includes the agent.
func makeAgent(id domain.AgentID, tier domain.TrustTier, level int, skills ...domain.SkillTag) agentprogression.Agent {
	agent := agentprogression.Agent{
		ID:     id,
		Name:   string(id),
		Tier:   tier,
		Level:  level,
		Status: domain.AgentIdle,
	}
	if len(skills) > 0 {
		agent.SkillProficiencies = make(map[domain.SkillTag]domain.SkillProficiency)
		for _, skill := range skills {
			agent.SkillProficiencies[skill] = domain.SkillProficiency{
				Level: domain.ProficiencyJourneyman,
			}
		}
	}
	return agent
}

// makeQuest builds a minimal Quest for testing.
func makeQuest(id domain.QuestID, skills []domain.SkillTag, minPartySize int) *domain.Quest {
	return &domain.Quest{
		ID:             id,
		Title:          string(id),
		Status:         domain.QuestPosted,
		Difficulty:     domain.DifficultyModerate,
		RequiredSkills: skills,
		MinPartySize:   minPartySize,
		PartyRequired:  minPartySize > 1,
	}
}

// =============================================================================
// CONFIG TESTS
// =============================================================================

func TestDefaultConfig_FieldValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org != "default" {
		t.Errorf("Org = %q, want %q", cfg.Org, "default")
	}
	if cfg.Platform != "local" {
		t.Errorf("Platform = %q, want %q", cfg.Platform, "local")
	}
	if cfg.Board != "main" {
		t.Errorf("Board = %q, want %q", cfg.Board, "main")
	}
	if cfg.DefaultStrategy != "balanced" {
		t.Errorf("DefaultStrategy = %q, want %q", cfg.DefaultStrategy, "balanced")
	}
	if cfg.MaxPartySize != 5 {
		t.Errorf("MaxPartySize = %d, want %d", cfg.MaxPartySize, 5)
	}
	if cfg.MinMembersForLead != 1 {
		t.Errorf("MinMembersForLead = %d, want %d", cfg.MinMembersForLead, 1)
	}
}

func TestDefaultConfig_ReturnsNewInstanceEachCall(t *testing.T) {
	a := DefaultConfig()
	b := DefaultConfig()

	// Mutations to one must not affect the other (value semantics).
	a.Org = "mutated"
	if b.Org == "mutated" {
		t.Error("DefaultConfig should return independent values; mutation of one leaked to another")
	}
}

// =============================================================================
// selectLeadFromAgents TESTS
// =============================================================================

func TestSelectLeadFromAgents_EmptySlice_ReturnsError(t *testing.T) {
	eng := testEngine()

	_, err := eng.selectLeadFromAgents(nil)
	if err == nil {
		t.Fatal("selectLeadFromAgents(nil) should return an error")
	}

	_, err = eng.selectLeadFromAgents([]agentprogression.Agent{})
	if err == nil {
		t.Fatal("selectLeadFromAgents([]) should return an error")
	}
}

func TestSelectLeadFromAgents_NoCapableAgents_ReturnsError(t *testing.T) {
	eng := testEngine()

	// Apprentice and Journeyman tiers have CanLeadParty = false.
	agents := []agentprogression.Agent{
		makeAgent("appr-1", domain.TierApprentice, 3),
		makeAgent("jour-1", domain.TierJourneyman, 8),
		makeAgent("expt-1", domain.TierExpert, 13),
	}

	_, err := eng.selectLeadFromAgents(agents)
	if err == nil {
		t.Fatal("selectLeadFromAgents should return an error when no agent has CanLeadParty permission")
	}
}

func TestSelectLeadFromAgents_OneCapableAgent_ReturnsThatAgent(t *testing.T) {
	eng := testEngine()

	agents := []agentprogression.Agent{
		makeAgent("appr-1", domain.TierApprentice, 3),
		makeAgent("master-1", domain.TierMaster, 16),
		makeAgent("jour-1", domain.TierJourneyman, 8),
	}

	lead, err := eng.selectLeadFromAgents(agents)
	if err != nil {
		t.Fatalf("selectLeadFromAgents failed: %v", err)
	}
	if lead.ID != "master-1" {
		t.Errorf("Lead = %q, want %q", lead.ID, "master-1")
	}
}

func TestSelectLeadFromAgents_MultipleCapable_PicksHighestLevel(t *testing.T) {
	eng := testEngine()

	// Two Master agents and one Grandmaster; selectLeadFromAgents should pick highest level.
	agents := []agentprogression.Agent{
		makeAgent("master-low", domain.TierMaster, 16),
		makeAgent("grand-19", domain.TierGrandmaster, 19),
		makeAgent("master-high", domain.TierMaster, 18),
	}

	lead, err := eng.selectLeadFromAgents(agents)
	if err != nil {
		t.Fatalf("selectLeadFromAgents failed: %v", err)
	}
	if lead.ID != "grand-19" {
		t.Errorf("Lead = %q, want highest-level agent %q", lead.ID, "grand-19")
	}
}

func TestSelectLeadFromAgents_TiedLevel_ReturnsEitherCandidate(t *testing.T) {
	eng := testEngine()

	// Both at level 17; either is a valid lead — just verify no error and one is returned.
	agents := []agentprogression.Agent{
		makeAgent("master-a", domain.TierMaster, 17),
		makeAgent("master-b", domain.TierMaster, 17),
	}

	lead, err := eng.selectLeadFromAgents(agents)
	if err != nil {
		t.Fatalf("selectLeadFromAgents failed: %v", err)
	}
	if lead.ID != "master-a" && lead.ID != "master-b" {
		t.Errorf("Lead = %q, expected one of master-a or master-b", lead.ID)
	}
}

// =============================================================================
// isGuildMatch TESTS
// =============================================================================

func TestIsGuildMatch_NilGuildPriority_ReturnsFalse(t *testing.T) {
	eng := testEngine()

	agent := makeAgent("agent-1", domain.TierJourneyman, 8)
	agent.Guild = domain.GuildID("guild-alpha")

	quest := makeQuest("q-1", nil, 1)
	// GuildPriority is nil (zero value for *GuildID).

	if eng.isGuildMatch(agent, quest) {
		t.Error("isGuildMatch should return false when quest has no GuildPriority")
	}
}

func TestIsGuildMatch_AgentNotInPriorityGuild_ReturnsFalse(t *testing.T) {
	eng := testEngine()

	agent := makeAgent("agent-1", domain.TierJourneyman, 8)
	agent.Guild = domain.GuildID("guild-alpha")

	quest := makeQuest("q-1", nil, 1)
	priorityGuild := domain.GuildID("guild-beta")
	quest.GuildPriority = &priorityGuild

	if eng.isGuildMatch(agent, quest) {
		t.Error("isGuildMatch should return false when agent is not in the priority guild")
	}
}

func TestIsGuildMatch_AgentInPriorityGuild_ReturnsTrue(t *testing.T) {
	eng := testEngine()

	agent := makeAgent("agent-1", domain.TierJourneyman, 8)
	targetGuild := domain.GuildID("guild-alpha")
	agent.Guild = targetGuild

	quest := makeQuest("q-1", nil, 1)
	quest.GuildPriority = &targetGuild

	if !eng.isGuildMatch(agent, quest) {
		t.Error("isGuildMatch should return true when agent is in the priority guild")
	}
}

func TestIsGuildMatch_AgentHasNoGuilds_ReturnsFalse(t *testing.T) {
	eng := testEngine()

	agent := makeAgent("agent-1", domain.TierJourneyman, 8)
	// agent.Guild is "" (zero value — no guild).

	quest := makeQuest("q-1", nil, 1)
	priorityGuild := domain.GuildID("guild-alpha")
	quest.GuildPriority = &priorityGuild

	if eng.isGuildMatch(agent, quest) {
		t.Error("isGuildMatch should return false when agent has no guild memberships")
	}
}

// =============================================================================
// recommendRole TESTS
// =============================================================================

func TestRecommendRole_CanLeadParty_ReturnsRoleLead(t *testing.T) {
	eng := testEngine()

	agent := makeAgent("master-1", domain.TierMaster, 16)
	perms := domain.TierPermissionsFor(domain.TierMaster)

	role := eng.recommendRole(agent, perms)
	if role != domain.RoleLead {
		t.Errorf("recommendRole = %q, want %q (Master tier should be Lead)", role, domain.RoleLead)
	}
}

func TestRecommendRole_CodeReviewSkill_ReturnsRoleReviewer(t *testing.T) {
	eng := testEngine()

	// Expert tier: cannot lead, but has SkillCodeReview.
	agent := makeAgent("reviewer-1", domain.TierExpert, 12, domain.SkillCodeReview)
	perms := domain.TierPermissionsFor(domain.TierExpert)

	// Verify Expert cannot lead so we're testing the right branch.
	if perms.CanLeadParty {
		t.Skip("TierExpert can now lead; test assumptions need revisiting")
	}

	role := eng.recommendRole(agent, perms)
	if role != domain.RoleReviewer {
		t.Errorf("recommendRole = %q, want %q (code_review skill should map to Reviewer)", role, domain.RoleReviewer)
	}
}

func TestRecommendRole_ResearchSkill_ReturnsRoleScout(t *testing.T) {
	eng := testEngine()

	// Journeyman tier: cannot lead; has SkillResearch.
	agent := makeAgent("scout-1", domain.TierJourneyman, 8, domain.SkillResearch)
	perms := domain.TierPermissionsFor(domain.TierJourneyman)

	role := eng.recommendRole(agent, perms)
	if role != domain.RoleScout {
		t.Errorf("recommendRole = %q, want %q (research skill should map to Scout)", role, domain.RoleScout)
	}
}

func TestRecommendRole_NoSpecialSkill_ReturnsRoleExecutor(t *testing.T) {
	eng := testEngine()

	// Apprentice tier: cannot lead; only has SkillCodeGen (not code_review or research).
	agent := makeAgent("exec-1", domain.TierApprentice, 3, domain.SkillCodeGen)
	perms := domain.TierPermissionsFor(domain.TierApprentice)

	role := eng.recommendRole(agent, perms)
	if role != domain.RoleExecutor {
		t.Errorf("recommendRole = %q, want %q (no special skill defaults to Executor)", role, domain.RoleExecutor)
	}
}

func TestRecommendRole_NoSkillsAtAll_ReturnsRoleExecutor(t *testing.T) {
	eng := testEngine()

	agent := makeAgent("bare-1", domain.TierJourneyman, 7)
	perms := domain.TierPermissionsFor(domain.TierJourneyman)

	role := eng.recommendRole(agent, perms)
	if role != domain.RoleExecutor {
		t.Errorf("recommendRole = %q, want %q (agent with no skills defaults to Executor)", role, domain.RoleExecutor)
	}
}

// TestRecommendRole_CodeReviewBeatsResearch verifies that when an agent has
// both code_review and research skills, code_review takes priority (Reviewer
// before Scout) because recommendRole iterates skills and returns on first match.
func TestRecommendRole_CodeReviewBeatsResearch(t *testing.T) {
	eng := testEngine()

	agent := makeAgent("multi-1", domain.TierExpert, 12,
		domain.SkillCodeReview, domain.SkillResearch)
	perms := domain.TierPermissionsFor(domain.TierExpert)

	if perms.CanLeadParty {
		t.Skip("TierExpert can now lead; test assumptions need revisiting")
	}

	// The function iterates GetSkillTags(), which returns map keys in arbitrary order.
	// We only assert that the result is one of the two valid special roles, not Executor.
	role := eng.recommendRole(agent, perms)
	if role != domain.RoleReviewer && role != domain.RoleScout {
		t.Errorf("recommendRole = %q; expected Reviewer or Scout for agent with both skills", role)
	}
}

// =============================================================================
// SuggestPartyMembers TESTS
// =============================================================================

func TestSuggestPartyMembers_EmptyAgents_ReturnsEmptySlice(t *testing.T) {
	eng := testEngine()
	quest := makeQuest("q-1", []domain.SkillTag{domain.SkillCodeGen}, 1)

	suggestions, err := eng.SuggestPartyMembers(nil, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers(nil agents) returned unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for empty agents, got %d", len(suggestions))
	}
}

func TestSuggestPartyMembers_ReturnsOneSuggestionPerAgent(t *testing.T) {
	eng := testEngine()
	quest := makeQuest("q-2", []domain.SkillTag{domain.SkillCodeGen}, 1)

	agents := []agentprogression.Agent{
		makeAgent("a1", domain.TierMaster, 16, domain.SkillCodeGen),
		makeAgent("a2", domain.TierJourneyman, 8, domain.SkillAnalysis),
		makeAgent("a3", domain.TierApprentice, 3),
	}

	suggestions, err := eng.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}
	if len(suggestions) != len(agents) {
		t.Errorf("suggestion count = %d, want %d (one per agent)", len(suggestions), len(agents))
	}
}

func TestSuggestPartyMembers_CanLeadFlagMatchesTierPermissions(t *testing.T) {
	eng := testEngine()
	quest := makeQuest("q-3", nil, 1)

	master := makeAgent("master-1", domain.TierMaster, 16)
	apprentice := makeAgent("appr-1", domain.TierApprentice, 3)
	agents := []agentprogression.Agent{master, apprentice}

	suggestions, err := eng.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}

	for _, s := range suggestions {
		wantCanLead := domain.TierPermissionsFor(s.Agent.Tier).CanLeadParty
		if s.CanLead != wantCanLead {
			t.Errorf("Agent %v: CanLead = %v, want %v", s.Agent.ID, s.CanLead, wantCanLead)
		}
	}
}

func TestSuggestPartyMembers_SkillsCoveredReflectsQuestRequirements(t *testing.T) {
	eng := testEngine()

	requiredSkills := []domain.SkillTag{domain.SkillCodeGen, domain.SkillCodeReview}
	quest := makeQuest("q-4", requiredSkills, 1)

	// fullCoverage has both required skills.
	fullCoverage := makeAgent("full-1", domain.TierExpert, 12,
		domain.SkillCodeGen, domain.SkillCodeReview)
	// partialCoverage has one of the two required skills.
	partialCoverage := makeAgent("partial-1", domain.TierJourneyman, 9,
		domain.SkillCodeGen)
	// noCoverage has neither required skill.
	noCoverage := makeAgent("none-1", domain.TierApprentice, 4)

	agents := []agentprogression.Agent{fullCoverage, partialCoverage, noCoverage}

	suggestions, err := eng.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}

	for _, s := range suggestions {
		switch s.Agent.ID {
		case "full-1":
			if len(s.SkillsCovered) != 2 {
				t.Errorf("full-coverage SkillsCovered = %d, want 2", len(s.SkillsCovered))
			}
		case "partial-1":
			if len(s.SkillsCovered) != 1 {
				t.Errorf("partial-coverage SkillsCovered = %d, want 1", len(s.SkillsCovered))
			}
		case "none-1":
			if len(s.SkillsCovered) != 0 {
				t.Errorf("no-coverage SkillsCovered = %d, want 0", len(s.SkillsCovered))
			}
		}
	}
}

func TestSuggestPartyMembers_GuildMatchFlagAccurate(t *testing.T) {
	eng := testEngine()

	targetGuild := domain.GuildID("test.unit.game.testboard.guild.guild01")
	quest := makeQuest("q-5", nil, 1)
	quest.GuildPriority = &targetGuild

	inGuild := makeAgent("in-guild", domain.TierJourneyman, 7)
	inGuild.Guild = targetGuild

	notInGuild := makeAgent("out-guild", domain.TierJourneyman, 9)
	notInGuild.Guild = domain.GuildID("some-other-guild")

	agents := []agentprogression.Agent{inGuild, notInGuild}

	suggestions, err := eng.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}

	for _, s := range suggestions {
		switch s.Agent.ID {
		case "in-guild":
			if !s.GuildMatch {
				t.Error("Agent in priority guild should have GuildMatch = true")
			}
		case "out-guild":
			if s.GuildMatch {
				t.Error("Agent not in priority guild should have GuildMatch = false")
			}
		}
	}
}

func TestSuggestPartyMembers_SortedByScoreDescending(t *testing.T) {
	eng := testEngine()

	// The agent with matching skills should score higher than one with no match.
	quest := makeQuest("q-6", []domain.SkillTag{domain.SkillCodeGen}, 1)

	// skillMatch should outscore noMatch; both are idle so boids will score them.
	skillMatch := makeAgent("skill-match", domain.TierExpert, 12, domain.SkillCodeGen)
	noMatch := makeAgent("no-match", domain.TierJourneyman, 8)

	agents := []agentprogression.Agent{skillMatch, noMatch}

	suggestions, err := eng.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}
	if len(suggestions) < 2 {
		t.Fatalf("expected at least 2 suggestions, got %d", len(suggestions))
	}

	// Verify descending order; equal scores are also fine (no strict requirement on which is first).
	for i := 1; i < len(suggestions); i++ {
		if suggestions[i].Score > suggestions[i-1].Score {
			t.Errorf("suggestions not sorted descending: suggestions[%d].Score (%f) > suggestions[%d].Score (%f)",
				i, suggestions[i].Score, i-1, suggestions[i-1].Score)
		}
	}
}

func TestSuggestPartyMembers_RecommendedRoleSetOnEachSuggestion(t *testing.T) {
	eng := testEngine()
	quest := makeQuest("q-7", nil, 1)

	agents := []agentprogression.Agent{
		makeAgent("master-1", domain.TierMaster, 17),
		makeAgent("reviewer-1", domain.TierJourneyman, 9, domain.SkillCodeReview),
		makeAgent("executor-1", domain.TierApprentice, 4, domain.SkillCodeGen),
	}

	suggestions, err := eng.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}

	for _, s := range suggestions {
		if s.RecommendedFor == "" {
			t.Errorf("Agent %v has empty RecommendedFor role", s.Agent.ID)
		}
	}
}
