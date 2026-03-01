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

	// Create an agent with rich state (name, skills, guilds).
	// Use AgentOnQuest with CurrentQuest set — consistent with an agent actively working.
	instance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.boardConfig.AgentEntityID(instance))
	questInstance := semdragons.GenerateInstance()
	questID := semdragons.QuestID(comp.boardConfig.QuestEntityID(questInstance))
	agent := &semdragons.Agent{
		ID:           agentID,
		Name:         "xp-preserve-agent",
		DisplayName:  "Shadow Weaver",
		Status:       semdragons.AgentOnQuest,
		CurrentQuest: &questID,
		Level:        5,
		XP:           200,
		XPToLevel:    300,
		Tier:         semdragons.TierApprentice,
		Guilds:       []semdragons.GuildID{"guild-alpha", "guild-beta"},
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
	quest := &questboard.Quest{
		ID:          domain.QuestID(questID),
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

	// Wait for the reactive handler to process the completion and write back the agent.
	// For a trivial quest with BaseXP=50 and no verdict/streak, TotalXP == 50.
	// Starting XP=200, XPToLevel=300, so the agent gains 50 XP → new XP=250 (no level-up).
	updatedAgent := waitForAgentUpdate(t, gc, agentID,
		func(a *semdragons.Agent) bool { return a.XP > 200 },
		3*time.Second,
		"agent XP to increase above 200 after quest completion")

	// XP assertions: base-only award is 50, so new XP should be ~250 (no level-up)
	if updatedAgent.XP <= 200 {
		t.Errorf("XP should have increased: got XP=%d, want > 200", updatedAgent.XP)
	}
	// Level must not change: XP gained (50) < XPToLevel (300)
	if updatedAgent.Level != 5 {
		t.Errorf("Level should not have changed: got %d, want 5", updatedAgent.Level)
	}
	// Agent is reset to idle by the completion handler
	if updatedAgent.Status != semdragons.AgentIdle {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, semdragons.AgentIdle)
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

	// Wait for the reactive handler to reset the agent to idle
	updatedAgent := waitForAgentUpdate(t, gc, agentID,
		func(a *semdragons.Agent) bool { return a.Status == semdragons.AgentIdle },
		3*time.Second,
		"agent Status to become idle after quest completion")

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

	// Wait for the reactive handler to apply the failure penalty and set cooldown.
	// Penalty math: baseLoss = 100 * 0.5 = 50; FailureSoft multiplier 0.25 = 12.5 → 12;
	// difficulty scale = 1.0 + 0*0.2 = 1.0 (DifficultyTrivial == 0); XPLost = 12.
	// Starting XP=100 → new XP should be ~88.
	updatedAgent := waitForAgentUpdate(t, gc, agentID,
		func(a *semdragons.Agent) bool { return a.Status == semdragons.AgentCooldown },
		3*time.Second,
		"agent Status to become cooldown after quest failure")

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
	// XP penalty: starting XP=100, XPLost~=12, so new XP should be below 100
	if updatedAgent.XP >= 100 {
		t.Errorf("XP should have decreased due to penalty: got XP=%d, want < 100", updatedAgent.XP)
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// waitForAgentUpdate polls KV until check returns true for the agent, or timeout
// is exceeded. Returns the passing agent state. Fails the test on timeout.
//
// This replaces fixed time.Sleep + manual read-back patterns. Polling at 50ms
// intervals means the test responds as soon as the handler writes the update,
// rather than waiting an arbitrary fixed duration.
func waitForAgentUpdate(t *testing.T, gc *semdragons.GraphClient, agentID semdragons.AgentID, check func(*semdragons.Agent) bool, timeout time.Duration, msg string) *semdragons.Agent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entity, err := gc.GetAgent(context.Background(), agentID)
		if err == nil {
			agent := semdragons.AgentFromEntityState(entity)
			if agent != nil && check(agent) {
				return agent
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
	return nil
}

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
