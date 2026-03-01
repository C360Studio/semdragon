//go:build integration

package bossbattle

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/questboard"
)

// =============================================================================
// INTEGRATION TESTS - BossBattle Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/bossbattle/...
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

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	// Test Initialize
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists (mirrors main.go startup)
	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Test Meta
	meta := comp.Meta()
	if meta.Name != "bossbattle" {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, "bossbattle")
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

	// Check for quest-state-watch input port (KV watch for quest state changes)
	hasQuestStateWatch := false
	for _, port := range inputs {
		if port.Name == "quest-state-watch" {
			hasQuestStateWatch = true
			break
		}
	}
	if !hasQuestStateWatch {
		t.Error("Missing quest-state-watch input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Error("Should have output ports defined")
	}

	// Check for battle-verdict output port
	hasBattleVerdict := false
	for _, port := range outputs {
		if port.Name == "battle-verdict" {
			hasBattleVerdict = true
			break
		}
	}
	if !hasBattleVerdict {
		t.Error("Missing battle-verdict output port")
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

	// Check battle-specific properties exist
	expectedProps := []string{"default_timeout", "max_concurrent", "auto_start_on_submit"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("Missing property %q in schema", prop)
		}
	}
}

func TestComponent_StartBattle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "startbattle")
	defer comp.Stop(5 * time.Second)

	// Create a test quest
	quest := &questboard.Quest{
		ID:         semdragons.QuestID(comp.boardConfig.QuestEntityID("test-quest")),
		Title:      "Test Quest",
		Difficulty: semdragons.DifficultyModerate,
		BaseXP:     100,
		Constraints: questboard.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   semdragons.ReviewStandard,
		},
	}

	// Start a battle
	battle, err := comp.StartBattle(ctx, quest, map[string]any{"result": "test output"})
	if err != nil {
		t.Fatalf("StartBattle failed: %v", err)
	}

	if battle == nil {
		t.Fatal("Battle should not be nil")
	}

	if battle.ID == "" {
		t.Error("Battle ID should be set")
	}

	if battle.QuestID != quest.ID {
		t.Errorf("Battle.QuestID = %v, want %v", battle.QuestID, quest.ID)
	}

	if battle.Status != semdragons.BattleActive {
		t.Errorf("Battle.Status = %v, want %v", battle.Status, semdragons.BattleActive)
	}

	// Check battle has criteria and judges
	if len(battle.Criteria) == 0 {
		t.Error("Battle should have criteria")
	}
	if len(battle.Judges) == 0 {
		t.Error("Battle should have judges")
	}
}

func TestComponent_ListActiveBattles(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "listactive")
	defer comp.Stop(5 * time.Second)

	// Initially should be empty
	active := comp.ListActiveBattles()
	if len(active) != 0 {
		t.Errorf("Expected 0 active battles initially, got %d", len(active))
	}

	// Start a battle
	quest := &questboard.Quest{
		ID:         semdragons.QuestID(comp.boardConfig.QuestEntityID("test-quest")),
		Title:      "Test Quest",
		Difficulty: semdragons.DifficultyModerate,
		Constraints: questboard.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   semdragons.ReviewHuman, // Human review so it stays active
		},
	}

	_, err := comp.StartBattle(ctx, quest, map[string]any{"result": "test"})
	if err != nil {
		t.Fatalf("StartBattle failed: %v", err)
	}

	// Should have one active battle now
	active = comp.ListActiveBattles()
	if len(active) != 1 {
		t.Errorf("Expected 1 active battle, got %d", len(active))
	}
}

func TestComponent_Stats(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "stats")
	defer comp.Stop(5 * time.Second)

	// Get initial stats
	stats := comp.Stats()
	if stats.Started != 0 {
		t.Errorf("Initial Started = %d, want 0", stats.Started)
	}

	// Start a battle
	quest := &questboard.Quest{
		ID:         semdragons.QuestID(comp.boardConfig.QuestEntityID("test-quest")),
		Title:      "Test Quest",
		Difficulty: semdragons.DifficultyModerate,
		Constraints: questboard.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   semdragons.ReviewStandard,
		},
	}

	_, err := comp.StartBattle(ctx, quest, map[string]any{"result": "test"})
	if err != nil {
		t.Fatalf("StartBattle failed: %v", err)
	}

	// Stats should reflect started battle
	stats = comp.Stats()
	if stats.Started != 1 {
		t.Errorf("Started = %d, want 1", stats.Started)
	}
}

// =============================================================================
// AGENT STATUS LIFECYCLE TESTS
// =============================================================================

// TestBattleStartSetsInBattle verifies that when a battle starts via the
// KV watcher (quest transitions to in_review), the agent is set to in_battle.
func TestBattleStartSetsInBattle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "inbattle")
	defer comp.Stop(5 * time.Second)

	gc := comp.Graph()

	// Create agent
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.boardConfig.AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:     agentID,
		Name:   "battle-agent",
		Status: semdragons.AgentOnQuest,
		Level:  7,
		Tier:   semdragons.TierJourneyman,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Create a quest that requires review and transition it to in_review
	questInstance := semdragons.GenerateInstance()
	questID := semdragons.QuestID(comp.boardConfig.QuestEntityID(questInstance))
	agentIDRef := semdragons.AgentID(agentID)
	quest := &questboard.Quest{
		ID:         questID,
		Title:      "Battle Start Quest",
		Status:     semdragons.QuestInProgress,
		Difficulty: semdragons.DifficultyModerate,
		BaseXP:     100,
		ClaimedBy:  &agentIDRef,
		Constraints: questboard.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   semdragons.ReviewStandard,
		},
	}

	// Write quest as in_progress first (seeds the watcher cache)
	if err := gc.PutEntityState(ctx, quest, "quest.started"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Give the watcher time to cache the in_progress state
	time.Sleep(200 * time.Millisecond)

	// Transition quest to in_review — this triggers battle start via KV watcher
	quest.Status = semdragons.QuestInReview
	quest.Output = map[string]any{"result": "test output"}
	if err := gc.EmitEntityUpdate(ctx, quest, "quest.in_review"); err != nil {
		t.Fatalf("Failed to transition quest to in_review: %v", err)
	}

	// Wait for the reactive handler to start the battle and update agent
	time.Sleep(1 * time.Second)

	// Read agent back from KV
	agentEntity, err := gc.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := semdragons.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	if updatedAgent.Status != semdragons.AgentInBattle {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, semdragons.AgentInBattle)
	}
}

// TestBattleVerdictTransitionsQuest verifies that after a battle completes,
// the quest is transitioned to completed (victory) or failed (defeat).
func TestBattleVerdictTransitionsQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "verdict")
	defer comp.Stop(5 * time.Second)

	gc := comp.Graph()

	// Start a battle directly (not via watcher, for deterministic control)
	quest := &questboard.Quest{
		ID:         semdragons.QuestID(comp.boardConfig.QuestEntityID("verdict-quest")),
		Title:      "Verdict Bridge Quest",
		Difficulty: semdragons.DifficultyModerate,
		BaseXP:     100,
		Status:     semdragons.QuestInReview,
		Constraints: questboard.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   semdragons.ReviewStandard,
		},
	}

	// Persist the quest so it can be read back later
	if err := gc.PutEntityState(ctx, quest, "quest.in_review"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Start battle which will evaluate and bridge verdict → quest
	battle, err := comp.StartBattle(ctx, quest, map[string]any{"result": "test output"})
	if err != nil {
		t.Fatalf("StartBattle failed: %v", err)
	}

	// Wait for async evaluation to complete and bridge to quest
	time.Sleep(2 * time.Second)

	// Read the quest back from KV to check if verdict was bridged
	questEntity, err := gc.GetEntityDirect(ctx, string(quest.ID))
	if err != nil {
		t.Fatalf("GetEntityDirect failed: %v", err)
	}

	// Reconstruct quest from entity state
	var questStatus string
	for _, triple := range questEntity.Triples {
		if triple.Predicate == "quest.status.state" {
			if v, ok := triple.Object.(string); ok {
				questStatus = v
			}
		}
	}

	// Battle should have completed — the evaluator returns a verdict
	// which triggers the bridge to either complete or fail the quest.
	if questStatus != string(semdragons.QuestCompleted) && questStatus != string(semdragons.QuestFailed) {
		t.Errorf("Quest status = %q after battle verdict, want %q or %q",
			questStatus, semdragons.QuestCompleted, semdragons.QuestFailed)
	}

	// Verify battle reached terminal state
	_ = battle // Battle was started, evaluation ran async
	stats := comp.Stats()
	if stats.Completed == 0 {
		t.Error("Battle should have completed")
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
	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}
