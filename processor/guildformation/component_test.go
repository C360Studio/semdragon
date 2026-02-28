//go:build integration

package guildformation

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// INTEGRATION TESTS - GuildFormation Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/guildformation/...
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "lifecycle"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	// Test Initialize
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test Meta
	meta := comp.Meta()
	if meta.Name != "guildformation" {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, "guildformation")
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
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

	// Test Stop
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stopped
	health = comp.Health()
	if health.Healthy {
		t.Error("Component should not be healthy after stop")
	}
}

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Error("Should have input ports defined")
	}

	// Check for agent-state input port
	hasAgentState := false
	for _, port := range inputs {
		if port.Name == "agent-state" {
			hasAgentState = true
			break
		}
	}
	if !hasAgentState {
		t.Error("Missing agent-state input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Error("Should have output ports defined")
	}

	// Check for guild-state output port
	hasGuildState := false
	for _, port := range outputs {
		if port.Name == "guild-state" {
			hasGuildState = true
			break
		}
	}
	if !hasGuildState {
		t.Error("Missing guild-state output port")
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
			t.Errorf("Field %q should be required", field)
		}
	}

	// Check guild formation properties exist
	expectedProps := []string{"min_founder_level", "founding_xp_cost", "default_max_members"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("Missing property %q in schema", prop)
		}
	}
}

func TestComponent_FoundGuild(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "foundguild")
	defer comp.Stop(5 * time.Second)

	// Create a high-level agent (level 11+ can found guilds)
	agent := createTestAgent(t, comp.Storage(), comp.boardConfig, "founder", 11)

	// Found a guild
	guild, err := comp.FoundGuild(ctx, agent.ID, "Test Guild", "Testing")
	if err != nil {
		t.Fatalf("FoundGuild failed: %v", err)
	}

	if guild == nil {
		t.Fatal("Guild should not be nil")
	}

	if guild.Name != "Test Guild" {
		t.Errorf("Guild.Name = %q, want %q", guild.Name, "Test Guild")
	}

	if guild.Culture != "Testing" {
		t.Errorf("Guild.Culture = %q, want %q", guild.Culture, "Testing")
	}

	if guild.FoundedBy != agent.ID {
		t.Errorf("Guild.FoundedBy = %v, want %v", guild.FoundedBy, agent.ID)
	}

	// Founder should be a member with Guildmaster rank
	foundMaster := false
	for _, member := range guild.Members {
		if member.AgentID == agent.ID && member.Rank == semdragons.GuildRankMaster {
			foundMaster = true
			break
		}
	}
	if !foundMaster {
		t.Error("Founder should be a Guildmaster member")
	}
}

func TestComponent_Stats(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "stats")
	defer comp.Stop(5 * time.Second)

	// Get initial stats
	stats := comp.Stats()
	initialGuilds := stats.GuildsCreated

	// Create a guild
	agent := createTestAgent(t, comp.Storage(), comp.boardConfig, "stats-founder", 11)
	_, err := comp.FoundGuild(ctx, agent.ID, "Stats Guild", "Testing")
	if err != nil {
		t.Fatalf("FoundGuild failed: %v", err)
	}

	// Stats should reflect created guild
	stats = comp.Stats()
	if stats.GuildsCreated != initialGuilds+1 {
		t.Errorf("GuildsCreated = %d, want %d", stats.GuildsCreated, initialGuilds+1)
	}
}

func TestComponent_DetectSkillClusters(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "clusters")
	defer comp.Stop(5 * time.Second)

	// Create agents with similar skills (should form a cluster)
	for i := 0; i < 5; i++ {
		createTestAgentWithSkills(t, comp.Storage(), comp.boardConfig, "cluster-agent", 8,
			[]semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillCodeReview})
	}

	// Detect clusters
	suggestions, err := comp.DetectSkillClusters(ctx)
	if err != nil {
		t.Fatalf("DetectSkillClusters failed: %v", err)
	}

	// Should have some suggestions (may vary based on algorithm parameters)
	t.Logf("Detected %d cluster suggestions", len(suggestions))
}

// =============================================================================
// HELPERS
// =============================================================================

func setupComponent(t *testing.T, client *natsclient.Client, name string) *Component {
	t.Helper()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = name

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}

func createTestAgent(t *testing.T, storage *semdragons.Storage, config *semdragons.BoardConfig, name string, level int) *semdragons.Agent {
	t.Helper()
	return createTestAgentWithSkills(t, storage, config, name, level, nil)
}

func createTestAgentWithSkills(t *testing.T, storage *semdragons.Storage, config *semdragons.BoardConfig, name string, level int, skills []semdragons.SkillTag) *semdragons.Agent {
	t.Helper()

	// Use just the instance as the ID - the engine expects instance IDs, not full entity IDs
	instance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(instance)

	agent := &semdragons.Agent{
		ID:     agentID,
		Name:   name,
		Level:  level,
		Tier:   semdragons.TierFromLevel(level),
		Status: semdragons.AgentIdle,
		XP:     1000, // Give some XP for founding guilds
	}

	if len(skills) > 0 {
		agent.SkillProficiencies = make(map[semdragons.SkillTag]semdragons.SkillProficiency)
		for _, skill := range skills {
			agent.SkillProficiencies[skill] = semdragons.SkillProficiency{
				Level: semdragons.ProficiencyJourneyman,
			}
		}
	}

	ctx := context.Background()
	if err := storage.PutAgent(ctx, instance, agent); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	return agent
}
