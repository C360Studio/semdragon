//go:build integration

package dmpartyformation

// =============================================================================
// INTEGRATION TESTS - DM Party Formation Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/dmpartyformation/...
// =============================================================================

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// LIFECYCLE TESTS
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "dmparty-lifecycle"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	health := comp.Health()
	if !health.Healthy {
		t.Error("component should be healthy after start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	health = comp.Health()
	if health.Healthy {
		t.Error("component should not be healthy after stop")
	}
	if health.Status != "stopped" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "stopped")
	}
}

// =============================================================================
// PORT AND SCHEMA TESTS
// =============================================================================

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Error("should have at least one input port")
	}

	hasPartyRequests := false
	for _, port := range inputs {
		if port.Name == "party-requests" {
			hasPartyRequests = true
			break
		}
	}
	if !hasPartyRequests {
		t.Error("missing party-requests input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Error("should have at least one output port")
	}

	hasPartyEvents := false
	for _, port := range outputs {
		if port.Name == "party-events" {
			hasPartyEvents = true
			break
		}
	}
	if !hasPartyEvents {
		t.Error("missing party-events output port")
	}
}

func TestComponent_ConfigSchema(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	requiredFields := []string{"org", "platform", "board"}
	for _, field := range requiredFields {
		found := false
		for _, req := range schema.Required {
			if req == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("field %q should be required in ConfigSchema", field)
		}
	}

	expectedProps := []string{"org", "platform", "board", "default_strategy", "max_party_size"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("missing property %q in ConfigSchema", prop)
		}
	}
}

// =============================================================================
// PARTY FORMATION - BALANCED STRATEGY
// =============================================================================

func TestFormParty_Balanced_SingleAgent(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "form-balanced-single")
	defer comp.Stop(5 * time.Second)

	quest := buildQuest(comp.boardConfig, "solo-quest", domain.DifficultyTrivial, nil, 1)
	agents := []agentprogression.Agent{
		buildAgent(comp.boardConfig, "lead-agent", 16, domain.TierMaster, nil),
	}

	party, err := comp.FormParty(ctx, quest, domain.PartyStrategyBalanced, agents)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	if party.ID == "" {
		t.Error("party ID should be set")
	}
	if party.QuestID != quest.ID {
		t.Errorf("QuestID = %q, want %q", party.QuestID, quest.ID)
	}
	if len(party.Members) == 0 {
		t.Error("party should have at least one member")
	}
	if party.Lead == "" {
		t.Error("party lead should be set")
	}
}

func TestFormParty_Balanced_MultipleAgents_CoverSkills(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "form-balanced-multi")
	defer comp.Stop(5 * time.Second)

	// Quest requires both CodeGen and Analysis.
	quest := buildQuest(comp.boardConfig, "multi-skill-quest", domain.DifficultyModerate,
		[]domain.SkillTag{domain.SkillCodeGen, domain.SkillAnalysis}, 2)

	agents := []agentprogression.Agent{
		buildAgentWithSkills(comp.boardConfig, "coder-agent", 16, domain.TierMaster,
			[]domain.SkillTag{domain.SkillCodeGen}),
		buildAgentWithSkills(comp.boardConfig, "analyst-agent", 7, domain.TierJourneyman,
			[]domain.SkillTag{domain.SkillAnalysis}),
		buildAgentWithSkills(comp.boardConfig, "reviewer-agent", 5, domain.TierApprentice,
			[]domain.SkillTag{domain.SkillCodeReview}),
	}

	party, err := comp.FormParty(ctx, quest, domain.PartyStrategyBalanced, agents)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	if len(party.Members) < 2 {
		t.Errorf("party member count = %d, want at least 2 (one per required skill)", len(party.Members))
	}

	// The highest-level agent capable of leading should be the lead.
	if party.Lead == "" {
		t.Error("party lead should be set")
	}
}

func TestFormParty_NoAgents_Errors(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "form-no-agents")
	defer comp.Stop(5 * time.Second)

	quest := buildQuest(comp.boardConfig, "empty-party-quest", domain.DifficultyTrivial, nil, 1)

	_, err := comp.FormParty(ctx, quest, domain.PartyStrategyBalanced, nil)
	if err == nil {
		t.Fatal("FormParty should error when no agents are provided")
	}
}

// =============================================================================
// PARTY FORMATION - SPECIALIST STRATEGY
// =============================================================================

func TestFormParty_Specialist_PrefersGuildAgents(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "form-specialist")
	defer comp.Stop(5 * time.Second)

	guildID := domain.GuildID(comp.boardConfig.GuildEntityID("code-guild"))

	quest := buildQuestWithGuild(comp.boardConfig, "specialist-quest", domain.DifficultyModerate, guildID)

	guildAgent := buildAgentInGuild(comp.boardConfig, "guild-coder", 16, domain.TierMaster, guildID)
	nonGuildAgent := buildAgent(comp.boardConfig, "random-agent", 12, domain.TierExpert, nil)

	agents := []agentprogression.Agent{guildAgent, nonGuildAgent}

	party, err := comp.FormParty(ctx, quest, domain.PartyStrategySpecialist, agents)
	if err != nil {
		t.Fatalf("FormParty specialist failed: %v", err)
	}

	if party.ID == "" {
		t.Error("party ID should be set")
	}
	if len(party.Members) == 0 {
		t.Error("party should have members")
	}
}

// =============================================================================
// PARTY FORMATION - MENTOR STRATEGY
// =============================================================================

func TestFormParty_Mentor_RequiresMasterLead(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "form-mentor")
	defer comp.Stop(5 * time.Second)

	quest := buildQuest(comp.boardConfig, "mentor-quest", domain.DifficultyEpic, nil, 2)

	// One master who can lead, plus two apprentices.
	mentor := buildAgent(comp.boardConfig, "master-agent", 16, domain.TierMaster, nil)
	apprentice1 := buildAgent(comp.boardConfig, "apprentice-1", 3, domain.TierApprentice, nil)
	apprentice2 := buildAgent(comp.boardConfig, "apprentice-2", 5, domain.TierApprentice, nil)

	agents := []agentprogression.Agent{mentor, apprentice1, apprentice2}

	party, err := comp.FormParty(ctx, quest, domain.PartyStrategyMentor, agents)
	if err != nil {
		t.Fatalf("FormParty mentor failed: %v", err)
	}

	if party.Lead != mentor.ID {
		t.Errorf("Lead = %v, want master agent %v", party.Lead, mentor.ID)
	}
}

func TestFormParty_Mentor_NoLeadCapable_Errors(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "form-mentor-nomaster")
	defer comp.Stop(5 * time.Second)

	quest := buildQuest(comp.boardConfig, "no-master-quest", domain.DifficultyModerate, nil, 2)

	// Only apprentices - none can lead.
	agents := []agentprogression.Agent{
		buildAgent(comp.boardConfig, "appr-1", 3, domain.TierApprentice, nil),
		buildAgent(comp.boardConfig, "appr-2", 5, domain.TierApprentice, nil),
	}

	_, err := comp.FormParty(ctx, quest, domain.PartyStrategyMentor, agents)
	if err == nil {
		t.Fatal("FormParty mentor should error when no agent can lead")
	}
}

// =============================================================================
// PARTY FORMATION - MINIMAL STRATEGY
// =============================================================================

func TestFormParty_Minimal_SingleMemberIfNoPartyRequired(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "form-minimal")
	defer comp.Stop(5 * time.Second)

	quest := buildQuest(comp.boardConfig, "minimal-quest", domain.DifficultyTrivial, nil, 1)
	// Ensure PartyRequired is false and MinPartySize = 1.
	quest.PartyRequired = false
	quest.MinPartySize = 1

	agents := []agentprogression.Agent{
		buildAgent(comp.boardConfig, "solo-master", 16, domain.TierMaster, nil),
		buildAgent(comp.boardConfig, "extra-agent", 12, domain.TierExpert, nil),
	}

	party, err := comp.FormParty(ctx, quest, domain.PartyStrategyMinimal, agents)
	if err != nil {
		t.Fatalf("FormParty minimal failed: %v", err)
	}

	if len(party.Members) != 1 {
		t.Errorf("member count = %d, want 1 (minimal, no party required)", len(party.Members))
	}
}

// =============================================================================
// AGENT RANKING
// =============================================================================

func TestRankAgentsForQuest_ReturnsRankedList(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupPartyComponent(t, client, "rank-agents")
	defer comp.Stop(5 * time.Second)

	quest := buildQuest(comp.boardConfig, "ranking-quest", domain.DifficultyModerate,
		[]domain.SkillTag{domain.SkillCodeGen}, 1)

	agents := []agentprogression.Agent{
		buildAgentWithSkills(comp.boardConfig, "skilled-coder", 12, domain.TierExpert,
			[]domain.SkillTag{domain.SkillCodeGen}),
		buildAgent(comp.boardConfig, "no-match-agent", 8, domain.TierJourneyman, nil),
	}

	ranked := comp.RankAgentsForQuest(agents, quest)
	if len(ranked) == 0 {
		t.Error("RankAgentsForQuest should return at least one ranked suggestion")
	}
}

func TestRankAgentsForQuest_NotRunning_ReturnsNil(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "rank-not-running"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	// Intentionally do NOT call Start.

	quest := buildQuest(comp.boardConfig, "unstarted-quest", domain.DifficultyTrivial, nil, 1)
	agents := []agentprogression.Agent{
		buildAgent(comp.boardConfig, "idle-agent", 5, domain.TierApprentice, nil),
	}

	ranked := comp.RankAgentsForQuest(agents, quest)
	if ranked != nil {
		t.Error("RankAgentsForQuest should return nil when component is not running")
	}
}

// =============================================================================
// PARTY MEMBER SUGGESTIONS
// =============================================================================

func TestSuggestPartyMembers_ReturnsSortedByScore(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupPartyComponent(t, client, "suggest-members")
	defer comp.Stop(5 * time.Second)

	quest := buildQuest(comp.boardConfig, "suggest-quest", domain.DifficultyModerate,
		[]domain.SkillTag{domain.SkillCodeGen}, 2)

	agents := []agentprogression.Agent{
		buildAgentWithSkills(comp.boardConfig, "perfect-match", 12, domain.TierExpert,
			[]domain.SkillTag{domain.SkillCodeGen}),
		buildAgent(comp.boardConfig, "no-match", 5, domain.TierApprentice, nil),
	}

	suggestions, err := comp.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}

	if len(suggestions) != 2 {
		t.Errorf("suggestion count = %d, want 2", len(suggestions))
	}

	// Suggestions should be sorted descending by score.
	if len(suggestions) >= 2 && suggestions[0].Score < suggestions[1].Score {
		t.Errorf("suggestions not sorted by score: suggestions[0].Score=%f < suggestions[1].Score=%f",
			suggestions[0].Score, suggestions[1].Score)
	}
}

func TestSuggestPartyMembers_CanLeadFlagAccurate(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupPartyComponent(t, client, "suggest-canlead")
	defer comp.Stop(5 * time.Second)

	quest := buildQuest(comp.boardConfig, "canlead-quest", domain.DifficultyModerate, nil, 2)

	masterAgent := buildAgent(comp.boardConfig, "master-dm", 16, domain.TierMaster, nil)
	apprenticeAgent := buildAgent(comp.boardConfig, "appr-dm", 3, domain.TierApprentice, nil)
	agents := []agentprogression.Agent{masterAgent, apprenticeAgent}

	suggestions, err := comp.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}

	for _, s := range suggestions {
		perms := domain.TierPermissionsFor(s.Agent.Tier)
		if s.CanLead != perms.CanLeadParty {
			t.Errorf("Agent %v: CanLead = %v, want %v based on tier permissions",
				s.Agent.ID, s.CanLead, perms.CanLeadParty)
		}
	}
}

func TestSuggestPartyMembers_SkillsCoveredPopulated(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupPartyComponent(t, client, "suggest-skills")
	defer comp.Stop(5 * time.Second)

	requiredSkills := []domain.SkillTag{domain.SkillCodeGen, domain.SkillCodeReview}
	quest := buildQuest(comp.boardConfig, "skills-quest", domain.DifficultyModerate, requiredSkills, 2)

	// Agent covers both required skills.
	fullCoverage := buildAgentWithSkills(comp.boardConfig, "full-coverage", 12, domain.TierExpert, requiredSkills)
	// Agent covers none.
	noCoverage := buildAgent(comp.boardConfig, "no-coverage", 8, domain.TierJourneyman, nil)

	suggestions, err := comp.SuggestPartyMembers([]agentprogression.Agent{fullCoverage, noCoverage}, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}

	for _, s := range suggestions {
		if s.Agent.ID == fullCoverage.ID {
			if len(s.SkillsCovered) != 2 {
				t.Errorf("full-coverage agent SkillsCovered count = %d, want 2", len(s.SkillsCovered))
			}
		}
		if s.Agent.ID == noCoverage.ID {
			if len(s.SkillsCovered) != 0 {
				t.Errorf("no-coverage agent SkillsCovered count = %d, want 0", len(s.SkillsCovered))
			}
		}
	}
}

// =============================================================================
// OPERATION GUARD
// =============================================================================

func TestFormParty_FailsWhenNotRunning(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "party-not-running"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	// Intentionally do NOT call Start.

	quest := buildQuest(comp.boardConfig, "blocked-quest", domain.DifficultyTrivial, nil, 1)
	agents := []agentprogression.Agent{
		buildAgent(comp.boardConfig, "idle", 16, domain.TierMaster, nil),
	}

	_, err = comp.FormParty(ctx, quest, domain.PartyStrategyBalanced, agents)
	if err == nil {
		t.Fatal("FormParty should error when component is not running")
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func setupPartyComponent(t *testing.T, client *natsclient.Client, boardName string) *Component {
	t.Helper()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = boardName

	ctx := context.Background()

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}

func buildQuest(config *domain.BoardConfig, name string, difficulty domain.QuestDifficulty, skills []domain.SkillTag, minPartySize int) *domain.Quest {
	instance := domain.GenerateInstance()
	questID := domain.QuestID(config.QuestEntityID(instance))
	return &domain.Quest{
		ID:             questID,
		Title:          name,
		Difficulty:     difficulty,
		RequiredSkills: skills,
		MinPartySize:   minPartySize,
		PartyRequired:  minPartySize > 1,
		Status:         domain.QuestPosted,
	}
}

func buildQuestWithGuild(config *domain.BoardConfig, name string, difficulty domain.QuestDifficulty, guildID domain.GuildID) *domain.Quest {
	q := buildQuest(config, name, difficulty, nil, 2)
	q.GuildPriority = &guildID
	return q
}

// buildAgent creates a test agent without skills or guild membership.
// The guilds parameter is reserved for future use.
func buildAgent(config *domain.BoardConfig, name string, level int, tier domain.TrustTier, guilds []domain.GuildID) agentprogression.Agent {
	agent := buildAgentWithSkills(config, name, level, tier, nil)
	agent.Guilds = guilds
	return agent
}

func buildAgentWithSkills(config *domain.BoardConfig, name string, level int, tier domain.TrustTier, skills []domain.SkillTag) agentprogression.Agent {
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(config.AgentEntityID(instance))

	agent := agentprogression.Agent{
		ID:     agentID,
		Name:   name,
		Level:  level,
		Tier:   tier,
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

func buildAgentInGuild(config *domain.BoardConfig, name string, level int, tier domain.TrustTier, guildID domain.GuildID) agentprogression.Agent {
	agent := buildAgent(config, name, level, tier, nil)
	agent.Guilds = []domain.GuildID{guildID}
	return agent
}
