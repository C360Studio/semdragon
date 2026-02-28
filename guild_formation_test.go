//go:build integration

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
	if config.MinFounderLevel != 11 {
		t.Errorf("expected MinFounderLevel=11, got %d", config.MinFounderLevel)
	}
	if config.FoundingXPCost != 500 {
		t.Errorf("expected FoundingXPCost=500, got %d", config.FoundingXPCost)
	}
}

func TestGuildFormationConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  GuildFormationConfig
		wantErr bool
	}{
		{
			name:    "valid defaults",
			config:  DefaultFormationConfig(),
			wantErr: false,
		},
		{
			name: "invalid MinClusterSize",
			config: GuildFormationConfig{
				MinClusterSize:      0,
				MinClusterStrength:  0.5,
				MinAgentLevel:       3,
				RequireQualityScore: 0.5,
				MinFounderLevel:     11,
				FoundingXPCost:      500,
				DefaultMaxMembers:   20,
			},
			wantErr: true,
		},
		{
			name: "invalid MinClusterStrength too high",
			config: GuildFormationConfig{
				MinClusterSize:      3,
				MinClusterStrength:  1.5,
				MinAgentLevel:       3,
				RequireQualityScore: 0.5,
				MinFounderLevel:     11,
				FoundingXPCost:      500,
				DefaultMaxMembers:   20,
			},
			wantErr: true,
		},
		{
			name: "invalid MinFounderLevel",
			config: GuildFormationConfig{
				MinClusterSize:      3,
				MinClusterStrength:  0.5,
				MinAgentLevel:       3,
				RequireQualityScore: 0.5,
				MinFounderLevel:     0,
				FoundingXPCost:      500,
				DefaultMaxMembers:   20,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDetectSkillClusters_WeakSimilarity(t *testing.T) {
	config := DefaultFormationConfig()
	config.MinClusterStrength = 0.7 // High threshold
	engine := NewGuildFormationEngine(nil, nil, config)

	// Create agents with same primary skill but very different secondary skills
	agents := []*Agent{
		{
			ID:     "agent-1",
			Level:  5,
			Skills: []SkillTag{SkillCodeGen, SkillResearch, SkillPlanning},
			Stats:  AgentStats{AvgQualityScore: 0.8},
		},
		{
			ID:     "agent-2",
			Level:  6,
			Skills: []SkillTag{SkillCodeGen, SkillDataTransform, SkillAnalysis},
			Stats:  AgentStats{AvgQualityScore: 0.7},
		},
		{
			ID:     "agent-3",
			Level:  4,
			Skills: []SkillTag{SkillCodeGen, SkillCustomerComms, SkillSummarization},
			Stats:  AgentStats{AvgQualityScore: 0.9},
		},
	}

	suggestions := engine.DetectSkillClusters(context.Background(), agents)

	// Should not form a cluster because Jaccard similarity is too low
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions due to weak similarity, got %d with strength %f",
			len(suggestions), suggestions[0].ClusterStrength)
	}
}

func TestDetectSkillClusters_ContextCancellation(t *testing.T) {
	config := DefaultFormationConfig()
	engine := NewGuildFormationEngine(nil, nil, config)

	// Create enough agents to trigger cancellation check
	agents := make([]*Agent, 100)
	for i := range agents {
		agents[i] = &Agent{
			ID:     AgentID(string(rune('a' + i%26))),
			Level:  5,
			Skills: []SkillTag{SkillCodeGen, SkillCodeReview},
			Stats:  AgentStats{AvgQualityScore: 0.8},
		}
	}

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return early due to cancellation
	suggestions := engine.DetectSkillClusters(ctx, agents)

	// May return partial results or empty, but should not panic
	_ = suggestions
}

// =============================================================================
// SOCIAL FORMATION TESTS
// =============================================================================

func TestFoundGuild_Success(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boardConfig := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "foundguild",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, boardConfig)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create an Expert-level agent with enough XP
	founderInstance := "founder"
	founder := &Agent{
		ID:        AgentID(founderInstance),
		Name:      "Expert Founder",
		Level:     12, // Expert tier
		XP:        1000,
		Status:    AgentIdle,
		Guilds:    []GuildID{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := storage.PutAgent(context.Background(), founderInstance, founder); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Found guild
	guild, err := engine.FoundGuild(context.Background(), AgentID(founderInstance), "Code Crafters", "We ship quality code")
	if err != nil {
		t.Fatalf("FoundGuild failed: %v", err)
	}

	// Verify guild properties
	if guild.Name != "Code Crafters" {
		t.Errorf("expected name 'Code Crafters', got '%s'", guild.Name)
	}
	if guild.Culture != "We ship quality code" {
		t.Errorf("expected culture 'We ship quality code', got '%s'", guild.Culture)
	}
	if guild.FoundedBy != AgentID(founderInstance) {
		t.Errorf("expected founder %s, got %s", founderInstance, guild.FoundedBy)
	}
	if len(guild.Members) != 1 {
		t.Errorf("expected 1 member (founder), got %d", len(guild.Members))
	}
	if guild.Members[0].Rank != GuildRankMaster {
		t.Errorf("expected founder rank Guildmaster, got %s", guild.Members[0].Rank)
	}

	// Verify XP was deducted
	updatedFounder, err := storage.GetAgent(context.Background(), founderInstance)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	expectedXP := int64(1000 - formConfig.FoundingXPCost)
	if updatedFounder.XP != expectedXP {
		t.Errorf("expected XP %d, got %d", expectedXP, updatedFounder.XP)
	}

	// Verify guild was added to agent
	if len(updatedFounder.Guilds) != 1 {
		t.Errorf("expected 1 guild, got %d", len(updatedFounder.Guilds))
	}
}

func TestFoundGuild_LevelTooLow(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boardConfig := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "foundguild2",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, boardConfig)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create a Journeyman agent (below Expert tier)
	agentInstance := "junior"
	agent := &Agent{
		ID:        AgentID(agentInstance),
		Name:      "Junior Dev",
		Level:     5, // Apprentice tier
		XP:        1000,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := storage.PutAgent(context.Background(), agentInstance, agent); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Should fail due to level requirement
	_, err = engine.FoundGuild(context.Background(), AgentID(agentInstance), "Fail Guild", "")
	if err == nil {
		t.Fatal("expected error for under-leveled founder")
	}
}

func TestFoundGuild_InsufficientXP(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boardConfig := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "foundguild3",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, boardConfig)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create an Expert agent with insufficient XP
	agentInstance := "poorexpert"
	agent := &Agent{
		ID:        AgentID(agentInstance),
		Name:      "Poor Expert",
		Level:     12,
		XP:        100, // Less than 500 required
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := storage.PutAgent(context.Background(), agentInstance, agent); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Should fail due to insufficient XP
	_, err = engine.FoundGuild(context.Background(), AgentID(agentInstance), "Fail Guild", "")
	if err == nil {
		t.Fatal("expected error for insufficient XP")
	}
}

func TestInviteToGuild(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boardConfig := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "invite",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, boardConfig)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create guild with an officer
	now := time.Now()
	guildInstance := "testguild"
	guild := &Guild{
		ID:         GuildID(guildInstance),
		Name:       "Test Guild",
		Status:     GuildActive,
		Founded:    now,
		MinLevel:   1,
		MaxMembers: 10,
		Members: []GuildMember{
			{AgentID: "officer", Rank: GuildRankOfficer, JoinedAt: now},
		},
		CreatedAt: now,
	}
	if err := storage.PutGuild(context.Background(), guildInstance, guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}

	// Create officer agent
	officerInstance := "officer"
	officer := &Agent{
		ID:        "officer",
		Name:      "Officer",
		Level:     10,
		Guilds:    []GuildID{GuildID(guildInstance)},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := storage.PutAgent(context.Background(), officerInstance, officer); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Create invitee
	inviteeInstance := "invitee"
	invitee := &Agent{
		ID:        "invitee",
		Name:      "New Member",
		Level:     5,
		Guilds:    []GuildID{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := storage.PutAgent(context.Background(), inviteeInstance, invitee); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Invite
	err = engine.InviteToGuild(context.Background(), "officer", GuildID(guildInstance), "invitee")
	if err != nil {
		t.Fatalf("InviteToGuild failed: %v", err)
	}

	// Verify invitee was added
	updatedGuild, err := storage.GetGuild(context.Background(), guildInstance)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if len(updatedGuild.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(updatedGuild.Members))
	}

	// Verify guild was added to invitee
	updatedInvitee, err := storage.GetAgent(context.Background(), inviteeInstance)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if len(updatedInvitee.Guilds) != 1 {
		t.Errorf("expected 1 guild, got %d", len(updatedInvitee.Guilds))
	}
}

func TestLeaveGuild(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boardConfig := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "leave",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, boardConfig)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create guild with multiple members
	now := time.Now()
	guildInstance := "leaveguild"
	guild := &Guild{
		ID:         GuildID(guildInstance),
		Name:       "Leave Test Guild",
		Status:     GuildActive,
		Founded:    now,
		MinLevel:   1,
		MaxMembers: 10,
		Members: []GuildMember{
			{AgentID: "master", Rank: GuildRankMaster, JoinedAt: now},
			{AgentID: "member", Rank: GuildRankMember, JoinedAt: now},
		},
		CreatedAt: now,
	}
	if err := storage.PutGuild(context.Background(), guildInstance, guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}

	// Create member agent
	memberInstance := "member"
	member := &Agent{
		ID:        "member",
		Name:      "Member",
		Level:     5,
		Guilds:    []GuildID{GuildID(guildInstance)},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := storage.PutAgent(context.Background(), memberInstance, member); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Leave
	err = engine.LeaveGuild(context.Background(), "member", GuildID(guildInstance))
	if err != nil {
		t.Fatalf("LeaveGuild failed: %v", err)
	}

	// Verify member was removed
	updatedGuild, err := storage.GetGuild(context.Background(), guildInstance)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if len(updatedGuild.Members) != 1 {
		t.Errorf("expected 1 member, got %d", len(updatedGuild.Members))
	}

	// Verify guild was removed from agent
	updatedMember, err := storage.GetAgent(context.Background(), memberInstance)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if len(updatedMember.Guilds) != 0 {
		t.Errorf("expected 0 guilds, got %d", len(updatedMember.Guilds))
	}
}

func TestLeaveGuild_OnlyGuildmaster(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boardConfig := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "leavemaster",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, boardConfig)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create guild with only one Guildmaster
	now := time.Now()
	guildInstance := "masterguild"
	guild := &Guild{
		ID:         GuildID(guildInstance),
		Name:       "Master Only Guild",
		Status:     GuildActive,
		Founded:    now,
		MinLevel:   1,
		MaxMembers: 10,
		Members: []GuildMember{
			{AgentID: "master", Rank: GuildRankMaster, JoinedAt: now},
		},
		CreatedAt: now,
	}
	if err := storage.PutGuild(context.Background(), guildInstance, guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}

	// Create master agent
	masterInstance := "master"
	master := &Agent{
		ID:        "master",
		Name:      "Guildmaster",
		Level:     15,
		Guilds:    []GuildID{GuildID(guildInstance)},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := storage.PutAgent(context.Background(), masterInstance, master); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Should fail - can't leave as only Guildmaster
	err = engine.LeaveGuild(context.Background(), "master", GuildID(guildInstance))
	if err == nil {
		t.Fatal("expected error when only Guildmaster tries to leave")
	}
}

func TestPromoteMember(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boardConfig := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "promote",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, boardConfig)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create guild
	now := time.Now()
	guildInstance := "promoteguild"
	guild := &Guild{
		ID:         GuildID(guildInstance),
		Name:       "Promote Test Guild",
		Status:     GuildActive,
		Founded:    now,
		MinLevel:   1,
		MaxMembers: 10,
		Members: []GuildMember{
			{AgentID: "master", Rank: GuildRankMaster, JoinedAt: now},
			{AgentID: "initiate", Rank: GuildRankInitiate, JoinedAt: now},
		},
		CreatedAt: now,
	}
	if err := storage.PutGuild(context.Background(), guildInstance, guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}

	// Promote initiate to member
	err = engine.PromoteMember(context.Background(), "master", GuildID(guildInstance), "initiate", GuildRankMember)
	if err != nil {
		t.Fatalf("PromoteMember failed: %v", err)
	}

	// Verify promotion
	updatedGuild, err := storage.GetGuild(context.Background(), guildInstance)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}

	var initiateRank GuildRank
	for _, m := range updatedGuild.Members {
		if m.AgentID == "initiate" {
			initiateRank = m.Rank
			break
		}
	}
	if initiateRank != GuildRankMember {
		t.Errorf("expected rank Member, got %s", initiateRank)
	}
}

func TestEvaluateGuildDiversity(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boardConfig := &BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "diversity",
	}

	storage, err := CreateStorage(context.Background(), tc.Client, boardConfig)
	if err != nil {
		t.Fatalf("CreateStorage failed: %v", err)
	}

	formConfig := DefaultFormationConfig()
	engine := NewGuildFormationEngine(storage, nil, formConfig)

	// Create guild
	now := time.Now()
	guildInstance := "divguild"
	guild := &Guild{
		ID:         GuildID(guildInstance),
		Name:       "Diversity Test Guild",
		Status:     GuildActive,
		Founded:    now,
		MinLevel:   1,
		MaxMembers: 10,
		Members: []GuildMember{
			{AgentID: "coder", Rank: GuildRankMember, JoinedAt: now},
			{AgentID: "tester", Rank: GuildRankMember, JoinedAt: now},
		},
		CreatedAt: now,
	}
	if err := storage.PutGuild(context.Background(), guildInstance, guild); err != nil {
		t.Fatalf("PutGuild failed: %v", err)
	}

	// Create diverse agents
	coder := &Agent{
		ID:                 "coder",
		Name:               "Coder",
		Level:              5,
		Skills:             []SkillTag{SkillCodeGen, SkillCodeReview},
		SkillProficiencies: map[SkillTag]SkillProficiency{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	coder.AddSkill(SkillCodeGen)
	coder.AddSkill(SkillCodeReview)
	if err := storage.PutAgent(context.Background(), "coder", coder); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	tester := &Agent{
		ID:                 "tester",
		Name:               "Tester",
		Level:              5,
		Skills:             []SkillTag{SkillResearch, SkillAnalysis},
		SkillProficiencies: map[SkillTag]SkillProficiency{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	tester.AddSkill(SkillResearch)
	tester.AddSkill(SkillAnalysis)
	if err := storage.PutAgent(context.Background(), "tester", tester); err != nil {
		t.Fatalf("PutAgent failed: %v", err)
	}

	// Evaluate diversity
	report, err := engine.EvaluateGuildDiversity(context.Background(), GuildID(guildInstance))
	if err != nil {
		t.Fatalf("EvaluateGuildDiversity failed: %v", err)
	}

	if report.TotalMembers != 2 {
		t.Errorf("expected 2 members, got %d", report.TotalMembers)
	}
	if len(report.UniqueSkills) != 4 {
		t.Errorf("expected 4 unique skills, got %d", len(report.UniqueSkills))
	}
}
