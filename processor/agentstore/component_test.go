//go:build integration

package agentstore

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
// INTEGRATION TESTS - AgentStore Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/agentstore/...
// =============================================================================

// TestCooldownSkipClearsStatus verifies that using a cooldown_skip consumable
// clears the agent's cooldown status and sets them back to idle.
func TestCooldownSkipClearsStatus(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "cooldownskip")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create agent in cooldown status with CooldownUntil in the future
	instance := semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	cooldownUntil := time.Now().Add(1 * time.Hour)
	agent := &semdragons.Agent{
		ID:            agentID, // domain.AgentID IS semdragons.AgentID (type alias)
		Name:          "cooldown-skip-agent",
		Status:        semdragons.AgentCooldown,
		Level:         5,
		Tier:          semdragons.TierApprentice,
		CooldownUntil: &cooldownUntil,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Bypass Purchase API to avoid XP/tier setup — testing cooldown_skip effect, not purchase flow
	inv := comp.GetInventory(agentID)
	inv.Consumables["cooldown_skip"] = 1

	// Use the cooldown_skip consumable
	err := comp.UseConsumable(ctx, agentID, "cooldown_skip", nil)
	if err != nil {
		t.Fatalf("UseConsumable failed: %v", err)
	}

	// Verify consumable was consumed
	if count := inv.Consumables["cooldown_skip"]; count != 0 {
		t.Errorf("cooldown_skip count = %d, want 0 (consumed)", count)
	}

	// Verify active effect was recorded
	effects := comp.GetActiveEffects(agentID)
	if len(effects) != 1 {
		t.Errorf("expected 1 active effect, got %d", len(effects))
	} else if effects[0].Effect.Type != ConsumableCooldownSkip {
		t.Errorf("active effect type = %v, want %v", effects[0].Effect.Type, ConsumableCooldownSkip)
	}

	// Read agent back from KV
	agentEntity, err := gc.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := semdragons.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	if updatedAgent.Status != semdragons.AgentIdle {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, semdragons.AgentIdle)
	}
	if updatedAgent.CooldownUntil != nil {
		t.Errorf("CooldownUntil should be nil, got %v", updatedAgent.CooldownUntil)
	}
}

// TestCooldownSkipWhenNotOnCooldown verifies that using a cooldown_skip consumable
// on an agent who is not in cooldown still consumes the item and records the active
// effect, but does not change the agent's idle status.
func TestCooldownSkipWhenNotOnCooldown(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupComponent(t, client, "skipnoop")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Setup: agent with Status=AgentIdle, no CooldownUntil
	instance := semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &semdragons.Agent{
		ID:    agentID, // domain.AgentID IS semdragons.AgentID (type alias)
		Name:  "idle-agent",
		Status: semdragons.AgentIdle,
		Level: 3,
		Tier:  semdragons.TierApprentice,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Bypass Purchase API to avoid XP/tier setup — testing cooldown_skip effect, not purchase flow
	inv := comp.GetInventory(agentID)
	inv.Consumables["cooldown_skip"] = 1

	// Use the cooldown_skip consumable
	err := comp.UseConsumable(ctx, agentID, "cooldown_skip", nil)
	if err != nil {
		t.Fatalf("UseConsumable failed: %v", err)
	}

	// Verify consumable was consumed
	if count := inv.Consumables["cooldown_skip"]; count != 0 {
		t.Errorf("cooldown_skip count = %d, want 0 (consumed)", count)
	}

	// Verify active effect was recorded
	effects := comp.GetActiveEffects(agentID)
	if len(effects) != 1 {
		t.Errorf("expected 1 active effect, got %d", len(effects))
	} else if effects[0].Effect.Type != ConsumableCooldownSkip {
		t.Errorf("active effect type = %v, want %v", effects[0].Effect.Type, ConsumableCooldownSkip)
	}

	// Read agent back from KV — status must remain idle (no cooldown to clear)
	agentEntity, err := gc.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := semdragons.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	if updatedAgent.Status != semdragons.AgentIdle {
		t.Errorf("Status = %v, want %v (idle agent must stay idle)", updatedAgent.Status, semdragons.AgentIdle)
	}
	if updatedAgent.CooldownUntil != nil {
		t.Errorf("CooldownUntil should be nil, got %v", updatedAgent.CooldownUntil)
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
