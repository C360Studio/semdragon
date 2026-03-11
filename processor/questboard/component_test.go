//go:build integration

package questboard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/partycoord"
)

// =============================================================================
// INTEGRATION TESTS - QuestBoard Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/questboard/...
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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

	// Ensure board-specific KV bucket exists (mirrors main.go startup)
	gc := semdragons.NewGraphClient(client, comp.BoardConfig())
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "postquest")
	defer comp.Stop(5 * time.Second)

	// Post a quest
	quest := domain.Quest{
		Title:       "Test Quest",
		Description: "A test quest for integration testing",
		Difficulty:  domain.DifficultyModerate,
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
	if posted.Status != domain.QuestPosted {
		t.Errorf("Status = %v, want %v", posted.Status, domain.QuestPosted)
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "claimquest")
	defer comp.Stop(5 * time.Second)

	// Create an agent first
	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "claim-agent", 5)

	// Post a quest
	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Claimable Quest",
		Difficulty: domain.DifficultyTrivial,
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
	if claimed.Status != domain.QuestClaimed {
		t.Errorf("Status = %v, want %v", claimed.Status, domain.QuestClaimed)
	}
	if claimed.ClaimedBy == nil || *claimed.ClaimedBy != agent.ID {
		t.Error("ClaimedBy should be set to agent ID")
	}
	if claimed.ClaimedAt == nil {
		t.Error("ClaimedAt should be set")
	}
}

func TestComponent_QuestLifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "lifecycle")
	defer comp.Stop(5 * time.Second)

	// Create agent
	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "lifecycle-agent", 5)

	// Post quest
	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Lifecycle Test Quest",
		Difficulty: domain.DifficultyTrivial,
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
	if started.Status != domain.QuestInProgress {
		t.Errorf("Status = %v, want %v", started.Status, domain.QuestInProgress)
	}
	if started.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
	t.Log("Started quest")

	// Submit result (no review required)
	err = comp.SubmitResult(ctx, quest.ID, map[string]any{"result": "success"})
	if err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Since RequireReview defaults to false, quest should be completed directly
	submitted, _ := comp.GetQuest(ctx, quest.ID)
	if submitted.Status != domain.QuestCompleted {
		t.Errorf("Status = %v, want %v (no review required)", submitted.Status, domain.QuestCompleted)
	}
	t.Log("Quest completed directly (no review)")
}

func TestComponent_QuestWithReview(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "review")
	defer comp.Stop(5 * time.Second)

	// Create agent with level 7 (TierJourneyman) to be able to claim DifficultyModerate quests
	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "review-agent", 7)

	// Post quest that requires review
	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Review Required Quest",
		Difficulty: domain.DifficultyModerate,
		Constraints: domain.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   domain.ReviewStandard,
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
	err = comp.SubmitResult(ctx, quest.ID, map[string]any{"code": "function() {}"})
	if err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Quest should be in review (bossbattle processor handles battle creation reactively)
	inReview, _ := comp.GetQuest(ctx, quest.ID)
	if inReview.Status != domain.QuestInReview {
		t.Errorf("Status = %v, want %v", inReview.Status, domain.QuestInReview)
	}

	// Complete the quest with a passing verdict
	verdict := domain.BattleVerdict{
		Passed:       true,
		QualityScore: 0.85,
		XPAwarded:    100,
		Feedback:     "Good work!",
	}
	if err := comp.CompleteQuest(ctx, quest.ID, verdict); err != nil {
		t.Fatalf("CompleteQuest failed: %v", err)
	}

	completed, _ := comp.GetQuest(ctx, quest.ID)
	if completed.Status != domain.QuestCompleted {
		t.Errorf("Status = %v, want %v", completed.Status, domain.QuestCompleted)
	}
}

func TestComponent_FailQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "failquest")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "fail-agent", 5)

	// Post quest with max attempts = 2
	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:       "Fail Test Quest",
		Difficulty:  domain.DifficultyTrivial,
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
	if failed1.Status != domain.QuestPosted {
		t.Errorf("After fail 1: Status = %v, want %v (reposted)", failed1.Status, domain.QuestPosted)
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
	if failed2.Status != domain.QuestFailed {
		t.Errorf("After fail 2: Status = %v, want %v (permanent)", failed2.Status, domain.QuestFailed)
	}
}

func TestComponent_AbandonQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "abandon")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "abandon-agent", 5)

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Abandon Test Quest",
		Difficulty: domain.DifficultyTrivial,
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
	if abandoned.Status != domain.QuestPosted {
		t.Errorf("Status = %v, want %v (back to posted)", abandoned.Status, domain.QuestPosted)
	}
	if abandoned.ClaimedBy != nil {
		t.Error("ClaimedBy should be nil after abandon")
	}
}

func TestComponent_EscalateQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "escalate")
	defer comp.Stop(5 * time.Second)

	// Create agent with level 7 (TierJourneyman) to be able to claim DifficultyModerate quests
	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "escalate-agent", 7)

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Escalate Test Quest",
		Difficulty: domain.DifficultyModerate,
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
	if escalated.Status != domain.QuestEscalated {
		t.Errorf("Status = %v, want %v", escalated.Status, domain.QuestEscalated)
	}
	if !escalated.Escalated {
		t.Error("Escalated flag should be true")
	}
}

func TestComponent_BoardStats(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "stats")
	defer comp.Stop(5 * time.Second)

	// Post some quests
	for i := 0; i < 3; i++ {
		_, err := comp.PostQuest(ctx, domain.Quest{
			Title:      "Stats Test Quest",
			Difficulty: domain.DifficultyTrivial,
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "available")
	defer comp.Stop(5 * time.Second)

	// Create agent with specific skills
	agent := createTestAgentWithSkills(t, comp.GraphClient(), comp.BoardConfig(), "avail-agent", 5,
		[]domain.SkillTag{domain.SkillCodeGen, domain.SkillAnalysis})

	// Post quests with different difficulties
	_, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Simple Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest (simple) failed: %v", err)
	}

	_, err = comp.PostQuest(ctx, domain.Quest{
		Title:      "Epic Quest (too hard)",
		Difficulty: domain.DifficultyEpic, // Requires higher tier
		MinTier:    domain.TierMaster,
	})
	if err != nil {
		t.Fatalf("PostQuest (epic) failed: %v", err)
	}

	// Get available quests for agent
	available, err := comp.AvailableQuests(ctx, agent.ID, QuestFilter{})
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
// AGENT STATUS LIFECYCLE TESTS
// =============================================================================

// TestClaimSetsAgentOnQuest verifies that claiming a quest transitions the
// agent to on_quest status with CurrentQuest set.
func TestClaimSetsAgentOnQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "claimstatus")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "status-claim-agent", 5)

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Claim Status Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}

	// Read agent back from KV to verify status was written
	agentEntity, err := comp.GraphClient().GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	if updatedAgent.Status != domain.AgentOnQuest {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, domain.AgentOnQuest)
	}
	if updatedAgent.CurrentQuest == nil {
		t.Fatal("CurrentQuest should be set after claim")
	}
	if *updatedAgent.CurrentQuest != domain.QuestID(quest.ID) {
		t.Errorf("CurrentQuest = %v, want %v", *updatedAgent.CurrentQuest, quest.ID)
	}
}

// TestAbandonResetsAgent verifies that abandoning a quest resets the agent
// to idle with CurrentQuest cleared.
func TestAbandonResetsAgent(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "abandonreset")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "status-abandon-agent", 5)

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Abandon Reset Quest",
		Difficulty: domain.DifficultyTrivial,
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

	// Read agent back from KV
	agentEntity, err := comp.GraphClient().GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	if updatedAgent.Status != domain.AgentIdle {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, domain.AgentIdle)
	}
	if updatedAgent.CurrentQuest != nil {
		t.Errorf("CurrentQuest should be nil after abandon, got %v", updatedAgent.CurrentQuest)
	}
}

// TestFailRepostResetsAgent verifies that when a quest fails with retries
// remaining (repost path), the agent is reset to idle so it can claim again.
func TestFailRepostResetsAgent(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "failrepost")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "status-repost-agent", 5)

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:       "Fail Repost Quest",
		Difficulty:  domain.DifficultyTrivial,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Claim → start → fail (should repost because MaxAttempts=3, Attempts=1)
	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}
	if err := comp.StartQuest(ctx, quest.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}
	if err := comp.FailQuest(ctx, quest.ID, "First failure"); err != nil {
		t.Fatalf("FailQuest failed: %v", err)
	}

	// Verify quest was reposted
	reposted, _ := comp.GetQuest(ctx, quest.ID)
	if reposted.Status != domain.QuestPosted {
		t.Fatalf("Quest should be reposted, got %v", reposted.Status)
	}

	// Read agent back from KV
	agentEntity, err := comp.GraphClient().GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	if updatedAgent.Status != domain.AgentIdle {
		t.Errorf("Status = %v, want %v (should reset on repost)", updatedAgent.Status, domain.AgentIdle)
	}
	if updatedAgent.CurrentQuest != nil {
		t.Errorf("CurrentQuest should be nil after repost, got %v", updatedAgent.CurrentQuest)
	}

	// Verify the agent can re-claim the reposted quest
	if err := comp.ClaimQuest(ctx, quest.ID, agent.ID); err != nil {
		t.Errorf("Agent should be able to re-claim after repost, got: %v", err)
	}
}

// TestRejectsClaimWhenOnQuest verifies that an agent with CurrentQuest set
// cannot claim another quest.
func TestRejectsClaimWhenOnQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "rejectonquest")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "busy-agent", 5)

	// Post two quests
	quest1, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "First Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest 1 failed: %v", err)
	}
	quest2, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Second Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest 2 failed: %v", err)
	}

	// Claim first quest
	if err := comp.ClaimQuest(ctx, quest1.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest 1 failed: %v", err)
	}

	// Try to claim second quest — should be rejected
	err = comp.ClaimQuest(ctx, quest2.ID, agent.ID)
	if err == nil {
		t.Fatal("Should reject claim when agent already on a quest")
	}
	if !strings.Contains(err.Error(), "already on a quest") {
		t.Errorf("Expected 'already on a quest' error, got: %v", err)
	}
}

// TestRejectsClaimWhenOnCooldown verifies that an agent on active cooldown
// cannot claim a quest.
func TestRejectsClaimWhenOnCooldown(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "rejectcooldown")
	defer comp.Stop(5 * time.Second)

	// Create agent directly with cooldown status and future CooldownUntil
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	cooldownUntil := time.Now().Add(1 * time.Hour) // Far in the future
	agent := &agentprogression.Agent{
		ID:            agentID,
		Name:          "cooldown-agent",
		Level:         5,
		Tier:          domain.TierApprentice,
		Status:        domain.AgentCooldown,
		CooldownUntil: &cooldownUntil,
	}
	if err := comp.GraphClient().PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Cooldown Rejection Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	err = comp.ClaimQuest(ctx, quest.ID, agentID)
	if err == nil {
		t.Fatal("Should reject claim when agent is on active cooldown")
	}
	if !strings.Contains(err.Error(), "agent on cooldown") {
		t.Errorf("Expected 'agent on cooldown' error, got: %v", err)
	}
}

// TestAllowsClaimWhenCooldownExpired verifies that an agent whose cooldown
// has expired can claim a quest.
func TestAllowsClaimWhenCooldownExpired(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "expiredcooldown")
	defer comp.Stop(5 * time.Second)

	// Create agent with EXPIRED cooldown (CooldownUntil in the past)
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	cooldownUntil := time.Now().Add(-1 * time.Hour) // In the past
	agent := &agentprogression.Agent{
		ID:            agentID,
		Name:          "expired-cooldown-agent",
		Level:         5,
		Tier:          domain.TierApprentice,
		Status:        domain.AgentCooldown,
		CooldownUntil: &cooldownUntil,
	}
	if err := comp.GraphClient().PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Expired Cooldown Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	err = comp.ClaimQuest(ctx, quest.ID, agentID)
	if err != nil {
		t.Fatalf("Should allow claim when cooldown is expired, got: %v", err)
	}
	agentEntity, err := comp.GraphClient().GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updatedAgent.Status != domain.AgentOnQuest {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, domain.AgentOnQuest)
	}
	if updatedAgent.CurrentQuest == nil {
		t.Error("CurrentQuest should be set after claim")
	}
}

// TestRejectsClaimWhenRetired verifies that a retired agent cannot claim quests.
func TestRejectsClaimWhenRetired(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "rejectretired")
	defer comp.Stop(5 * time.Second)

	// Create agent directly with retired status
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &agentprogression.Agent{
		ID:     agentID,
		Name:   "retired-agent",
		Level:  5,
		Tier:   domain.TierApprentice,
		Status: domain.AgentRetired,
	}
	if err := comp.GraphClient().PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Retired Rejection Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	err = comp.ClaimQuest(ctx, quest.ID, agentID)
	if err == nil {
		t.Fatal("Should reject claim when agent is retired")
	}
	if !strings.Contains(err.Error(), "agent is retired") {
		t.Errorf("Expected 'agent is retired' error, got: %v", err)
	}
}

// TestRejectsClaimWhenInBattle verifies that an agent in battle cannot claim quests.
func TestRejectsClaimWhenInBattle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "rejectinbattle")
	defer comp.Stop(5 * time.Second)

	// Create agent directly with in_battle status
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &agentprogression.Agent{
		ID:     agentID,
		Name:   "battle-agent",
		Level:  5,
		Tier:   domain.TierApprentice,
		Status: domain.AgentInBattle,
	}
	if err := comp.GraphClient().PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	quest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Battle Rejection Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	err = comp.ClaimQuest(ctx, quest.ID, agentID)
	if err == nil {
		t.Fatal("Should reject claim when agent is in battle")
	}
	if !strings.Contains(err.Error(), "agent is in battle") {
		t.Errorf("Expected 'agent is in battle' error, got: %v", err)
	}
}

// =============================================================================
// PDAG-02: DEPENDENCY GATE AND PARTY VISIBILITY TESTS
// =============================================================================

// TestClaimQuest_BlockedByUnmetDependency verifies that claiming quest B before
// quest A (its dependency) is completed returns an "unmet dependencies" error.
func TestClaimQuest_BlockedByUnmetDependency(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "depgate")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "dep-agent", 5)

	// Post chain: questA → questB (B depends on A). Use DifficultyTrivial so that
	// a level-5 agent passes the tier gate — the test exercises the dep gate, not tier.
	diffTrivial := domain.DifficultyTrivial
	chain, err := comp.PostQuestChain(ctx, domain.QuestChainBrief{
		Quests: []domain.QuestChainEntry{
			{Title: "Quest A", Goal: "First in chain", Difficulty: &diffTrivial, DependsOn: []int{}},
			{Title: "Quest B", Goal: "Depends on A", Difficulty: &diffTrivial, DependsOn: []int{0}},
		},
	})
	if err != nil {
		t.Fatalf("PostQuestChain failed: %v", err)
	}
	questA := chain[0]
	questB := chain[1]

	// Attempt to claim B before A is completed — must be rejected.
	err = comp.ClaimQuest(ctx, questB.ID, agent.ID)
	if err == nil {
		t.Fatal("Expected error claiming quest with unmet dependency, got nil")
	}
	if !strings.Contains(err.Error(), "unmet dependencies") {
		t.Errorf("Expected 'unmet dependencies' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), string(questA.ID)) {
		t.Errorf("Expected error to name the blocking quest %s, got: %v", questA.ID, err)
	}
}

// TestClaimQuest_AllowedAfterDependencyCompleted verifies that quest B can be
// claimed once quest A (its dependency) has been completed.
func TestClaimQuest_AllowedAfterDependencyCompleted(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "depgate2")
	defer comp.Stop(5 * time.Second)

	agentA := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "dep-agent-a", 5)
	agentB := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "dep-agent-b", 5)

	diffTrivial := domain.DifficultyTrivial
	chain, err := comp.PostQuestChain(ctx, domain.QuestChainBrief{
		Quests: []domain.QuestChainEntry{
			{Title: "Chain A", Goal: "First quest", Difficulty: &diffTrivial, DependsOn: []int{}},
			{Title: "Chain B", Goal: "Depends on A", Difficulty: &diffTrivial, DependsOn: []int{0}},
		},
	})
	if err != nil {
		t.Fatalf("PostQuestChain failed: %v", err)
	}
	questA := chain[0]
	questB := chain[1]

	// Complete quest A: claim → start → submit (no review required).
	if err := comp.ClaimQuest(ctx, questA.ID, agentA.ID); err != nil {
		t.Fatalf("ClaimQuest A failed: %v", err)
	}
	if err := comp.StartQuest(ctx, questA.ID); err != nil {
		t.Fatalf("StartQuest A failed: %v", err)
	}
	if err := comp.SubmitResult(ctx, questA.ID, "done"); err != nil {
		t.Fatalf("SubmitResult A failed: %v", err)
	}
	completed, _ := comp.GetQuest(ctx, questA.ID)
	if completed.Status != domain.QuestCompleted {
		t.Fatalf("Quest A status = %v, want completed", completed.Status)
	}

	// Now claiming B must succeed.
	if err := comp.ClaimQuest(ctx, questB.ID, agentB.ID); err != nil {
		t.Errorf("ClaimQuest B should succeed after A is completed, got: %v", err)
	}
}

// TestAvailableQuests_ExcludesPartySubQuests verifies that a quest with PartyID
// set is never returned by AvailableQuests, regardless of other filter options.
func TestAvailableQuests_ExcludesPartySubQuests(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "partyvisibility")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "visibility-agent", 5)

	// Post a normal quest that should be visible.
	_, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Public Quest",
		Difficulty: domain.DifficultyTrivial,
	})
	if err != nil {
		t.Fatalf("PostQuest (public) failed: %v", err)
	}

	// Post a party sub-quest by setting PartyID directly on the quest entity —
	// simulating what PostSubQuests does when called from a party context.
	partyID := domain.PartyID(comp.BoardConfig().PartyEntityID(domain.GenerateInstance()))
	partyQuest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Party Sub-Quest",
		Difficulty: domain.DifficultyTrivial,
		PartyID:    &partyID,
	})
	if err != nil {
		t.Fatalf("PostQuest (party) failed: %v", err)
	}

	available, err := comp.AvailableQuests(ctx, agent.ID, QuestFilter{})
	if err != nil {
		t.Fatalf("AvailableQuests failed: %v", err)
	}

	for _, q := range available {
		if q.ID == partyQuest.ID {
			t.Errorf("Party sub-quest %s should not appear in AvailableQuests", partyQuest.ID)
		}
	}

	// The public quest must still appear.
	found := false
	for _, q := range available {
		if q.Title == "Public Quest" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Public quest should appear in AvailableQuests")
	}
}

// TestClaimQuest_RejectsNonMemberOnPartyQuest verifies that an agent who is not
// a member of the owning party cannot claim a party sub-quest via ClaimQuest.
func TestClaimQuest_RejectsNonMemberOnPartyQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "partymember")
	defer comp.Stop(5 * time.Second)

	outsider := createTestAgent(t, comp.GraphClient(), comp.BoardConfig(), "outsider-agent", 5)

	// Build a party entity in KV with no members.
	partyID := domain.PartyID(comp.BoardConfig().PartyEntityID(domain.GenerateInstance()))
	party := &partycoord.Party{
		ID:     partyID,
		Name:   "Test Party",
		Status: domain.PartyActive,
		Lead:   domain.AgentID(comp.BoardConfig().AgentEntityID(domain.GenerateInstance())),
		// Members intentionally empty — outsider is not in this party.
	}
	if err := comp.GraphClient().PutEntityState(ctx, party, "party.lifecycle.created"); err != nil {
		t.Fatalf("Failed to create test party: %v", err)
	}

	// Post a quest owned by that party.
	partyQuest, err := comp.PostQuest(ctx, domain.Quest{
		Title:      "Party-Owned Quest",
		Difficulty: domain.DifficultyTrivial,
		PartyID:    &partyID,
	})
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Non-member claim must be rejected.
	err = comp.ClaimQuest(ctx, partyQuest.ID, outsider.ID)
	if err == nil {
		t.Fatal("Expected error when non-member claims party quest, got nil")
	}
	if !strings.Contains(err.Error(), "not a member") {
		t.Errorf("Expected 'not a member' error, got: %v", err)
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

func createTestAgent(t *testing.T, storage *semdragons.GraphClient, config *domain.BoardConfig, name string, level int) *agentprogression.Agent {
	t.Helper()
	return createTestAgentWithSkills(t, storage, config, name, level, nil)
}

func createTestAgentWithSkills(t *testing.T, storage *semdragons.GraphClient, config *domain.BoardConfig, name string, level int, skills []domain.SkillTag) *agentprogression.Agent {
	t.Helper()

	instance := domain.GenerateInstance()
	agentID := domain.AgentID(config.AgentEntityID(instance))

	agent := &agentprogression.Agent{
		ID:     agentID,
		Name:   name,
		Level:  level,
		Tier:   domain.TierFromLevel(level),
		Status: domain.AgentIdle,
		XP:     0,
	}

	// Set up skill proficiencies
	if len(skills) > 0 {
		agent.SkillProficiencies = make(map[domain.SkillTag]domain.SkillProficiency)
		for _, skill := range skills {
			agent.SkillProficiencies[skill] = domain.SkillProficiency{
				Level: domain.ProficiencyJourneyman,
			}
		}
	}

	ctx := context.Background()
	if err := storage.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	return agent
}
