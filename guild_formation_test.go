package semdragons

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []SkillTag
		b        []SkillTag
		expected float64
	}{
		{
			name:     "identical sets",
			a:        []SkillTag{SkillCodeGen, SkillCodeReview},
			b:        []SkillTag{SkillCodeGen, SkillCodeReview},
			expected: 1.0,
		},
		{
			name:     "disjoint sets",
			a:        []SkillTag{SkillCodeGen},
			b:        []SkillTag{SkillResearch},
			expected: 0.0,
		},
		{
			name:     "partial overlap",
			a:        []SkillTag{SkillCodeGen, SkillCodeReview, SkillResearch},
			b:        []SkillTag{SkillCodeGen, SkillCodeReview, SkillAnalysis},
			expected: 0.5, // 2 common / 4 total
		},
		{
			name:     "empty sets",
			a:        []SkillTag{},
			b:        []SkillTag{},
			expected: 1.0,
		},
		{
			name:     "one empty set",
			a:        []SkillTag{SkillCodeGen},
			b:        []SkillTag{},
			expected: 0.0,
		},
		{
			name:     "subset relationship",
			a:        []SkillTag{SkillCodeGen},
			b:        []SkillTag{SkillCodeGen, SkillCodeReview},
			expected: 0.5, // 1 common / 2 total
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JaccardSimilarity(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("JaccardSimilarity(%v, %v) = %f, want %f",
					tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestDetectSkillClusters(t *testing.T) {
	config := DefaultFormationConfig()
	engine := NewGuildFormationEngine(nil, nil, config)

	// Create agents with overlapping skills
	agents := []*Agent{
		{
			ID:     "agent-1",
			Level:  5,
			Skills: []SkillTag{SkillCodeGen, SkillCodeReview, SkillResearch},
			Stats:  AgentStats{AvgQualityScore: 0.8},
		},
		{
			ID:     "agent-2",
			Level:  6,
			Skills: []SkillTag{SkillCodeGen, SkillCodeReview, SkillAnalysis},
			Stats:  AgentStats{AvgQualityScore: 0.7},
		},
		{
			ID:     "agent-3",
			Level:  4,
			Skills: []SkillTag{SkillCodeGen, SkillCodeReview},
			Stats:  AgentStats{AvgQualityScore: 0.9},
		},
		{
			ID:     "agent-4",
			Level:  7,
			Skills: []SkillTag{SkillDataTransform, SkillAnalysis},
			Stats:  AgentStats{AvgQualityScore: 0.6},
		},
	}

	suggestions := engine.DetectSkillClusters(context.Background(), agents)

	// Should detect a cluster for code_generation (3 agents share it)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}

	// Find the code_generation cluster
	var codeGenCluster *GuildSuggestion
	for i := range suggestions {
		if suggestions[i].PrimarySkill == SkillCodeGen {
			codeGenCluster = &suggestions[i]
			break
		}
	}

	if codeGenCluster == nil {
		t.Fatal("expected code_generation cluster")
	}

	if len(codeGenCluster.AgentIDs) != 3 {
		t.Errorf("expected 3 agents in cluster, got %d", len(codeGenCluster.AgentIDs))
	}

	if codeGenCluster.ClusterStrength < 0.5 {
		t.Errorf("expected cluster strength >= 0.5, got %f", codeGenCluster.ClusterStrength)
	}

	// code_review should be a secondary skill
	hasCodeReview := false
	for _, skill := range codeGenCluster.SecondarySkills {
		if skill == SkillCodeReview {
			hasCodeReview = true
			break
		}
	}
	if !hasCodeReview {
		t.Error("expected code_review as secondary skill")
	}
}

func TestDetectSkillClusters_NotEnoughAgents(t *testing.T) {
	config := DefaultFormationConfig()
	config.MinClusterSize = 3
	engine := NewGuildFormationEngine(nil, nil, config)

	// Only 2 agents
	agents := []*Agent{
		{ID: "agent-1", Level: 5, Skills: []SkillTag{SkillCodeGen}, Stats: AgentStats{AvgQualityScore: 0.8}},
		{ID: "agent-2", Level: 6, Skills: []SkillTag{SkillCodeGen}, Stats: AgentStats{AvgQualityScore: 0.7}},
	}

	suggestions := engine.DetectSkillClusters(context.Background(), agents)

	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions with only 2 agents, got %d", len(suggestions))
	}
}

func TestDetectSkillClusters_LowQualityFiltered(t *testing.T) {
	config := DefaultFormationConfig()
	config.RequireQualityScore = 0.5
	engine := NewGuildFormationEngine(nil, nil, config)

	// Agents with low quality scores
	agents := []*Agent{
		{ID: "agent-1", Level: 5, Skills: []SkillTag{SkillCodeGen}, Stats: AgentStats{AvgQualityScore: 0.3}},
		{ID: "agent-2", Level: 6, Skills: []SkillTag{SkillCodeGen}, Stats: AgentStats{AvgQualityScore: 0.2}},
		{ID: "agent-3", Level: 7, Skills: []SkillTag{SkillCodeGen}, Stats: AgentStats{AvgQualityScore: 0.4}},
	}

	suggestions := engine.DetectSkillClusters(context.Background(), agents)

	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions for low-quality agents, got %d", len(suggestions))
	}
}

func TestEvaluateAgentForGuilds(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	config := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "evaltest",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, config)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create a guild
	guild := &Guild{
		ID:             "guild-coders",
		Name:           "Code Generation Guild",
		Skills:         []SkillTag{SkillCodeGen, SkillCodeReview},
		MinLevelToJoin: 3,
		AutoRecruit:    true,
		Status:         GuildActive,
		CreatedAt:      time.Now(),
	}
	if err := storage.PutGuild(context.Background(), "coders", guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}

	// Index guild by skill
	if err := storage.AddGuildSkillIndex(context.Background(), SkillCodeGen, "coders"); err != nil {
		t.Fatalf("AddGuildSkillIndex failed: %v", err)
	}

	// Create a qualifying agent
	agent := &Agent{
		ID:     "agent-coder",
		Level:  5,
		Skills: []SkillTag{SkillCodeGen, SkillResearch},
		Stats:  AgentStats{AvgQualityScore: 0.8},
	}

	matches, err := engine.EvaluateAgentForGuilds(context.Background(), agent)
	if err != nil {
		t.Fatalf("EvaluateAgentForGuilds failed: %v", err)
	}

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	match := matches[0]
	if match.GuildID != "guild-coders" {
		t.Errorf("expected guild-coders, got %s", match.GuildID)
	}
	if !match.CanAutoJoin {
		t.Error("expected CanAutoJoin to be true")
	}
	if len(match.SkillsMatched) != 1 {
		t.Errorf("expected 1 matched skill, got %d", len(match.SkillsMatched))
	}
}

func TestEvaluateAgentForGuilds_LevelTooLow(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	config := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "leveltest",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, config)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create guild requiring level 10
	guild := &Guild{
		ID:             "guild-experts",
		Name:           "Expert Coders",
		Skills:         []SkillTag{SkillCodeGen},
		MinLevelToJoin: 10,
		AutoRecruit:    true,
		Status:         GuildActive,
		CreatedAt:      time.Now(),
	}
	if err := storage.PutGuild(context.Background(), "experts", guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}
	if err := storage.AddGuildSkillIndex(context.Background(), SkillCodeGen, "experts"); err != nil {
		t.Fatalf("AddGuildSkillIndex failed: %v", err)
	}

	// Agent at level 5 (too low)
	agent := &Agent{
		ID:     "agent-junior",
		Level:  5,
		Skills: []SkillTag{SkillCodeGen},
		Stats:  AgentStats{AvgQualityScore: 0.8},
	}

	matches, err := engine.EvaluateAgentForGuilds(context.Background(), agent)
	if err != nil {
		t.Fatalf("EvaluateAgentForGuilds failed: %v", err)
	}

	if len(matches) != 0 {
		t.Errorf("expected no matches for under-leveled agent, got %d", len(matches))
	}
}

func TestProcessAutoRecruit(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	config := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "autorecruit",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, config)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	events := NewEventPublisher(tc.Client)
	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, events, formConfig)

	// Create an auto-recruit guild - use consistent instance/ID
	autoGuildInstance := "auto"
	guild := &Guild{
		ID:             GuildID(autoGuildInstance),
		Name:           "Auto-Recruit Guild",
		Skills:         []SkillTag{SkillCodeGen},
		MinLevelToJoin: 3,
		AutoRecruit:    true,
		Status:         GuildActive,
		Members:        []GuildMember{},
		CreatedAt:      time.Now(),
	}
	if err := storage.PutGuild(context.Background(), autoGuildInstance, guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}
	if err := storage.AddGuildSkillIndex(context.Background(), SkillCodeGen, autoGuildInstance); err != nil {
		t.Fatalf("AddGuildSkillIndex failed: %v", err)
	}

	// Create a non-auto-recruit guild
	manualGuildInstance := "manual"
	guildManual := &Guild{
		ID:             GuildID(manualGuildInstance),
		Name:           "Manual-Only Guild",
		Skills:         []SkillTag{SkillCodeGen},
		MinLevelToJoin: 3,
		AutoRecruit:    false,
		Status:         GuildActive,
		Members:        []GuildMember{},
		CreatedAt:      time.Now(),
	}
	if err := storage.PutGuild(context.Background(), manualGuildInstance, guildManual); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}
	if err := storage.AddGuildSkillIndex(context.Background(), SkillCodeGen, manualGuildInstance); err != nil {
		t.Fatalf("AddGuildSkillIndex failed: %v", err)
	}

	// Create and store agent - use same instance for ID and storage key
	agentInstance := "recruit"
	agent := &Agent{
		ID:        AgentID(agentInstance),
		Level:     5,
		Skills:    []SkillTag{SkillCodeGen},
		Stats:     AgentStats{AvgQualityScore: 0.8},
		Guilds:    []GuildID{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := storage.PutAgent(context.Background(), agentInstance, agent); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Process auto-recruit
	joined, err := engine.ProcessAutoRecruit(context.Background(), AgentID(agentInstance))
	if err != nil {
		t.Fatalf("ProcessAutoRecruit failed: %v", err)
	}

	// Should only join the auto-recruit guild
	if len(joined) != 1 {
		t.Errorf("expected 1 guild joined, got %d", len(joined))
	}
	if len(joined) > 0 && joined[0] != GuildID(autoGuildInstance) {
		t.Errorf("expected %s, got %s", autoGuildInstance, joined[0])
	}

	// Verify guild membership updated
	updatedGuild, err := storage.GetGuild(context.Background(), autoGuildInstance)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if len(updatedGuild.Members) != 1 {
		t.Errorf("expected 1 guild member, got %d", len(updatedGuild.Members))
	}
	if len(updatedGuild.Members) > 0 && updatedGuild.Members[0].AgentID != AgentID(agentInstance) {
		t.Errorf("expected %s, got %s", agentInstance, updatedGuild.Members[0].AgentID)
	}

	// Verify agent guilds updated
	updatedAgent, err := storage.GetAgent(context.Background(), "recruit")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if len(updatedAgent.Guilds) != 1 {
		t.Errorf("expected 1 agent guild, got %d", len(updatedAgent.Guilds))
	}
}

func TestProcessAutoRecruit_AlreadyMember(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	config := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "alreadymember",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, config)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create guild with agent already a member
	guild := &Guild{
		ID:             "guild-existing",
		Name:           "Existing Guild",
		Skills:         []SkillTag{SkillCodeGen},
		MinLevelToJoin: 3,
		AutoRecruit:    true,
		Status:         GuildActive,
		Members: []GuildMember{
			{AgentID: "existing", Rank: GuildRankMember, JoinedAt: time.Now()},
		},
		CreatedAt: time.Now(),
	}
	if err := storage.PutGuild(context.Background(), "existing", guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}
	if err := storage.AddGuildSkillIndex(context.Background(), SkillCodeGen, "existing"); err != nil {
		t.Fatalf("AddGuildSkillIndex failed: %v", err)
	}

	// Store agent - use same instance for ID and storage key
	agentInstance := "existing"
	agent := &Agent{
		ID:        AgentID(agentInstance),
		Level:     5,
		Skills:    []SkillTag{SkillCodeGen},
		Stats:     AgentStats{AvgQualityScore: 0.8},
		Guilds:    []GuildID{"guild-existing"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := storage.PutAgent(context.Background(), agentInstance, agent); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Process should not re-join
	joined, err := engine.ProcessAutoRecruit(context.Background(), AgentID(agentInstance))
	if err != nil {
		t.Fatalf("ProcessAutoRecruit failed: %v", err)
	}

	if len(joined) != 0 {
		t.Errorf("expected no guilds joined (already member), got %d", len(joined))
	}
}

func TestGuildFormationConfig_Defaults(t *testing.T) {
	config := DefaultFormationConfig()

	if config.MinClusterSize != 3 {
		t.Errorf("expected MinClusterSize=3, got %d", config.MinClusterSize)
	}
	if config.MinClusterStrength != 0.6 {
		t.Errorf("expected MinClusterStrength=0.6, got %f", config.MinClusterStrength)
	}
	if config.MinAgentLevel != 3 {
		t.Errorf("expected MinAgentLevel=3, got %d", config.MinAgentLevel)
	}
	if config.RequireQualityScore != 0.5 {
		t.Errorf("expected RequireQualityScore=0.5, got %f", config.RequireQualityScore)
	}
}
