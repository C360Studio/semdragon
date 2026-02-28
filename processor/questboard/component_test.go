//go:build integration

package questboard

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// INTEGRATION TESTS - QuestBoard Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/questboard/...
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	// Create component via factory
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
	if meta.Name != "questboard" {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, "questboard")
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

func TestComponent_PostQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "postquest")
	defer comp.Stop(5 * time.Second)

	// Post a quest
	quest := semdragons.Quest{
		Title:       "Test Quest",
		Description: "A test quest for integration testing",
		Difficulty:  semdragons.DifficultyModerate,
		BaseXP:      100,
	}

	posted, err := comp.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Verify quest properties
	if posted.ID == "" {
		t.Error("Quest ID should be set")
	}
	if posted.Status != semdragons.QuestPosted {
		t.Errorf("Status = %v, want %v", posted.Status, semdragons.QuestPosted)
	}
	if posted.TrajectoryID == "" {
		t.Error("TrajectoryID should be set")
	}
	if posted.PostedAt.IsZero() {
		t.Error("PostedAt should be set")
	}

	// Verify we can retrieve it
	retrieved, err := comp.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if retrieved.Title != "Test Quest" {
		t.Errorf("Title = %q, want %q", retrieved.Title, "Test Quest")
	}
}

func TestComponent_ClaimQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "claimquest")
	defer comp.Stop(5 * time.Second)

	// Create an agent first
	agent := createTestAgent(t, comp.Storage(), comp.BoardConfig(), "claim-agent", 5)

	// Post a quest
	quest, err := comp.PostQuest(ctx, semdragons.Quest{
		Title:      "Claimable Quest",
		Difficulty: semdragons.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Claim the quest
	err = comp.ClaimQuest(ctx, quest.ID, agent.ID)
	if err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}

	// Verify claim
	claimed, err := comp.GetQuest(ctx, quest.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if claimed.Status != semdragons.QuestClaimed {
		t.Errorf("Status = %v, want %v", claimed.Status, semdragons.QuestClaimed)
	}
	if claimed.ClaimedBy == nil || *claimed.ClaimedBy != agent.ID {
		t.Error("ClaimedBy should be set to agent ID")
	}
	if claimed.ClaimedAt == nil {
		t.Error("ClaimedAt should be set")
	}
}

func TestComponent_QuestLifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "lifecycle")
	defer comp.Stop(5 * time.Second)

	// Create agent
	agent := createTestAgent(t, comp.Storage(), comp.BoardConfig(), "lifecycle-agent", 5)

	// Post quest
	quest, err := comp.PostQuest(ctx, semdragons.Quest{
		Title:      "Lifecycle Test Quest",
		Difficulty: semdragons.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}
	t.Logf("Posted quest: %s", quest.ID)

	// Claim
	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}
	t.Log("Claimed quest")

	// Start
	if err := comp.StartQuest(ctx, quest.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}

	started, _ := comp.GetQuest(ctx, quest.ID)
	if started.Status != semdragons.QuestInProgress {
		t.Errorf("Status = %v, want %v", started.Status, semdragons.QuestInProgress)
	}
	if started.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
	t.Log("Started quest")

	// Submit result (no review required)
	battle, err := comp.SubmitResult(ctx, quest.ID, map[string]any{"result": "success"})
	if err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Since RequireReview defaults to false, quest should be completed directly
	submitted, _ := comp.GetQuest(ctx, quest.ID)
	if submitted.Status != semdragons.QuestCompleted {
		t.Errorf("Status = %v, want %v (no review required)", submitted.Status, semdragons.QuestCompleted)
	}
	if battle != nil {
		t.Error("Battle should be nil when RequireReview is false")
	}
	t.Log("Quest completed directly (no review)")
}

func TestComponent_QuestWithReview(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "review")
	defer comp.Stop(5 * time.Second)

	// Create agent with level 7 (TierJourneyman) to be able to claim DifficultyModerate quests
	agent := createTestAgent(t, comp.Storage(), comp.BoardConfig(), "review-agent", 7)

	// Post quest that requires review
	quest, err := comp.PostQuest(ctx, semdragons.Quest{
		Title:      "Review Required Quest",
		Difficulty: semdragons.DifficultyModerate,
		Constraints: semdragons.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   semdragons.ReviewStandard,
		},
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Claim and start
	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}
	if err := comp.StartQuest(ctx, quest.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}

	// Submit result
	battle, err := comp.SubmitResult(ctx, quest.ID, map[string]any{"code": "function() {}"})
	if err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Should create battle
	if battle == nil {
		t.Fatal("Battle should be created when RequireReview is true")
	}
	if battle.Status != semdragons.BattleActive {
		t.Errorf("Battle.Status = %v, want %v", battle.Status, semdragons.BattleActive)
	}
	if battle.QuestID != quest.ID {
		t.Errorf("Battle.QuestID = %v, want %v", battle.QuestID, quest.ID)
	}

	// Quest should be in review
	inReview, _ := comp.GetQuest(ctx, quest.ID)
	if inReview.Status != semdragons.QuestInReview {
		t.Errorf("Status = %v, want %v", inReview.Status, semdragons.QuestInReview)
	}

	// Complete the quest with a passing verdict
	verdict := semdragons.BattleVerdict{
		Passed:       true,
		QualityScore: 0.85,
		XPAwarded:    100,
		Feedback:     "Good work!",
	}
	if err := comp.CompleteQuest(ctx, quest.ID, verdict); err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	completed, _ := comp.GetQuest(ctx, quest.ID)
	if completed.Status != semdragons.QuestCompleted {
		t.Errorf("Status = %v, want %v", completed.Status, semdragons.QuestCompleted)
	}
}

func TestComponent_FailQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "failquest")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.Storage(), comp.BoardConfig(), "fail-agent", 5)

	// Post quest with max attempts = 2
	quest, err := comp.PostQuest(ctx, semdragons.Quest{
		Title:       "Fail Test Quest",
		Difficulty:  semdragons.DifficultyTrivial,
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// First attempt - should repost
	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest (1) failed: %v", err)
	}
	if err := comp.StartQuest(ctx, quest.ID); err != nil {
		t.Fatalf("StartQuest (1) failed: %v", err)
	}
	if err := comp.FailQuest(ctx, quest.ID, "Test failure 1"); err != nil {
		t.Fatalf("FailQuest (1) failed: %v", err)
	}

	failed1, _ := comp.GetQuest(ctx, quest.ID)
	if failed1.Status != semdragons.QuestPosted {
		t.Errorf("After fail 1: Status = %v, want %v (reposted)", failed1.Status, semdragons.QuestPosted)
	}
	if failed1.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", failed1.Attempts)
	}

	// Second attempt - should fail permanently
	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest (2) failed: %v", err)
	}
	if err := comp.StartQuest(ctx, quest.ID); err != nil {
		t.Fatalf("StartQuest (2) failed: %v", err)
	}
	if err := comp.FailQuest(ctx, quest.ID, "Test failure 2"); err != nil {
		t.Fatalf("FailQuest (2) failed: %v", err)
	}

	failed2, _ := comp.GetQuest(ctx, quest.ID)
	if failed2.Status != semdragons.QuestFailed {
		t.Errorf("After fail 2: Status = %v, want %v (permanent)", failed2.Status, semdragons.QuestFailed)
	}
}

func TestComponent_AbandonQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "abandon")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.Storage(), comp.BoardConfig(), "abandon-agent", 5)

	quest, err := comp.PostQuest(ctx, semdragons.Quest{
		Title:      "Abandon Test Quest",
		Difficulty: semdragons.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Claim then abandon
	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}

	if err := comp.AbandonQuest(ctx, quest.ID, "Changed my mind"); err != nil {
		t.Fatalf("AbandonQuest failed: %v", err)
	}

	abandoned, _ := comp.GetQuest(ctx, quest.ID)
	if abandoned.Status != semdragons.QuestPosted {
		t.Errorf("Status = %v, want %v (back to posted)", abandoned.Status, semdragons.QuestPosted)
	}
	if abandoned.ClaimedBy != nil {
		t.Error("ClaimedBy should be nil after abandon")
	}
}

func TestComponent_EscalateQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "escalate")
	defer comp.Stop(5 * time.Second)

	// Create agent with level 7 (TierJourneyman) to be able to claim DifficultyModerate quests
	agent := createTestAgent(t, comp.Storage(), comp.BoardConfig(), "escalate-agent", 7)

	quest, err := comp.PostQuest(ctx, semdragons.Quest{
		Title:      "Escalate Test Quest",
		Difficulty: semdragons.DifficultyModerate,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Claim and start
	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}
	if err := comp.StartQuest(ctx, quest.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}

	// Escalate
	if err := comp.EscalateQuest(ctx, quest.ID, "Need DM attention"); err != nil {
		t.Fatalf("EscalateQuest failed: %v", err)
	}

	escalated, _ := comp.GetQuest(ctx, quest.ID)
	if escalated.Status != semdragons.QuestEscalated {
		t.Errorf("Status = %v, want %v", escalated.Status, semdragons.QuestEscalated)
	}
	if !escalated.Escalated {
		t.Error("Escalated flag should be true")
	}
}

func TestComponent_BoardStats(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "stats")
	defer comp.Stop(5 * time.Second)

	// Post some quests
	for i := 0; i < 3; i++ {
		_, err := comp.PostQuest(ctx, semdragons.Quest{
			Title:      "Stats Test Quest",
			Difficulty: semdragons.DifficultyTrivial,
		})
		if err != nil {
			t.Fatalf("PostQuest %d failed: %v", i, err)
		}
	}

	// Get stats
	stats, err := comp.BoardStats(ctx)
	if err != nil {
		t.Fatalf("BoardStats failed: %v", err)
	}

	if stats.TotalPosted != 3 {
		t.Errorf("TotalPosted = %d, want 3", stats.TotalPosted)
	}
}

func TestComponent_AvailableQuests(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "available")
	defer comp.Stop(5 * time.Second)

	// Create agent with specific skills
	agent := createTestAgentWithSkills(t, comp.Storage(), comp.BoardConfig(), "avail-agent", 5,
		[]semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillAnalysis})

	// Post quests with different difficulties
	_, err := comp.PostQuest(ctx, semdragons.Quest{
		Title:      "Simple Quest",
		Difficulty: semdragons.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest (simple) failed: %v", err)
	}

	_, err = comp.PostQuest(ctx, semdragons.Quest{
		Title:      "Epic Quest (too hard)",
		Difficulty: semdragons.DifficultyEpic, // Requires higher tier
		MinTier:    semdragons.TierMaster,
	})
	if err != nil {
		t.Fatalf("PostQuest (epic) failed: %v", err)
	}

	// Get available quests for agent
	available, err := comp.AvailableQuests(ctx, agent.ID, semdragons.QuestFilter{})
	if err != nil {
		t.Fatalf("AvailableQuests failed: %v", err)
	}

	// Should only see the simple quest (epic is above agent's tier)
	if len(available) != 1 {
		t.Errorf("Got %d available quests, want 1", len(available))
	}
	if len(available) > 0 && available[0].Title != "Simple Quest" {
		t.Errorf("Available quest = %q, want %q", available[0].Title, "Simple Quest")
	}
}

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Error("Should have input ports defined")
	}

	outputs := comp.OutputPorts()
	if len(outputs) < 2 {
		t.Errorf("Should have at least 2 output ports, got %d", len(outputs))
	}

	// Check for quest-lifecycle output port
	hasQuestLifecycle := false
	for _, port := range outputs {
		if port.Name == "quest-lifecycle" {
			hasQuestLifecycle = true
			break
		}
	}
	if !hasQuestLifecycle {
		t.Error("Missing quest-lifecycle output port")
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

	// Check properties exist
	expectedProps := []string{"org", "platform", "board", "default_max_attempts", "enable_evaluation"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("Missing property %q in schema", prop)
		}
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

	instance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(config.AgentEntityID(instance))

	agent := &semdragons.Agent{
		ID:     agentID,
		Name:   name,
		Level:  level,
		Tier:   semdragons.TierFromLevel(level),
		Status: semdragons.AgentIdle,
		XP:     0,
	}

	// Set up skill proficiencies
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
