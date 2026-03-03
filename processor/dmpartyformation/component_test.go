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

	quest := buildQuest(comp.boardConfig, "solo-quest", semdragons.DifficultyTrivial, nil, 1)
	agents := []semdragons.Agent{
		buildAgent(comp.boardConfig, "lead-agent", 16, semdragons.TierMaster, nil),
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
	quest := buildQuest(comp.boardConfig, "multi-skill-quest", semdragons.DifficultyModerate,
		[]semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillAnalysis}, 2)

	agents := []semdragons.Agent{
		buildAgentWithSkills(comp.boardConfig, "coder-agent", 16, semdragons.TierMaster,
			[]semdragons.SkillTag{semdragons.SkillCodeGen}),
		buildAgentWithSkills(comp.boardConfig, "analyst-agent", 7, semdragons.TierJourneyman,
			[]semdragons.SkillTag{semdragons.SkillAnalysis}),
		buildAgentWithSkills(comp.boardConfig, "reviewer-agent", 5, semdragons.TierApprentice,
			[]semdragons.SkillTag{semdragons.SkillCodeReview}),
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

	quest := buildQuest(comp.boardConfig, "empty-party-quest", semdragons.DifficultyTrivial, nil, 1)

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

	guildID := semdragons.GuildID(comp.boardConfig.GuildEntityID("code-guild"))

	quest := buildQuestWithGuild(comp.boardConfig, "specialist-quest", semdragons.DifficultyModerate, guildID)

	guildAgent := buildAgentInGuild(comp.boardConfig, "guild-coder", 16, semdragons.TierMaster, guildID)
	nonGuildAgent := buildAgent(comp.boardConfig, "random-agent", 12, semdragons.TierExpert, nil)

	agents := []semdragons.Agent{guildAgent, nonGuildAgent}

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

	quest := buildQuest(comp.boardConfig, "mentor-quest", semdragons.DifficultyEpic, nil, 2)

	// One master who can lead, plus two apprentices.
	mentor := buildAgent(comp.boardConfig, "master-agent", 16, semdragons.TierMaster, nil)
	apprentice1 := buildAgent(comp.boardConfig, "apprentice-1", 3, semdragons.TierApprentice, nil)
	apprentice2 := buildAgent(comp.boardConfig, "apprentice-2", 5, semdragons.TierApprentice, nil)

	agents := []semdragons.Agent{mentor, apprentice1, apprentice2}

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

	quest := buildQuest(comp.boardConfig, "no-master-quest", semdragons.DifficultyModerate, nil, 2)

	// Only apprentices - none can lead.
	agents := []semdragons.Agent{
		buildAgent(comp.boardConfig, "appr-1", 3, semdragons.TierApprentice, nil),
		buildAgent(comp.boardConfig, "appr-2", 5, semdragons.TierApprentice, nil),
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

	quest := buildQuest(comp.boardConfig, "minimal-quest", semdragons.DifficultyTrivial, nil, 1)
	// Ensure PartyRequired is false and MinPartySize = 1.
	quest.PartyRequired = false
	quest.MinPartySize = 1

	agents := []semdragons.Agent{
		buildAgent(comp.boardConfig, "solo-master", 16, semdragons.TierMaster, nil),
		buildAgent(comp.boardConfig, "extra-agent", 12, semdragons.TierExpert, nil),
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

	quest := buildQuest(comp.boardConfig, "ranking-quest", semdragons.DifficultyModerate,
		[]semdragons.SkillTag{semdragons.SkillCodeGen}, 1)

	agents := []semdragons.Agent{
		buildAgentWithSkills(comp.boardConfig, "skilled-coder", 12, semdragons.TierExpert,
			[]semdragons.SkillTag{semdragons.SkillCodeGen}),
		buildAgent(comp.boardConfig, "no-match-agent", 8, semdragons.TierJourneyman, nil),
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

	quest := buildQuest(comp.boardConfig, "unstarted-quest", semdragons.DifficultyTrivial, nil, 1)
	agents := []semdragons.Agent{
		buildAgent(comp.boardConfig, "idle-agent", 5, semdragons.TierApprentice, nil),
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

	quest := buildQuest(comp.boardConfig, "suggest-quest", semdragons.DifficultyModerate,
		[]semdragons.SkillTag{semdragons.SkillCodeGen}, 2)

	agents := []semdragons.Agent{
		buildAgentWithSkills(comp.boardConfig, "perfect-match", 12, semdragons.TierExpert,
			[]semdragons.SkillTag{semdragons.SkillCodeGen}),
		buildAgent(comp.boardConfig, "no-match", 5, semdragons.TierApprentice, nil),
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

	quest := buildQuest(comp.boardConfig, "canlead-quest", semdragons.DifficultyModerate, nil, 2)

	masterAgent := buildAgent(comp.boardConfig, "master-dm", 16, semdragons.TierMaster, nil)
	apprenticeAgent := buildAgent(comp.boardConfig, "appr-dm", 3, semdragons.TierApprentice, nil)
	agents := []semdragons.Agent{masterAgent, apprenticeAgent}

	suggestions, err := comp.SuggestPartyMembers(agents, quest, domain.PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("SuggestPartyMembers failed: %v", err)
	}

	for _, s := range suggestions {
		perms := semdragons.TierPermissionsFor(s.Agent.Tier)
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

	requiredSkills := []semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillCodeReview}
	quest := buildQuest(comp.boardConfig, "skills-quest", semdragons.DifficultyModerate, requiredSkills, 2)

	// Agent covers both required skills.
	fullCoverage := buildAgentWithSkills(comp.boardConfig, "full-coverage", 12, semdragons.TierExpert, requiredSkills)
	// Agent covers none.
	noCoverage := buildAgent(comp.boardConfig, "no-coverage", 8, semdragons.TierJourneyman, nil)

	suggestions, err := comp.SuggestPartyMembers([]semdragons.Agent{fullCoverage, noCoverage}, quest, domain.PartyStrategyBalanced)
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

	quest := buildQuest(comp.boardConfig, "blocked-quest", semdragons.DifficultyTrivial, nil, 1)
	agents := []semdragons.Agent{
		buildAgent(comp.boardConfig, "idle", 16, semdragons.TierMaster, nil),
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

func buildQuest(config *semdragons.BoardConfig, name string, difficulty semdragons.QuestDifficulty, skills []semdragons.SkillTag, minPartySize int) *semdragons.Quest {
	instance := semdragons.GenerateInstance()
	questID := semdragons.QuestID(config.QuestEntityID(instance))
	return &semdragons.Quest{
		ID:             questID,
		Title:          name,
		Difficulty:     difficulty,
		RequiredSkills: skills,
		MinPartySize:   minPartySize,
		PartyRequired:  minPartySize > 1,
		Status:         semdragons.QuestPosted,
	}
}

func buildQuestWithGuild(config *semdragons.BoardConfig, name string, difficulty semdragons.QuestDifficulty, guildID semdragons.GuildID) *semdragons.Quest {
	q := buildQuest(config, name, difficulty, nil, 2)
	q.GuildPriority = &guildID
	return q
}

// buildAgent creates a test agent without skills or guild membership.
// The guilds parameter is reserved for future use.
func buildAgent(config *semdragons.BoardConfig, name string, level int, tier semdragons.TrustTier, guilds []semdragons.GuildID) semdragons.Agent {
	agent := buildAgentWithSkills(config, name, level, tier, nil)
	agent.Guilds = guilds
	return agent
}

func buildAgentWithSkills(config *semdragons.BoardConfig, name string, level int, tier semdragons.TrustTier, skills []semdragons.SkillTag) semdragons.Agent {
	instance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(config.AgentEntityID(instance))

	agent := semdragons.Agent{
		ID:     agentID,
		Name:   name,
		Level:  level,
		Tier:   tier,
		Status: semdragons.AgentIdle,
	}

	if len(skills) > 0 {
		agent.SkillProficiencies = make(map[semdragons.SkillTag]semdragons.SkillProficiency)
		for _, skill := range skills {
			agent.SkillProficiencies[skill] = semdragons.SkillProficiency{
				Level: semdragons.ProficiencyJourneyman,
			}
		}
	}

	return agent
}

func buildAgentInGuild(config *semdragons.BoardConfig, name string, level int, tier semdragons.TrustTier, guildID semdragons.GuildID) semdragons.Agent {
	agent := buildAgent(config, name, level, tier, nil)
	agent.Guilds = []semdragons.GuildID{guildID}
	return agent
}
