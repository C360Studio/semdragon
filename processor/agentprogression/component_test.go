//go:build integration

package agentprogression

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/questboard"
)

// =============================================================================
// INTEGRATION TESTS - AgentProgression Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/agentprogression/...
// =============================================================================

// TestXPPreservesAgentData verifies that XP updates via read-modify-write
// preserve all agent fields (name, skills, guilds, status). This is the
// regression test for the AgentXPPayload overwrite bug.
func TestXPPreservesAgentData(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "xppreserve")
	defer comp.Stop(5 * time.Second)

	gc := comp.graph

	// Create an agent with rich state (name, skills, guilds)
	instance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.boardConfig.AgentEntityID(instance))
	agent := &semdragons.Agent{
		ID:          agentID,
		Name:        "xp-preserve-agent",
		DisplayName: "Shadow Weaver",
		Status:      semdragons.AgentIdle,
		Level:       5,
		XP:          200,
		XPToLevel:   300,
		Tier:        semdragons.TierApprentice,
		Guilds:      []semdragons.GuildID{"guild-alpha", "guild-beta"},
		SkillProficiencies: map[semdragons.SkillTag]semdragons.SkillProficiency{
			semdragons.SkillCodeGen:  {Level: semdragons.ProficiencyJourneyman},
			semdragons.SkillAnalysis: {Level: semdragons.ProficiencyExpert},
		},
		Stats: semdragons.AgentStats{
			QuestsCompleted: 10,
			QuestsFailed:    2,
		},
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Create a quest claimed by this agent, then transition it to completed.
	// The component's KV watcher will detect the transition and process XP.
	questInstance := semdragons.GenerateInstance()
	questID := domain.QuestID(comp.boardConfig.QuestEntityID(questInstance))
	quest := &questboard.Quest{
		ID:          questID,
		Title:       "XP Preserve Test Quest",
		Status:      domain.QuestInProgress,
		Difficulty:  semdragons.DifficultyTrivial,
		BaseXP:      50,
		MaxAttempts: 3,
		Attempts:    1,
		ClaimedBy:   (*domain.AgentID)(&agentID),
	}

	// Write the quest with in_progress status first (seeds the watcher cache)
	if err := gc.PutEntityState(ctx, quest, "quest.started"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Give the watcher time to cache the in_progress state
	time.Sleep(200 * time.Millisecond)

	// Transition quest to completed — this triggers XP processing
	quest.Status = domain.QuestCompleted
	now := time.Now()
	quest.CompletedAt = &now
	if err := gc.EmitEntityUpdate(ctx, quest, "quest.completed"); err != nil {
		t.Fatalf("Failed to complete test quest: %v", err)
	}

	// Wait for the reactive handler to process the completion
	time.Sleep(500 * time.Millisecond)

	// Read agent back from KV
	agentEntity, err := gc.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("Failed to read agent after XP update: %v", err)
	}
	updatedAgent := semdragons.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	// Verify XP was applied (agent should have more XP than before)
	if updatedAgent.XP <= 200 && updatedAgent.Level <= 5 {
		t.Errorf("XP should have increased: got XP=%d Level=%d", updatedAgent.XP, updatedAgent.Level)
	}

	// CRITICAL: Verify all other fields are preserved (the overwrite bug regression)
	if updatedAgent.Name != "xp-preserve-agent" {
		t.Errorf("Name was overwritten: got %q, want %q", updatedAgent.Name, "xp-preserve-agent")
	}
	if updatedAgent.DisplayName != "Shadow Weaver" {
		t.Errorf("DisplayName was overwritten: got %q, want %q", updatedAgent.DisplayName, "Shadow Weaver")
	}
	if len(updatedAgent.Guilds) != 2 {
		t.Errorf("Guilds were overwritten: got %d guilds, want 2", len(updatedAgent.Guilds))
	}
	if len(updatedAgent.SkillProficiencies) != 2 {
		t.Errorf("Skills were overwritten: got %d skills, want 2", len(updatedAgent.SkillProficiencies))
	}
	if updatedAgent.Stats.QuestsCompleted != 11 {
		t.Errorf("QuestsCompleted = %d, want 11 (was 10 + 1 completion)", updatedAgent.Stats.QuestsCompleted)
	}
}

// TestCompletionResetsAgent verifies that quest completion sets agent to idle
// with CurrentQuest cleared and QuestsCompleted incremented.
func TestCompletionResetsAgent(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "compreset")
	defer comp.Stop(5 * time.Second)

	gc := comp.graph

	// Create agent in on_quest status with CurrentQuest set
	instance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.boardConfig.AgentEntityID(instance))
	questInstance := semdragons.GenerateInstance()
	questID := semdragons.QuestID(comp.boardConfig.QuestEntityID(questInstance))
	agent := &semdragons.Agent{
		ID:           agentID,
		Name:         "completion-agent",
		Status:       semdragons.AgentOnQuest,
		Level:        3,
		XP:           100,
		XPToLevel:    200,
		Tier:         semdragons.TierApprentice,
		CurrentQuest: &questID,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Create quest in_progress first, then complete it
	quest := &questboard.Quest{
		ID:          domain.QuestID(questID),
		Title:       "Completion Reset Test",
		Status:      domain.QuestInProgress,
		Difficulty:  semdragons.DifficultyTrivial,
		BaseXP:      50,
		MaxAttempts: 3,
		Attempts:    1,
		ClaimedBy:   (*domain.AgentID)(&agentID),
	}
	if err := gc.PutEntityState(ctx, quest, "quest.started"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Complete the quest
	quest.Status = domain.QuestCompleted
	now := time.Now()
	quest.CompletedAt = &now
	if err := gc.EmitEntityUpdate(ctx, quest, "quest.completed"); err != nil {
		t.Fatalf("Failed to complete test quest: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Read agent back
	agentEntity, err := gc.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("Failed to read agent: %v", err)
	}
	updatedAgent := semdragons.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent")
	}

	if updatedAgent.Status != semdragons.AgentIdle {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, semdragons.AgentIdle)
	}
	if updatedAgent.CurrentQuest != nil {
		t.Errorf("CurrentQuest should be nil, got %v", updatedAgent.CurrentQuest)
	}
	if updatedAgent.Stats.QuestsCompleted != 1 {
		t.Errorf("QuestsCompleted = %d, want 1", updatedAgent.Stats.QuestsCompleted)
	}
}

// TestFailureSetsAgentCooldown verifies that quest failure with penalty sets
// agent to cooldown status with CooldownUntil and increments QuestsFailed.
func TestFailureSetsAgentCooldown(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "failcooldown")
	defer comp.Stop(5 * time.Second)

	gc := comp.graph

	// Create agent
	instance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.boardConfig.AgentEntityID(instance))
	questInstance := semdragons.GenerateInstance()
	questID := semdragons.QuestID(comp.boardConfig.QuestEntityID(questInstance))
	agent := &semdragons.Agent{
		ID:           agentID,
		Name:         "cooldown-agent",
		Status:       semdragons.AgentOnQuest,
		Level:        3,
		XP:           100,
		XPToLevel:    200,
		Tier:         semdragons.TierApprentice,
		CurrentQuest: &questID,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Create quest in_progress, then fail it permanently
	quest := &questboard.Quest{
		ID:            domain.QuestID(questID),
		Title:         "Cooldown Failure Test",
		Status:        domain.QuestInProgress,
		Difficulty:    semdragons.DifficultyTrivial,
		BaseXP:        100,
		MaxAttempts:   1,
		Attempts:      1,
		ClaimedBy:     (*domain.AgentID)(&agentID),
		FailureType:   questboard.FailureQuality,
		FailureReason: "quality too low",
	}
	if err := gc.PutEntityState(ctx, quest, "quest.started"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Fail the quest permanently (attempts == maxAttempts)
	quest.Status = domain.QuestFailed
	if err := gc.EmitEntityUpdate(ctx, quest, "quest.failed"); err != nil {
		t.Fatalf("Failed to fail test quest: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Read agent back
	agentEntity, err := gc.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("Failed to read agent: %v", err)
	}
	updatedAgent := semdragons.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent")
	}

	// Default failure type is FailureSoft (from questboard.FailureQuality mapping),
	// which has CooldownDur of 1 minute
	if updatedAgent.Status != semdragons.AgentCooldown {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, semdragons.AgentCooldown)
	}
	if updatedAgent.CooldownUntil == nil {
		t.Error("CooldownUntil should be set")
	} else if updatedAgent.CooldownUntil.Before(time.Now()) {
		t.Error("CooldownUntil should be in the future")
	}
	if updatedAgent.CurrentQuest != nil {
		t.Errorf("CurrentQuest should be nil, got %v", updatedAgent.CurrentQuest)
	}
	if updatedAgent.Stats.QuestsFailed != 1 {
		t.Errorf("QuestsFailed = %d, want 1", updatedAgent.Stats.QuestsFailed)
	}
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

	ctx := context.Background()

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists (mirrors main.go startup)
	gc := semdragons.NewGraphClient(client, comp.BoardConfig())
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}
