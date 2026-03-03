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
	"github.com/c360studio/semdragons/processor/agentprogression"
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
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	cooldownUntil := time.Now().Add(1 * time.Hour)
	agent := &agentprogression.Agent{
		ID:            agentID, // domain.AgentID IS semdragons.AgentID (type alias)
		Name:          "cooldown-skip-agent",
		Status:        domain.AgentCooldown,
		Level:         5,
		Tier:          domain.TierApprentice,
		CooldownUntil: &cooldownUntil,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Bypass Purchase API to avoid XP/tier setup — testing cooldown_skip effect, not purchase flow
	inv := comp.GetInventory(agentID)
	inv.SetConsumable("cooldown_skip", 1)

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
	updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	if updatedAgent.Status != domain.AgentIdle {
		t.Errorf("Status = %v, want %v", updatedAgent.Status, domain.AgentIdle)
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
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &agentprogression.Agent{
		ID:     agentID, // domain.AgentID IS semdragons.AgentID (type alias)
		Name:   "idle-agent",
		Status: domain.AgentIdle,
		Level:  3,
		Tier:   domain.TierApprentice,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Bypass Purchase API to avoid XP/tier setup — testing cooldown_skip effect, not purchase flow
	inv := comp.GetInventory(agentID)
	inv.SetConsumable("cooldown_skip", 1)

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
	updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent from entity state")
	}

	if updatedAgent.Status != domain.AgentIdle {
		t.Errorf("Status = %v, want %v (idle agent must stay idle)", updatedAgent.Status, domain.AgentIdle)
	}
	if updatedAgent.CooldownUntil != nil {
		t.Errorf("CooldownUntil should be nil, got %v", updatedAgent.CooldownUntil)
	}
}

// TestInventorySurvivesRestart proves that a fresh agentstore component
// rehydrates inventories from KV on Start(). This is the core restart-survival
// test: purchase on instance A, stop A, start instance B, verify B sees the
// purchased item.
func TestInventorySurvivesRestart(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	boardName := "restart"

	// --- Instance 1: purchase items ---
	comp1 := setupComponent(t, client, boardName)
	gc := semdragons.NewGraphClient(client, comp1.BoardConfig())

	// Create agent with enough XP to buy multiple items.
	// web_search (permanent tool): 50 XP
	// context_expander (rental tool, 10 uses): 200 XP (requires Journeyman)
	// retry_token (consumable): 50 XP
	// Total: 300 XP needed. Use Journeyman tier (level 6) for tier-gated items.
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp1.BoardConfig().AgentEntityID(instance))
	agent := &agentprogression.Agent{
		ID:     agentID,
		Name:   "restart-test-agent",
		Status: domain.AgentIdle,
		Level:  8,
		Tier:   domain.TierJourneyman,
		XP:     1000,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Purchase a permanent tool (web_search: 50 XP)
	owned, err := comp1.Purchase(ctx, agentID, "web_search", 1000, 8, nil)
	if err != nil {
		t.Fatalf("Purchase web_search failed: %v", err)
	}
	if owned == nil {
		t.Fatal("Purchase returned nil OwnedItem")
	}

	// Purchase a rental tool (context_expander: 200 XP, 10 uses)
	owned2, err := comp1.Purchase(ctx, agentID, "context_expander", 950, 8, nil)
	if err != nil {
		t.Fatalf("Purchase context_expander failed: %v", err)
	}
	if owned2 == nil {
		t.Fatal("Purchase context_expander returned nil OwnedItem")
	}

	// Purchase a consumable (retry_token: 50 XP)
	owned3, err := comp1.Purchase(ctx, agentID, "retry_token", 750, 8, nil)
	if err != nil {
		t.Fatalf("Purchase retry_token failed: %v", err)
	}
	if owned3 == nil {
		t.Fatal("Purchase retry_token returned nil OwnedItem")
	}

	// Verify inventory on instance 1
	inv1 := comp1.GetInventory(agentID)
	if !inv1.HasTool("web_search") {
		t.Fatal("instance 1: expected web_search in inventory")
	}
	if !inv1.HasTool("context_expander") {
		t.Fatal("instance 1: expected context_expander in inventory")
	}
	if !inv1.HasConsumable("retry_token") {
		t.Fatal("instance 1: expected retry_token in inventory")
	}
	expectedSpent := int64(50 + 200 + 50) // 300
	if inv1.TotalSpent != expectedSpent {
		t.Errorf("instance 1: TotalSpent = %d, want %d", inv1.TotalSpent, expectedSpent)
	}

	// Stop instance 1
	if err := comp1.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop instance 1 failed: %v", err)
	}

	// --- Instance 2: fresh component, same KV ---
	comp2 := setupComponent(t, client, boardName)
	defer comp2.Stop(5 * time.Second)

	// Verify inventory was rehydrated from KV
	inv2 := comp2.GetInventory(agentID)

	// Permanent tool
	if !inv2.HasTool("web_search") {
		t.Error("instance 2: expected web_search in inventory after restart")
	}

	// Rental tool with uses remaining
	if !inv2.HasTool("context_expander") {
		t.Error("instance 2: expected context_expander in inventory after restart")
	}

	// Consumable
	if !inv2.HasConsumable("retry_token") {
		t.Error("instance 2: expected retry_token consumable after restart")
	}
	if count := inv2.ConsumableCount("retry_token"); count != 1 {
		t.Errorf("instance 2: retry_token count = %d, want 1", count)
	}

	// TotalSpent
	if inv2.TotalSpent != expectedSpent {
		t.Errorf("instance 2: TotalSpent = %d, want %d", inv2.TotalSpent, expectedSpent)
	}

	// ItemName populated from catalog during rehydration
	inv2.mu.RLock()
	if name := inv2.OwnedTools["web_search"].ItemName; name != "Web Search" {
		t.Errorf("instance 2: web_search ItemName = %q, want %q", name, "Web Search")
	}
	inv2.mu.RUnlock()

	// Verify XP cache was also rehydrated
	xp, ok := comp2.AgentXP(agentID)
	if !ok {
		t.Error("instance 2: expected agent in XP cache after restart")
	}
	// 1000 - 50 - 200 - 50 = 700
	if xp != 700 {
		t.Errorf("instance 2: cached XP = %d, want 700", xp)
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
