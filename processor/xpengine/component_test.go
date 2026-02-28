//go:build integration

package xpengine

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// INTEGRATION TESTS - XPEngine Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/xpengine/...
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
	if meta.Name != "xpengine" {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, "xpengine")
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

	// Check for quest-completed input port
	hasQuestCompleted := false
	for _, port := range inputs {
		if port.Name == "quest-completed" {
			hasQuestCompleted = true
			break
		}
	}
	if !hasQuestCompleted {
		t.Error("Missing quest-completed input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Error("Should have output ports defined")
	}

	// Check for agent-xp output port
	hasAgentXP := false
	for _, port := range outputs {
		if port.Name == "agent-xp" {
			hasAgentXP = true
			break
		}
	}
	if !hasAgentXP {
		t.Error("Missing agent-xp output port")
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

	// Check XP-specific properties exist
	expectedProps := []string{"quality_multiplier", "speed_multiplier", "streak_multiplier", "retry_penalty_rate"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("Missing property %q in schema", prop)
		}
	}
}

func TestComponent_ProcessFailure(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "failure")
	defer comp.Stop(5 * time.Second)

	// Create a test agent
	agent := createTestAgent(t, comp.Storage(), comp.boardConfig, "fail-agent", 5)

	// Create a test quest
	quest := semdragons.Quest{
		ID:          semdragons.QuestID(comp.boardConfig.QuestEntityID("test-quest")),
		Title:       "Test Quest",
		Difficulty:  semdragons.DifficultyModerate,
		BaseXP:      100,
		Attempts:    1,
		MaxAttempts: 3,
	}

	// Process failure
	penalty, err := comp.ProcessFailure(ctx, agent.ID, quest, semdragons.FailureSoft)
	if err != nil {
		t.Fatalf("ProcessFailure failed: %v", err)
	}

	if penalty == nil {
		t.Fatal("Penalty should not be nil")
	}

	if penalty.XPLost <= 0 {
		t.Error("XPLost should be positive for a failure")
	}
}

func TestComponent_CalculateXP(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testClient.Client

	comp := setupComponent(t, client, "calcxp")
	defer comp.Stop(5 * time.Second)

	// Create XP context
	xpCtx := semdragons.XPContext{
		Quest: semdragons.Quest{
			Difficulty: semdragons.DifficultyModerate,
			BaseXP:     100,
		},
		Agent: semdragons.Agent{
			Level: 5,
		},
		BattleResult: semdragons.BattleVerdict{
			Passed:       true,
			QualityScore: 0.9,
		},
		Duration: 30 * time.Minute,
		Streak:   3,
	}

	// Calculate XP
	award := comp.CalculateXP(xpCtx)

	if award.BaseXP != 100 {
		t.Errorf("BaseXP = %d, want 100", award.BaseXP)
	}
	if award.TotalXP <= 0 {
		t.Error("TotalXP should be positive")
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

	ctx := context.Background()
	if err := storage.PutAgent(ctx, instance, agent); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	return agent
}
