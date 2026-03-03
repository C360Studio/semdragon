//go:build integration

package guildformation

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
// INTEGRATION TESTS - GuildFormation Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/guildformation/...
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
	config.Board = "lifecycle"
	config.EnableAutoFormation = false // Disable auto-formation so watcher is not started

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	// Test Initialize
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists (mirrors main.go startup)
	gc := semdragons.NewGraphClient(client, comp.BoardConfig())
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Test Meta
	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}
	if meta.Version == "" {
		t.Error("Meta.Version should not be empty")
	}

	// Test Start
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify running
	health := comp.Health()
	if !health.Healthy {
		t.Error("Component should be healthy after start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	// Test double-start is rejected
	err = comp.Start(ctx)
	if err == nil {
		t.Error("Second Start should return error")
	}

	// Test Stop
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stopped
	health = comp.Health()
	if health.Healthy {
		t.Error("Component should not be healthy after stop")
	}
	if health.Status != "stopped" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "stopped")
	}

	// Verify idempotent stop
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Errorf("Second Stop should not error: %v", err)
	}
}

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Error("Should have at least one input port defined")
	}

	// Verify the agent-state-watch port is present
	hasAgentWatch := false
	for _, port := range inputs {
		if port.Name == "agent-state-watch" {
			hasAgentWatch = true
			if port.Direction != component.DirectionInput {
				t.Errorf("agent-state-watch port direction = %v, want input", port.Direction)
			}
		}
	}
	if !hasAgentWatch {
		t.Error("Missing agent-state-watch input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Error("Should have at least one output port defined")
	}

	// Verify guild-events output port is present and required
	hasGuildEvents := false
	for _, port := range outputs {
		if port.Name == "guild-events" {
			hasGuildEvents = true
			if !port.Required {
				t.Error("guild-events port should be required")
			}
			if port.Direction != component.DirectionOutput {
				t.Errorf("guild-events port direction = %v, want output", port.Direction)
			}
		}
	}
	if !hasGuildEvents {
		t.Error("Missing guild-events output port")
	}
}

func TestComponent_ConfigSchema(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	// Check required fields
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
			t.Errorf("Field %q should be in Required list", field)
		}
	}

	// Check expected properties are declared
	expectedProps := []string{"org", "platform", "board", "min_members_for_formation", "max_guild_size", "enable_auto_formation"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("Missing property %q in ConfigSchema", prop)
		}
	}
}

func TestComponent_CreateGuild(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "createguild")
	defer comp.Stop(5 * time.Second)

	founderID := makeAgentID(t, comp.BoardConfig(), "founder")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "Iron Circle",
		Culture:   "Discipline and excellence",
		Motto:     "We sharpen each other",
		FounderID: founderID,
		MinLevel:  3,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	// Verify returned guild
	if guild.ID == "" {
		t.Error("Guild ID should be set")
	}
	if guild.Name != "Iron Circle" {
		t.Errorf("Name = %q, want %q", guild.Name, "Iron Circle")
	}
	if guild.Culture != "Discipline and excellence" {
		t.Errorf("Culture = %q, want %q", guild.Culture, "Discipline and excellence")
	}
	if guild.Status != domain.GuildActive {
		t.Errorf("Status = %v, want %v", guild.Status, domain.GuildActive)
	}
	if guild.FoundedBy != domain.AgentID(founderID) {
		t.Errorf("FoundedBy = %v, want %v", guild.FoundedBy, founderID)
	}
	if guild.Founded.IsZero() {
		t.Error("Founded timestamp should be set")
	}

	// Founder should be an automatic member with Master rank
	if len(guild.Members) != 1 {
		t.Fatalf("Members count = %d, want 1 (founder)", len(guild.Members))
	}
	if guild.Members[0].AgentID != domain.AgentID(founderID) {
		t.Errorf("Founder should be first member")
	}
	if guild.Members[0].Rank != domain.GuildRankMaster {
		t.Errorf("Founder rank = %v, want %v", guild.Members[0].Rank, domain.GuildRankMaster)
	}

	// Guild should be retrievable from in-memory state
	retrieved, ok := comp.GetGuild(domain.GuildID(guild.ID))
	if !ok {
		t.Fatal("GetGuild: guild not found after creation")
	}
	if retrieved.Name != "Iron Circle" {
		t.Errorf("Retrieved Name = %q, want %q", retrieved.Name, "Iron Circle")
	}

	// Agent-to-guild mapping should be updated
	agentGuilds := comp.GetAgentGuilds(founderID)
	if len(agentGuilds) != 1 {
		t.Errorf("Agent guild count = %d, want 1", len(agentGuilds))
	}

	// Metrics should be updated
	flow := comp.DataFlow()
	if flow.LastActivity.IsZero() {
		t.Error("LastActivity should be set after CreateGuild")
	}
}

func TestComponent_CreateGuildWhenNotRunning(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "notrunning"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	// Deliberately do NOT start the component

	founderID := domain.AgentID("test.platform.game.board.agent.founder1")
	_, err = comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "Ghost Guild",
		FounderID: founderID,
	})
	if err == nil {
		t.Error("CreateGuild should fail when component is not running")
	}
}

func TestComponent_JoinGuild(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "joinguild")
	defer comp.Stop(5 * time.Second)

	founderID := makeAgentID(t, comp.BoardConfig(), "guild-founder")
	memberID := makeAgentID(t, comp.BoardConfig(), "new-member")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "The Seekers",
		Culture:   "Truth above all",
		FounderID: founderID,
		MinLevel:  1,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	guildID := domain.GuildID(guild.ID)

	if err := comp.JoinGuild(ctx, guildID, memberID); err != nil {
		t.Fatalf("JoinGuild failed: %v", err)
	}

	// Verify membership
	updated, ok := comp.GetGuild(guildID)
	if !ok {
		t.Fatal("GetGuild: guild not found")
	}
	if len(updated.Members) != 2 {
		t.Fatalf("Members count = %d, want 2 (founder + new member)", len(updated.Members))
	}

	// New member should have Initiate rank
	var newMember *domain.GuildMember
	for i := range updated.Members {
		if updated.Members[i].AgentID == domain.AgentID(memberID) {
			newMember = &updated.Members[i]
			break
		}
	}
	if newMember == nil {
		t.Fatal("New member not found in guild members list")
	}
	if newMember.Rank != domain.GuildRankInitiate {
		t.Errorf("New member rank = %v, want %v", newMember.Rank, domain.GuildRankInitiate)
	}
	if newMember.JoinedAt.IsZero() {
		t.Error("JoinedAt should be set")
	}

	// Agent-to-guild mapping should include new member
	memberGuilds := comp.GetAgentGuilds(memberID)
	if len(memberGuilds) != 1 {
		t.Errorf("Member guild count = %d, want 1", len(memberGuilds))
	}
	if memberGuilds[0] != guildID {
		t.Errorf("Member guild = %v, want %v", memberGuilds[0], guildID)
	}
}

func TestComponent_JoinGuildAlreadyMember(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "joinduplicate")
	defer comp.Stop(5 * time.Second)

	founderID := makeAgentID(t, comp.BoardConfig(), "dupe-founder")
	memberID := makeAgentID(t, comp.BoardConfig(), "dupe-member")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "Lone Star",
		FounderID: founderID,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	guildID := domain.GuildID(guild.ID)

	if err := comp.JoinGuild(ctx, guildID, memberID); err != nil {
		t.Fatalf("First JoinGuild failed: %v", err)
	}

	// Joining again should fail with ErrAlreadyMember
	err = comp.JoinGuild(ctx, guildID, memberID)
	if err == nil {
		t.Fatal("Second JoinGuild should return an error")
	}
	if err != ErrAlreadyMember {
		t.Errorf("Expected ErrAlreadyMember, got: %v", err)
	}
}

func TestComponent_JoinGuildFull(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "guildfull")
	defer comp.Stop(5 * time.Second)

	// Create component with a small max guild size
	comp.config.MaxGuildSize = 2

	founderID := makeAgentID(t, comp.BoardConfig(), "full-founder")
	member1ID := makeAgentID(t, comp.BoardConfig(), "full-member1")
	member2ID := makeAgentID(t, comp.BoardConfig(), "full-member2")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "Tiny Guild",
		FounderID: founderID,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}
	guildID := domain.GuildID(guild.ID)

	// Add up to max (founder already counts as 1, max is 2)
	if err := comp.JoinGuild(ctx, guildID, member1ID); err != nil {
		t.Fatalf("JoinGuild member1 failed: %v", err)
	}

	// This should fail with ErrGuildFull
	err = comp.JoinGuild(ctx, guildID, member2ID)
	if err == nil {
		t.Fatal("JoinGuild should fail when guild is at capacity")
	}
	if err != ErrGuildFull {
		t.Errorf("Expected ErrGuildFull, got: %v", err)
	}
}

func TestComponent_JoinGuildNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "guildnotfound")
	defer comp.Stop(5 * time.Second)

	nonExistentID := domain.GuildID("test.integration.game.guildnotfound.guild.ghost")
	memberID := makeAgentID(t, comp.BoardConfig(), "lonely-agent")

	err := comp.JoinGuild(ctx, nonExistentID, memberID)
	if err == nil {
		t.Error("JoinGuild should fail for non-existent guild")
	}
}

func TestComponent_LeaveGuild(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "leaveguild")
	defer comp.Stop(5 * time.Second)

	founderID := makeAgentID(t, comp.BoardConfig(), "leave-founder")
	memberID := makeAgentID(t, comp.BoardConfig(), "leave-member")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "Wayfarers",
		FounderID: founderID,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}
	guildID := domain.GuildID(guild.ID)

	if err := comp.JoinGuild(ctx, guildID, memberID); err != nil {
		t.Fatalf("JoinGuild failed: %v", err)
	}

	if err := comp.LeaveGuild(ctx, guildID, memberID, "pursuing other opportunities"); err != nil {
		t.Fatalf("LeaveGuild failed: %v", err)
	}

	// Verify member was removed
	updated, ok := comp.GetGuild(guildID)
	if !ok {
		t.Fatal("GetGuild: guild not found after leave")
	}
	if len(updated.Members) != 1 {
		t.Errorf("Members count = %d, want 1 (only founder)", len(updated.Members))
	}
	for _, m := range updated.Members {
		if m.AgentID == domain.AgentID(memberID) {
			t.Error("Departed member should not be in Members list")
		}
	}

	// Agent-to-guild mapping should be cleared
	memberGuilds := comp.GetAgentGuilds(memberID)
	if len(memberGuilds) != 0 {
		t.Errorf("Member guild count = %d, want 0 after leaving", len(memberGuilds))
	}
}

func TestComponent_LeaveGuildFounderRejected(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "founderleave")
	defer comp.Stop(5 * time.Second)

	founderID := makeAgentID(t, comp.BoardConfig(), "founder-leave")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "Masters Hall",
		FounderID: founderID,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	// Founder (guildmaster) cannot leave without transferring
	err = comp.LeaveGuild(ctx, domain.GuildID(guild.ID), founderID, "leaving own guild")
	if err == nil {
		t.Error("Founder should not be able to leave without transferring leadership")
	}
}

func TestComponent_PromoteMember(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "promotemember")
	defer comp.Stop(5 * time.Second)

	founderID := makeAgentID(t, comp.BoardConfig(), "promo-founder")
	memberID := makeAgentID(t, comp.BoardConfig(), "promo-member")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "The Ascendants",
		FounderID: founderID,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}
	guildID := domain.GuildID(guild.ID)

	if err := comp.JoinGuild(ctx, guildID, memberID); err != nil {
		t.Fatalf("JoinGuild failed: %v", err)
	}

	// Promote from Initiate to Officer
	if err := comp.PromoteMember(ctx, guildID, memberID, domain.GuildRankOfficer); err != nil {
		t.Fatalf("PromoteMember failed: %v", err)
	}

	// Verify rank updated
	updated, ok := comp.GetGuild(guildID)
	if !ok {
		t.Fatal("GetGuild: guild not found after promotion")
	}

	for _, m := range updated.Members {
		if m.AgentID == domain.AgentID(memberID) {
			if m.Rank != domain.GuildRankOfficer {
				t.Errorf("Rank = %v, want %v", m.Rank, domain.GuildRankOfficer)
			}
			return
		}
	}
	t.Error("Promoted member not found in guild")
}

func TestComponent_PromoteMemberNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "promotenomember")
	defer comp.Stop(5 * time.Second)

	founderID := makeAgentID(t, comp.BoardConfig(), "promo-solo-founder")
	nonMemberID := makeAgentID(t, comp.BoardConfig(), "not-a-member")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "Solo Guild",
		FounderID: founderID,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	err = comp.PromoteMember(ctx, domain.GuildID(guild.ID), nonMemberID, domain.GuildRankOfficer)
	if err == nil {
		t.Error("PromoteMember should fail for non-member agent")
	}
}

func TestComponent_DisbandGuild(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "disband")
	defer comp.Stop(5 * time.Second)

	founderID := makeAgentID(t, comp.BoardConfig(), "disband-founder")
	memberID := makeAgentID(t, comp.BoardConfig(), "disband-member")

	guild, err := comp.CreateGuild(ctx, CreateGuildParams{
		Name:      "Doomed Brotherhood",
		FounderID: founderID,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}
	guildID := domain.GuildID(guild.ID)

	if err := comp.JoinGuild(ctx, guildID, memberID); err != nil {
		t.Fatalf("JoinGuild failed: %v", err)
	}

	// Verify guild appears in ListGuilds before disbanding
	active := comp.ListGuilds()
	if len(active) != 1 {
		t.Errorf("ListGuilds before disband = %d, want 1", len(active))
	}

	if err := comp.DisbandGuild(ctx, guildID, "mission accomplished"); err != nil {
		t.Fatalf("DisbandGuild failed: %v", err)
	}

	// Active guild list should no longer include the disbanded guild
	active = comp.ListGuilds()
	if len(active) != 0 {
		t.Errorf("ListGuilds after disband = %d, want 0", len(active))
	}

	// Agent-to-guild mappings should be cleared for all former members
	founderGuilds := comp.GetAgentGuilds(founderID)
	if len(founderGuilds) != 0 {
		t.Errorf("Founder guild count = %d, want 0 after disband", len(founderGuilds))
	}
	memberGuilds := comp.GetAgentGuilds(memberID)
	if len(memberGuilds) != 0 {
		t.Errorf("Member guild count = %d, want 0 after disband", len(memberGuilds))
	}
}

func TestComponent_DisbandGuildNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "disbandnotfound")
	defer comp.Stop(5 * time.Second)

	ghost := domain.GuildID("test.integration.game.disbandnotfound.guild.ghost")
	err := comp.DisbandGuild(ctx, ghost, "never existed")
	if err == nil {
		t.Error("DisbandGuild should fail for non-existent guild")
	}
}

func TestComponent_ListGuilds(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "listguilds")
	defer comp.Stop(5 * time.Second)

	// No guilds initially
	guilds := comp.ListGuilds()
	if len(guilds) != 0 {
		t.Errorf("Initial guild count = %d, want 0", len(guilds))
	}

	f1 := makeAgentID(t, comp.BoardConfig(), "list-founder1")
	f2 := makeAgentID(t, comp.BoardConfig(), "list-founder2")
	f3 := makeAgentID(t, comp.BoardConfig(), "list-founder3")

	// Create three guilds
	for _, params := range []CreateGuildParams{
		{Name: "Alpha", FounderID: f1},
		{Name: "Beta", FounderID: f2},
		{Name: "Gamma", FounderID: f3},
	} {
		if _, err := comp.CreateGuild(ctx, params); err != nil {
			t.Fatalf("CreateGuild %q failed: %v", params.Name, err)
		}
	}

	guilds = comp.ListGuilds()
	if len(guilds) != 3 {
		t.Errorf("Guild count = %d, want 3", len(guilds))
	}

	// Disband one
	g2, err := comp.CreateGuild(ctx, CreateGuildParams{Name: "ToDisband", FounderID: makeAgentID(t, comp.BoardConfig(), "disband-f")})
	if err != nil {
		t.Fatalf("CreateGuild ToDisband failed: %v", err)
	}
	if err := comp.DisbandGuild(ctx, domain.GuildID(g2.ID), "test cleanup"); err != nil {
		t.Fatalf("DisbandGuild failed: %v", err)
	}

	// Should still only show active guilds (not the disbanded one)
	guilds = comp.ListGuilds()
	if len(guilds) != 3 {
		t.Errorf("Active guild count after one disband = %d, want 3", len(guilds))
	}
}

func TestComponent_GetAgentGuilds_MultiGuild(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupGuildComponent(t, client, "multiguild")
	defer comp.Stop(5 * time.Second)

	// An agent can belong to multiple guilds
	agentID := makeAgentID(t, comp.BoardConfig(), "multi-member")
	f1 := makeAgentID(t, comp.BoardConfig(), "multi-f1")
	f2 := makeAgentID(t, comp.BoardConfig(), "multi-f2")

	g1, err := comp.CreateGuild(ctx, CreateGuildParams{Name: "First", FounderID: f1})
	if err != nil {
		t.Fatalf("CreateGuild 1 failed: %v", err)
	}
	g2, err := comp.CreateGuild(ctx, CreateGuildParams{Name: "Second", FounderID: f2})
	if err != nil {
		t.Fatalf("CreateGuild 2 failed: %v", err)
	}

	if err := comp.JoinGuild(ctx, domain.GuildID(g1.ID), agentID); err != nil {
		t.Fatalf("JoinGuild 1 failed: %v", err)
	}
	if err := comp.JoinGuild(ctx, domain.GuildID(g2.ID), agentID); err != nil {
		t.Fatalf("JoinGuild 2 failed: %v", err)
	}

	agentGuilds := comp.GetAgentGuilds(agentID)
	if len(agentGuilds) != 2 {
		t.Errorf("Agent guild count = %d, want 2", len(agentGuilds))
	}
}

func TestComponent_HealthReflectsErrors(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupGuildComponent(t, client, "healtherror")
	defer comp.Stop(5 * time.Second)

	// Artificially inject an error count
	comp.errorsCount.Add(3)

	health := comp.Health()
	if health.ErrorCount != 3 {
		t.Errorf("Health.ErrorCount = %d, want 3", health.ErrorCount)
	}
	if health.LastError == "" {
		t.Error("Health.LastError should be set when errors > 0")
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func setupGuildComponent(t *testing.T, client *natsclient.Client, board string) *Component {
	t.Helper()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = board
	config.EnableAutoFormation = false // Suppress agent watcher in unit tests

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists (mirrors main.go startup)
	gc := semdragons.NewGraphClient(client, comp.BoardConfig())
	ctx := context.Background()
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}

// makeAgentID constructs a fully-qualified agent entity ID for tests.
func makeAgentID(t *testing.T, cfg *domain.BoardConfig, suffix string) domain.AgentID {
	t.Helper()
	instance := suffix + "-" + domain.GenerateInstance()
	return domain.AgentID(cfg.AgentEntityID(instance))
}
