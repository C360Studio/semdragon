//go:build integration

package bossbattle

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// INTEGRATION TESTS - BossBattle Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/bossbattle/...
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

	// Check for quest-submitted input port
	hasQuestSubmitted := false
	for _, port := range inputs {
		if port.Name == "quest-submitted" {
			hasQuestSubmitted = true
			break
		}
	}
	if !hasQuestSubmitted {
		t.Error("Missing quest-submitted input port")
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "startbattle")
	defer comp.Stop(5 * time.Second)

	// Create a test quest
	quest := &semdragons.Quest{
		ID:          semdragons.QuestID(comp.boardConfig.QuestEntityID("test-quest")),
		Title:       "Test Quest",
		Difficulty:  semdragons.DifficultyModerate,
		BaseXP:      100,
		Constraints: semdragons.QuestConstraints{
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
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
	quest := &semdragons.Quest{
		ID:         semdragons.QuestID(comp.boardConfig.QuestEntityID("test-quest")),
		Title:      "Test Quest",
		Difficulty: semdragons.DifficultyModerate,
		Constraints: semdragons.QuestConstraints{
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
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
	quest := &semdragons.Quest{
		ID:         semdragons.QuestID(comp.boardConfig.QuestEntityID("test-quest")),
		Title:      "Test Quest",
		Difficulty: semdragons.DifficultyModerate,
		Constraints: semdragons.QuestConstraints{
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
