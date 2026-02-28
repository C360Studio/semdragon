//go:build integration

package semdragons

import (
	"context"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
)

// TestStorePurchaseFlow tests the full purchase flow:
// list items → check affordability → purchase → verify inventory
func TestStorePurchaseFlow(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "store",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	publisher := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, publisher, xpEngine)

	// Initialize catalog with default consumables
	if err := store.InitializeCatalog(ctx); err != nil {
		t.Fatalf("failed to initialize catalog: %v", err)
	}

	// Create an agent with 500 XP
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:        AgentID(config.AgentEntityID(agentInstance)),
		Name:      "TestBuyer",
		Level:     8,
		Tier:      TierJourneyman,
		Status:    AgentIdle,
		XP:        500,
		XPToLevel: xpEngine.XPToNextLevel(8),
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// List items visible to the agent
	items, err := store.ListItems(ctx, agent.ID)
	if err != nil {
		t.Fatalf("ListItems failed: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected some items to be available")
	}

	// Find retry token (50 XP)
	var retryToken *StoreItem
	for _, item := range items {
		if item.ID == "retry_token" {
			retryToken = &item
			break
		}
	}
	if retryToken == nil {
		t.Fatal("retry_token not found in catalog")
	}

	// Check affordability
	canAfford, shortfall, err := store.CanAfford(ctx, agent.ID, "retry_token")
	if err != nil {
		t.Fatalf("CanAfford failed: %v", err)
	}
	if !canAfford {
		t.Errorf("agent should be able to afford retry_token, shortfall: %d", shortfall)
	}

	// Purchase
	owned, err := store.Purchase(ctx, agent.ID, "retry_token")
	if err != nil {
		t.Fatalf("Purchase failed: %v", err)
	}
	if owned == nil {
		t.Fatal("expected owned item")
	}
	if owned.ItemID != "retry_token" {
		t.Errorf("expected retry_token, got %s", owned.ItemID)
	}
	if owned.XPSpent != 50 {
		t.Errorf("expected 50 XP spent, got %d", owned.XPSpent)
	}

	// Verify inventory
	inv, err := store.GetInventory(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetInventory failed: %v", err)
	}
	if !inv.HasConsumable("retry_token") {
		t.Error("inventory should have retry_token")
	}
	if inv.ConsumableCount("retry_token") != 1 {
		t.Errorf("expected 1 retry_token, got %d", inv.ConsumableCount("retry_token"))
	}
	if inv.TotalSpent != 50 {
		t.Errorf("expected 50 total spent, got %d", inv.TotalSpent)
	}

	// Verify agent XP was deducted
	updatedAgent, err := storage.GetAgent(ctx, agentInstance)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if updatedAgent.XP != 450 {
		t.Errorf("expected agent to have 450 XP after purchase, got %d", updatedAgent.XP)
	}
}

// TestStoreTierGating tests that items are filtered by agent tier
func TestStoreTierGating(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "tiergating",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, nil, xpEngine)

	// Add items with different tier requirements
	store.AddItem(ctx, NewStoreItem("apprentice_item", "Apprentice Tool").
		AsTool("tool_basic").
		Permanent().
		Cost(25).
		RequireTier(TierApprentice).
		Build())

	store.AddItem(ctx, NewStoreItem("expert_item", "Expert Tool").
		AsTool("tool_advanced").
		Permanent().
		Cost(200).
		RequireTier(TierExpert).
		Build())

	store.AddItem(ctx, NewStoreItem("master_item", "Master Tool").
		AsTool("tool_master").
		Permanent().
		Cost(500).
		RequireTier(TierMaster).
		Build())

	// Create an Apprentice agent
	apprenticeInstance := GenerateInstance()
	apprentice := &Agent{
		ID:     AgentID(config.AgentEntityID(apprenticeInstance)),
		Name:   "Newbie",
		Level:  3,
		Tier:   TierApprentice,
		Status: AgentIdle,
		XP:     1000,
	}
	if err := storage.PutAgent(ctx, apprenticeInstance, apprentice); err != nil {
		t.Fatalf("failed to create apprentice: %v", err)
	}

	// Create an Expert agent
	expertInstance := GenerateInstance()
	expert := &Agent{
		ID:     AgentID(config.AgentEntityID(expertInstance)),
		Name:   "Veteran",
		Level:  12,
		Tier:   TierExpert,
		Status: AgentIdle,
		XP:     1000,
	}
	if err := storage.PutAgent(ctx, expertInstance, expert); err != nil {
		t.Fatalf("failed to create expert: %v", err)
	}

	// Apprentice should only see apprentice item
	apprenticeItems, err := store.ListItems(ctx, apprentice.ID)
	if err != nil {
		t.Fatalf("ListItems for apprentice failed: %v", err)
	}
	if len(apprenticeItems) != 1 {
		t.Errorf("apprentice should see 1 item, got %d", len(apprenticeItems))
	}
	if len(apprenticeItems) > 0 && apprenticeItems[0].ID != "apprentice_item" {
		t.Errorf("apprentice should only see apprentice_item, got %s", apprenticeItems[0].ID)
	}

	// Expert should see apprentice and expert items (but not master)
	expertItems, err := store.ListItems(ctx, expert.ID)
	if err != nil {
		t.Fatalf("ListItems for expert failed: %v", err)
	}
	if len(expertItems) != 2 {
		t.Errorf("expert should see 2 items, got %d", len(expertItems))
	}

	// Apprentice cannot purchase expert item
	_, purchaseErr := store.Purchase(ctx, apprentice.ID, "expert_item")
	if purchaseErr == nil {
		t.Error("apprentice should not be able to purchase expert_item")
	}
}

// TestStoreInsufficientXP tests that purchase fails with insufficient XP
func TestStoreInsufficientXP(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "insufficientxp",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, nil, xpEngine)
	store.InitializeCatalog(ctx)

	// Create an agent with only 10 XP
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Name:   "Broke",
		Level:  5,
		Tier:   TierApprentice,
		Status: AgentIdle,
		XP:     10,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Check affordability
	canAfford, shortfall, err := store.CanAfford(ctx, agent.ID, "retry_token")
	if err != nil {
		t.Fatalf("CanAfford failed: %v", err)
	}
	if canAfford {
		t.Error("agent should not be able to afford retry_token with only 10 XP")
	}
	if shortfall != 40 {
		t.Errorf("expected shortfall of 40, got %d", shortfall)
	}

	// Purchase should fail
	_, purchaseErr := store.Purchase(ctx, agent.ID, "retry_token")
	if purchaseErr == nil {
		t.Error("purchase should fail with insufficient XP")
	}
}

// TestStoreConsumableUsage tests using consumables
func TestStoreConsumableUsage(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "consumable",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	publisher := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, publisher, xpEngine)
	store.InitializeCatalog(ctx)

	// Create an agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Name:   "Consumer",
		Level:  8,
		Tier:   TierJourneyman,
		Status: AgentIdle,
		XP:     500,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Buy two xp_boost consumables
	for range 2 {
		_, err := store.Purchase(ctx, agent.ID, "xp_boost")
		if err != nil {
			t.Fatalf("Purchase failed: %v", err)
		}
	}

	// Verify we have 2
	inv, _ := store.GetInventory(ctx, agent.ID)
	if inv.ConsumableCount("xp_boost") != 2 {
		t.Errorf("expected 2 xp_boost, got %d", inv.ConsumableCount("xp_boost"))
	}

	// Use one
	if err := store.UseConsumable(ctx, agent.ID, "xp_boost", nil); err != nil {
		t.Fatalf("UseConsumable failed: %v", err)
	}

	// Verify we have 1 left
	inv, _ = store.GetInventory(ctx, agent.ID)
	if inv.ConsumableCount("xp_boost") != 1 {
		t.Errorf("expected 1 xp_boost remaining, got %d", inv.ConsumableCount("xp_boost"))
	}

	// Verify active effect exists
	effects, err := store.GetActiveEffects(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetActiveEffects failed: %v", err)
	}
	if len(effects) != 1 {
		t.Errorf("expected 1 active effect, got %d", len(effects))
	}
	if effects[0].Effect.Type != ConsumableXPBoost {
		t.Errorf("expected xp_boost effect, got %s", effects[0].Effect.Type)
	}
	if effects[0].Effect.Magnitude != 2.0 {
		t.Errorf("expected magnitude 2.0, got %f", effects[0].Effect.Magnitude)
	}
}

// TestStoreRentalTracking tests rental item use tracking
func TestStoreRentalTracking(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "rental",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, nil, xpEngine)

	// Add a rental tool
	store.AddItem(ctx, NewStoreItem("premium_tool", "Premium Tool").
		AsTool("tool_premium").
		Rental(3). // 3 uses
		Cost(100).
		RequireTier(TierApprentice).
		Build())

	// Create an agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Name:   "Renter",
		Level:  5,
		Tier:   TierApprentice,
		Status: AgentIdle,
		XP:     500,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Purchase rental tool
	owned, err := store.Purchase(ctx, agent.ID, "premium_tool")
	if err != nil {
		t.Fatalf("Purchase failed: %v", err)
	}
	if owned.PurchaseType != PurchaseRental {
		t.Error("expected rental purchase type")
	}
	if owned.UsesRemaining != 3 {
		t.Errorf("expected 3 uses remaining, got %d", owned.UsesRemaining)
	}

	// Verify we have the tool
	hasTool, err := store.HasTool(ctx, agent.ID, "tool_premium")
	if err != nil {
		t.Fatalf("HasTool failed: %v", err)
	}
	if !hasTool {
		t.Error("agent should have tool_premium")
	}
}

// TestStoreMultiplePurchases tests buying multiple consumables
func TestStoreMultiplePurchases(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "multipurchase",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, nil, xpEngine)
	store.InitializeCatalog(ctx)

	// Create an agent with lots of XP
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Name:   "BigSpender",
		Level:  10,
		Tier:   TierJourneyman,
		Status: AgentIdle,
		XP:     1000,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Buy 5 retry tokens
	for range 5 {
		_, err := store.Purchase(ctx, agent.ID, "retry_token")
		if err != nil {
			t.Fatalf("Purchase failed: %v", err)
		}
	}

	// Verify inventory
	inv, _ := store.GetInventory(ctx, agent.ID)
	if inv.ConsumableCount("retry_token") != 5 {
		t.Errorf("expected 5 retry_tokens, got %d", inv.ConsumableCount("retry_token"))
	}
	if inv.TotalSpent != 250 { // 5 * 50 XP
		t.Errorf("expected 250 total spent, got %d", inv.TotalSpent)
	}

	// Verify agent XP
	updatedAgent, _ := storage.GetAgent(ctx, agentInstance)
	if updatedAgent.XP != 750 { // 1000 - 250
		t.Errorf("expected 750 XP remaining, got %d", updatedAgent.XP)
	}
}

// TestStoreEffectConsumption tests consuming active effects
func TestStoreEffectConsumption(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "effectconsume",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	publisher := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, publisher, xpEngine)

	// Add insight_scroll with 3-quest duration
	store.AddItem(ctx, NewStoreItem("insight_scroll", "Insight Scroll").
		AsConsumable(ConsumableEffect{
			Type:     ConsumableInsightScroll,
			Duration: 3,
		}).
		Cost(50).
		Build())

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Name:   "Seer",
		Level:  5,
		Tier:   TierApprentice,
		Status: AgentIdle,
		XP:     200,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Purchase and activate
	store.Purchase(ctx, agent.ID, "insight_scroll")
	store.UseConsumable(ctx, agent.ID, "insight_scroll", nil)

	// Verify 3 quests remaining
	effects, _ := store.GetActiveEffects(ctx, agent.ID)
	if len(effects) != 1 || effects[0].QuestsRemaining != 3 {
		t.Errorf("expected 1 effect with 3 quests remaining")
	}

	// Consume effect (simulating quest completion)
	store.ConsumeEffect(ctx, agent.ID, ConsumableInsightScroll)
	effects, _ = store.GetActiveEffects(ctx, agent.ID)
	if len(effects) != 1 || effects[0].QuestsRemaining != 2 {
		t.Errorf("expected 1 effect with 2 quests remaining")
	}

	// Consume twice more
	store.ConsumeEffect(ctx, agent.ID, ConsumableInsightScroll)
	store.ConsumeEffect(ctx, agent.ID, ConsumableInsightScroll)

	// Effect should be removed
	effects, _ = store.GetActiveEffects(ctx, agent.ID)
	if len(effects) != 0 {
		t.Errorf("expected 0 effects after exhaustion, got %d", len(effects))
	}
}

// TestStoreOutOfStock tests that out of stock items cannot be purchased
func TestStoreOutOfStock(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "outofstock",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, nil, xpEngine)

	// Add an out-of-stock item
	store.AddItem(ctx, NewStoreItem("rare_item", "Rare Item").
		AsConsumable(ConsumableEffect{Type: ConsumableRetryToken}).
		Cost(100).
		OutOfStock().
		Build())

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Name:   "Buyer",
		Level:  5,
		Tier:   TierApprentice,
		Status: AgentIdle,
		XP:     500,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Item should not appear in list
	items, _ := store.ListItems(ctx, agent.ID)
	for _, item := range items {
		if item.ID == "rare_item" {
			t.Error("out of stock item should not appear in list")
		}
	}

	// Purchase should fail
	_, purchaseErr := store.Purchase(ctx, agent.ID, "rare_item")
	if purchaseErr == nil {
		t.Error("should not be able to purchase out of stock item")
	}
}

// TestStoreDuplicatePermanentToolPurchase tests that duplicate permanent tool purchases fail
func TestStoreDuplicatePermanentToolPurchase(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "duplicate",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	xpEngine := NewDefaultXPEngine()
	store := NewDefaultStore(storage, nil, xpEngine)

	// Add a permanent tool
	store.AddItem(ctx, NewStoreItem("unique_tool", "Unique Tool").
		AsTool("tool_unique").
		Permanent().
		Cost(100).
		RequireTier(TierApprentice).
		Build())

	// Create an agent with lots of XP
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Name:   "Collector",
		Level:  5,
		Tier:   TierApprentice,
		Status: AgentIdle,
		XP:     500,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// First purchase should succeed
	_, err = store.Purchase(ctx, agent.ID, "unique_tool")
	if err != nil {
		t.Fatalf("first purchase should succeed: %v", err)
	}

	// Second purchase should fail
	_, err = store.Purchase(ctx, agent.ID, "unique_tool")
	if err == nil {
		t.Error("second purchase of same permanent tool should fail")
	}

	// Verify agent still has 400 XP (only charged once)
	updatedAgent, _ := storage.GetAgent(ctx, agentInstance)
	if updatedAgent.XP != 400 {
		t.Errorf("expected 400 XP (charged once), got %d", updatedAgent.XP)
	}
}

// TestXPEngineSpendXP tests the SpendXP method on XPEngine
func TestXPEngineSpendXP(t *testing.T) {
	xpEngine := NewDefaultXPEngine()

	agent := &Agent{
		ID:    "test-agent",
		Level: 5,
		XP:    100,
	}

	// Can spend check
	if !xpEngine.CanSpend(agent, 50) {
		t.Error("agent should be able to spend 50 XP")
	}
	if xpEngine.CanSpend(agent, 150) {
		t.Error("agent should not be able to spend 150 XP")
	}

	// Spend XP
	err := xpEngine.SpendXP(agent, 30, "test purchase")
	if err != nil {
		t.Fatalf("SpendXP failed: %v", err)
	}
	if agent.XP != 70 {
		t.Errorf("expected 70 XP after spending 30, got %d", agent.XP)
	}

	// Level should not change
	if agent.Level != 5 {
		t.Errorf("level should not change after spending XP, got %d", agent.Level)
	}

	// Insufficient XP error
	err = xpEngine.SpendXP(agent, 100, "expensive item")
	if err == nil {
		t.Error("expected error when spending more XP than available")
	}
	insufficientErr, ok := err.(*InsufficientXPError)
	if !ok {
		t.Errorf("expected InsufficientXPError, got %T", err)
	} else {
		if insufficientErr.Have != 70 {
			t.Errorf("expected Have=70, got %d", insufficientErr.Have)
		}
		if insufficientErr.Need != 100 {
			t.Errorf("expected Need=100, got %d", insufficientErr.Need)
		}
	}
}
