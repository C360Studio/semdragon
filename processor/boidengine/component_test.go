//go:build integration

package boidengine

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// INTEGRATION TESTS - BoidEngine Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/boidengine/...
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
	if meta.Name != "boidengine" {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, "boidengine")
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

	// Check for boid-suggestions output port
	hasSuggestions := false
	for _, port := range outputs {
		if port.Name == "boid-suggestions" {
			hasSuggestions = true
			break
		}
	}
	if !hasSuggestions {
		t.Error("Missing boid-suggestions output port")
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

	// Check boid rule properties exist
	expectedProps := []string{"separation_weight", "alignment_weight", "cohesion_weight", "hunger_weight", "affinity_weight", "caution_weight"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("Missing property %q in schema", prop)
		}
	}
}

func TestComponent_UpdateRules(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupComponent(t, client, "rules")
	defer comp.Stop(5 * time.Second)

	// Get initial rules
	rules := comp.GetRules()
	initialSeparation := rules.SeparationWeight

	// Update rules
	newRules := rules
	newRules.SeparationWeight = 2.0
	comp.UpdateRules(newRules)

	// Verify update
	updated := comp.GetRules()
	if updated.SeparationWeight != 2.0 {
		t.Errorf("SeparationWeight = %v, want 2.0", updated.SeparationWeight)
	}
	if updated.SeparationWeight == initialSeparation && initialSeparation != 2.0 {
		t.Error("Rules should have been updated")
	}
}

func TestComponent_ComputeAttractionsNow(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "compute")
	defer comp.Stop(5 * time.Second)

	graph := comp.Graph()

	// Create test agents
	for i := 0; i < 3; i++ {
		instance := semdragons.GenerateInstance()
		agentID := semdragons.AgentID(comp.boardConfig.AgentEntityID(instance))
		agent := &semdragons.Agent{
			ID:     agentID,
			Name:   "test-agent",
			Level:  5,
			Status: semdragons.AgentIdle,
		}
		if err := graph.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
			t.Fatalf("Failed to create agent %d: %v", i, err)
		}
	}

	// Create test quests
	for i := 0; i < 3; i++ {
		instance := semdragons.GenerateInstance()
		quest := &semdragons.Quest{
			ID:         semdragons.QuestID(comp.boardConfig.QuestEntityID(instance)),
			Title:      "Test Quest",
			Difficulty: semdragons.DifficultyTrivial,
			Status:     semdragons.QuestPosted,
		}
		if err := graph.PutEntityState(ctx, quest, "quest.lifecycle.posted"); err != nil {
			t.Fatalf("Failed to create quest %d: %v", i, err)
		}
	}

	// Wait for KV watchers to pick up the data
	time.Sleep(100 * time.Millisecond)

	// Compute attractions
	attractions := comp.ComputeAttractionsNow()

	// Should have some attractions (may be 0 if no matching skills, but shouldn't error)
	t.Logf("Computed %d attractions", len(attractions))
}

func TestComponent_Stats(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupComponent(t, client, "stats")
	defer comp.Stop(5 * time.Second)

	// Get stats
	stats := comp.Stats()

	// Agents and quests tracked should be >= 0
	if stats.AgentsTracked < 0 {
		t.Errorf("AgentsTracked = %d, should be >= 0", stats.AgentsTracked)
	}
	if stats.QuestsTracked < 0 {
		t.Errorf("QuestsTracked = %d, should be >= 0", stats.QuestsTracked)
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
