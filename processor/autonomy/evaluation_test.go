package autonomy

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semdragons/processor/dmapproval"
	"github.com/c360studio/semdragons/processor/guildformation"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// TEST REGISTRY HELPERS
// =============================================================================
// Unit tests use component.NewRegistry + RegisterInstance to inject sibling
// components without starting them. Zero-value agentstore.Component and
// guildformation.Component have safe no-op semantics for SeedCatalog/ListItems
// and GetAgentGuild/ListGuilds respectively.
// =============================================================================

// newTestRegistry builds a minimal *component.Registry with pre-registered zero-value
// agentstore and guildformation components. The components are not started — only
// methods backed by sync.Map (SeedCatalog, ListItems, GetAgentGuild, ListGuilds) are
// safe to call on them. Execute paths that call Purchase/UseConsumable/JoinGuild are
// not exercised in unit tests.
func newTestRegistry(t *testing.T, store *agentstore.Component, guilds *guildformation.Component, approval *dmapproval.Component) *component.Registry {
	t.Helper()
	reg := component.NewRegistry()
	if store != nil {
		if err := reg.RegisterInstance(agentstore.ComponentName, store); err != nil {
			t.Fatalf("failed to register agentstore in test registry: %v", err)
		}
	}
	if guilds != nil {
		if err := reg.RegisterInstance(guildformation.ComponentName, guilds); err != nil {
			t.Fatalf("failed to register guildformation in test registry: %v", err)
		}
	}
	if approval != nil {
		if err := reg.RegisterInstance(dmapproval.ComponentName, approval); err != nil {
			t.Fatalf("failed to register dmapproval in test registry: %v", err)
		}
	}
	return reg
}

// =============================================================================
// UNIT TESTS - Action matrix, interval mapping, config, backoff
// =============================================================================
// No Docker required. Run with: go test ./processor/autonomy/...
// =============================================================================

// newTestComponent creates a minimal Component for unit tests (no NATS/graph).
func newTestComponent() *Component {
	cfg := DefaultConfig()
	return &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}
}

func TestActionsForState_Idle(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(domain.AgentIdle)
	want := []string{"review_guild_applications", "claim_quest", "join_guild", "create_guild", "use_consumable", "shop"}

	if len(actions) != len(want) {
		t.Fatalf("idle actions: got %d, want %d", len(actions), len(want))
	}

	for i, act := range actions {
		if act.name != want[i] {
			t.Errorf("idle action[%d] = %q, want %q", i, act.name, want[i])
		}
	}
}

func TestActionsForState_OnQuest(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(domain.AgentOnQuest)
	want := []string{"shop_strategic", "use_consumable"}

	if len(actions) != len(want) {
		t.Fatalf("on_quest actions: got %d, want %d", len(actions), len(want))
	}

	for i, act := range actions {
		if act.name != want[i] {
			t.Errorf("on_quest action[%d] = %q, want %q", i, act.name, want[i])
		}
	}
}

func TestActionsForState_InBattle(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(domain.AgentInBattle)
	want := []string{"use_consumable"}

	if len(actions) != len(want) {
		t.Fatalf("in_battle actions: got %d, want %d", len(actions), len(want))
	}

	if actions[0].name != want[0] {
		t.Errorf("in_battle action[0] = %q, want %q", actions[0].name, want[0])
	}
}

func TestActionsForState_Cooldown(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(domain.AgentCooldown)
	want := []string{"use_cooldown_skip", "review_guild_applications", "join_guild", "shop"}

	if len(actions) != len(want) {
		t.Fatalf("cooldown actions: got %d, want %d", len(actions), len(want))
	}

	for i, act := range actions {
		if act.name != want[i] {
			t.Errorf("cooldown action[%d] = %q, want %q", i, act.name, want[i])
		}
	}
}

func TestActionsForState_Retired(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(domain.AgentRetired)
	if actions != nil {
		t.Errorf("retired actions: got %v, want nil", actions)
	}
}

func TestActions_ReturnFalseWithoutStore(t *testing.T) {
	// When store is nil (newTestComponent has no store), all shopping and
	// consumable actions must return shouldExecute=false — safe degradation.
	c := newTestComponent()
	actions := []action{
		c.shopAction(),
		c.shopStrategicAction(),
		c.useConsumableAction(),
		c.useCooldownSkipAction(),
		c.joinGuildAction(), // nil guilds guard
	}

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.nilstore",
		Status: domain.AgentIdle,
		Level:  5,
		XP:     9999,
		Consumables: map[string]int{
			"cooldown_skip":  1,
			"xp_boost":       1,
			"quality_shield": 1,
		},
	}
	tracker := &agentTracker{
		agent: agent,
		suggestions: []boidengine.SuggestedClaim{
			{QuestID: "test.local.game.board1.quest.q1", Score: 1.0},
		},
	}

	for _, act := range actions {
		if act.shouldExecute(agent, tracker) {
			t.Errorf("action %q shouldExecute returned true with nil store", act.name)
		}
	}
}

func TestClaimQuestAction_ShouldExecute_WithSuggestions(t *testing.T) {
	c := newTestComponent()
	act := c.claimQuestAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.claim1",
		Status: domain.AgentIdle,
		Level:  5,
	}
	tracker := &agentTracker{
		agent: agent,
		suggestions: []boidengine.SuggestedClaim{
			{QuestID: "test.local.game.board1.quest.q1", Score: 3.0},
		},
	}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should return true when idle with suggestions")
	}
}

func TestClaimQuestAction_ShouldExecute_NoSuggestions(t *testing.T) {
	c := newTestComponent()
	act := c.claimQuestAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.claim2",
		Status: domain.AgentIdle,
		Level:  5,
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should return false when no suggestions")
	}
}

func TestClaimQuestAction_ShouldExecute_NotIdle(t *testing.T) {
	c := newTestComponent()
	act := c.claimQuestAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.claim3",
		Status: domain.AgentOnQuest,
		Level:  5,
	}
	tracker := &agentTracker{
		agent: agent,
		suggestions: []boidengine.SuggestedClaim{
			{QuestID: "test.local.game.board1.quest.q1", Score: 3.0},
		},
	}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should return false when not idle")
	}
}

// =============================================================================
// SHOPPING ACTION TESTS
// =============================================================================

// newTestComponentWithStore creates a Component backed by a registry that contains a
// zero-value agentstore.Component. Only SeedCatalog() and ListItems() are safe to
// call on the store (they use sync.Map zero-value semantics). Execute paths that call
// Purchase/UseConsumable are not exercised in unit tests.
func newTestComponentWithStore(t *testing.T) *Component {
	t.Helper()
	cfg := DefaultConfig()
	store := &agentstore.Component{}
	reg := newTestRegistry(t, store, nil, nil)
	c := &Component{
		config: &cfg,
		deps:   component.Dependencies{ComponentRegistry: reg},
		trackers: make(map[string]*agentTracker),
	}
	return c
}

func TestShopAction_ShouldExecute_IdleWithSurplus(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.shopAction()

	agent := &agentprogression.Agent{
		ID:        "test.local.game.board1.agent.shop1",
		Status:    domain.AgentIdle,
		XP:        500,
		XPToLevel: 400, // surplus = 100, threshold = 50
	}
	tracker := &agentTracker{agent: agent}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be true when idle with XP surplus above threshold")
	}
}

func TestShopAction_ShouldExecute_IdleNoSurplus(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.shopAction()

	agent := &agentprogression.Agent{
		ID:        "test.local.game.board1.agent.shop2",
		Status:    domain.AgentIdle,
		XP:        300,
		XPToLevel: 400, // surplus = -100 (negative)
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when XP < XPToLevel")
	}
}

func TestShopAction_ShouldExecute_CooldownWithXP(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.shopAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.shop3",
		Status: domain.AgentCooldown,
		XP:     100, // above CooldownShopMinXP (25)
	}
	tracker := &agentTracker{agent: agent}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be true when in cooldown with XP above min")
	}
}

func TestShopAction_ShouldExecute_CooldownLowXP(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.shopAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.shop4",
		Status: domain.AgentCooldown,
		XP:     10, // below CooldownShopMinXP (25)
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when cooldown XP below minimum")
	}
}

func TestShopAction_ShouldExecute_NilStore(t *testing.T) {
	c := newTestComponent() // no store
	act := c.shopAction()

	agent := &agentprogression.Agent{
		ID:        "test.local.game.board1.agent.shop5",
		Status:    domain.AgentIdle,
		XP:        9999,
		XPToLevel: 100,
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when store is nil")
	}
}

func TestShopAction_ShouldExecute_OnQuestIsFalse(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.shopAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.shop6",
		Status: domain.AgentOnQuest,
		XP:     9999,
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shopAction should not execute when on_quest (use shopStrategic instead)")
	}
}

// =============================================================================
// STRATEGIC SHOPPING TESTS
// =============================================================================

func TestShopStrategic_ShouldExecute_OnQuestMissingShield(t *testing.T) {
	c := newTestComponentWithStore(t)
	// Seed the catalog so ListItems returns items
	c.resolveStore().SeedCatalog(agentstore.DefaultCatalog())
	act := c.shopStrategicAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.strat1",
		Status: domain.AgentOnQuest,
		XP:     500,
		Tier:   domain.TierJourneyman, // quality_shield requires Journeyman
	}
	tracker := &agentTracker{agent: agent}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be true when on quest, missing quality_shield, can afford")
	}
}

func TestShopStrategic_ShouldExecute_HasBoth(t *testing.T) {
	c := newTestComponentWithStore(t)
	// Catalog not seeded -- shouldExecute short-circuits on ownership check
	// before reaching ListItems. Both consumables are already owned.
	act := c.shopStrategicAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.strat2",
		Status: domain.AgentOnQuest,
		XP:     500,
		Tier:   domain.TierJourneyman,
		Consumables: map[string]int{
			string(agentstore.ConsumableQualityShield): 1,
			string(agentstore.ConsumableXPBoost):       1,
		},
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when agent already has both consumables")
	}
}

func TestShopStrategic_ShouldExecute_NotOnQuest(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.shopStrategicAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.strat3",
		Status: domain.AgentIdle,
		XP:     500,
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when not on quest")
	}
}

// =============================================================================
// USE CONSUMABLE TESTS
// =============================================================================

func TestUseConsumable_ShouldExecute_IdleWithBoostAndSuggestions(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useConsumableAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.use1",
		Status: domain.AgentIdle,
		Consumables: map[string]int{
			string(agentstore.ConsumableXPBoost): 1,
		},
	}
	tracker := &agentTracker{
		agent: agent,
		suggestions: []boidengine.SuggestedClaim{
			{QuestID: "test.local.game.board1.quest.q1", Score: 2.0},
		},
	}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be true when idle with xp_boost and suggestions")
	}
}

func TestUseConsumable_ShouldExecute_IdleNoConsumables(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useConsumableAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.use2",
		Status: domain.AgentIdle,
	}
	tracker := &agentTracker{
		agent: agent,
		suggestions: []boidengine.SuggestedClaim{
			{QuestID: "test.local.game.board1.quest.q1", Score: 2.0},
		},
	}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false with no consumables")
	}
}

func TestUseConsumable_ShouldExecute_IdleNoSuggestions(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useConsumableAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.use3",
		Status: domain.AgentIdle,
		Consumables: map[string]int{
			string(agentstore.ConsumableXPBoost): 1,
		},
	}
	tracker := &agentTracker{agent: agent} // no suggestions

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when idle with no suggestions (nothing to boost)")
	}
}

func TestUseConsumable_ShouldExecute_OnQuestWithBoost(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useConsumableAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.use4",
		Status: domain.AgentOnQuest,
		Consumables: map[string]int{
			string(agentstore.ConsumableXPBoost): 1,
		},
	}
	tracker := &agentTracker{agent: agent}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be true when on quest with xp_boost")
	}
}

func TestUseConsumable_ShouldExecute_OnQuestBoostAlreadyActive(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useConsumableAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.use5",
		Status: domain.AgentOnQuest,
		Consumables: map[string]int{
			string(agentstore.ConsumableXPBoost): 1,
		},
		ActiveEffects: []agentprogression.AgentEffect{
			{EffectType: string(agentstore.ConsumableXPBoost), QuestsRemaining: 1},
		},
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when xp_boost is already active")
	}
}

func TestUseConsumable_ShouldExecute_InBattleWithShield(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useConsumableAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.use6",
		Status: domain.AgentInBattle,
		Consumables: map[string]int{
			string(agentstore.ConsumableQualityShield): 1,
		},
	}
	tracker := &agentTracker{agent: agent}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be true when in battle with quality_shield")
	}
}

func TestUseConsumable_ShouldExecute_InBattleShieldAlreadyActive(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useConsumableAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.use7",
		Status: domain.AgentInBattle,
		Consumables: map[string]int{
			string(agentstore.ConsumableQualityShield): 1,
		},
		ActiveEffects: []agentprogression.AgentEffect{
			{EffectType: string(agentstore.ConsumableQualityShield), QuestsRemaining: 1},
		},
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when quality_shield is already active")
	}
}

// =============================================================================
// COOLDOWN SKIP TESTS
// =============================================================================

func TestUseCooldownSkip_ShouldExecute_CooldownWithSkip(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useCooldownSkipAction()

	future := time.Now().Add(5 * time.Minute) // 5min remaining > 30s threshold
	agent := &agentprogression.Agent{
		ID:            "test.local.game.board1.agent.skip1",
		Status:        domain.AgentCooldown,
		CooldownUntil: &future,
		Consumables: map[string]int{
			string(agentstore.ConsumableCooldownSkip): 1,
		},
	}
	tracker := &agentTracker{agent: agent}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be true when in cooldown with skip consumable and enough remaining time")
	}
}

func TestUseCooldownSkip_ShouldExecute_AlmostDone(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useCooldownSkipAction()

	soon := time.Now().Add(5 * time.Second) // 5s remaining < 30s threshold
	agent := &agentprogression.Agent{
		ID:            "test.local.game.board1.agent.skip2",
		Status:        domain.AgentCooldown,
		CooldownUntil: &soon,
		Consumables: map[string]int{
			string(agentstore.ConsumableCooldownSkip): 1,
		},
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when remaining cooldown below threshold")
	}
}

func TestUseCooldownSkip_ShouldExecute_NoConsumable(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useCooldownSkipAction()

	future := time.Now().Add(5 * time.Minute)
	agent := &agentprogression.Agent{
		ID:            "test.local.game.board1.agent.skip3",
		Status:        domain.AgentCooldown,
		CooldownUntil: &future,
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when agent has no cooldown_skip consumable")
	}
}

func TestUseCooldownSkip_ShouldExecute_NilCooldownUntil(t *testing.T) {
	c := newTestComponentWithStore(t)
	act := c.useCooldownSkipAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.skip4",
		Status: domain.AgentCooldown,
		Consumables: map[string]int{
			string(agentstore.ConsumableCooldownSkip): 1,
		},
		// CooldownUntil not set
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when CooldownUntil is nil")
	}
}

// =============================================================================
// HELPER FUNCTION TESTS
// =============================================================================

func TestHasActiveEffect_Found(t *testing.T) {
	agent := &agentprogression.Agent{
		ActiveEffects: []agentprogression.AgentEffect{
			{EffectType: "xp_boost", QuestsRemaining: 1},
		},
	}
	if !hasActiveEffect(agent, "xp_boost") {
		t.Error("hasActiveEffect should return true when effect exists with quests remaining")
	}
}

func TestHasActiveEffect_Expired(t *testing.T) {
	agent := &agentprogression.Agent{
		ActiveEffects: []agentprogression.AgentEffect{
			{EffectType: "xp_boost", QuestsRemaining: 0},
		},
	}
	if hasActiveEffect(agent, "xp_boost") {
		t.Error("hasActiveEffect should return false when QuestsRemaining is 0")
	}
}

func TestHasActiveEffect_NotPresent(t *testing.T) {
	agent := &agentprogression.Agent{}
	if hasActiveEffect(agent, "xp_boost") {
		t.Error("hasActiveEffect should return false when no effects present")
	}
}

func TestHasConsumable_Found(t *testing.T) {
	agent := &agentprogression.Agent{
		Consumables: map[string]int{"xp_boost": 2},
	}
	if !hasConsumable(agent, "xp_boost") {
		t.Error("hasConsumable should return true when count > 0")
	}
}

func TestHasConsumable_Zero(t *testing.T) {
	agent := &agentprogression.Agent{
		Consumables: map[string]int{"xp_boost": 0},
	}
	if hasConsumable(agent, "xp_boost") {
		t.Error("hasConsumable should return false when count is 0")
	}
}

func TestHasConsumable_NilMap(t *testing.T) {
	agent := &agentprogression.Agent{}
	if hasConsumable(agent, "xp_boost") {
		t.Error("hasConsumable should return false when Consumables map is nil")
	}
}

func TestPickBestItem_PrefersToolsOverConsumables(t *testing.T) {
	items := []agentstore.StoreItem{
		{ID: "cheap_consumable", ItemType: agentstore.ItemTypeConsumable, XPCost: 50},
		{ID: "expensive_tool", ItemType: agentstore.ItemTypeTool, XPCost: 200},
	}
	agent := &agentprogression.Agent{}
	result := pickBestItem(agent, items, 300)
	if result == nil || result.ID != "expensive_tool" {
		t.Errorf("pickBestItem should prefer tool, got %v", result)
	}
}

func TestPickBestItem_SkipsOwnedTools(t *testing.T) {
	items := []agentstore.StoreItem{
		{ID: "owned_tool", ItemType: agentstore.ItemTypeTool, XPCost: 200},
		{ID: "new_consumable", ItemType: agentstore.ItemTypeConsumable, XPCost: 50},
	}
	agent := &agentprogression.Agent{
		OwnedTools: map[string]agentprogression.OwnedTool{
			"owned_tool": {},
		},
	}
	result := pickBestItem(agent, items, 300)
	if result == nil || result.ID != "new_consumable" {
		t.Errorf("pickBestItem should skip owned tool and pick consumable, got %v", result)
	}
}

func TestPickBestItem_CapsConsumableAt2(t *testing.T) {
	items := []agentstore.StoreItem{
		{ID: "stocked_consumable", ItemType: agentstore.ItemTypeConsumable, XPCost: 50},
	}
	agent := &agentprogression.Agent{
		Consumables: map[string]int{"stocked_consumable": 2},
	}
	result := pickBestItem(agent, items, 300)
	if result != nil {
		t.Errorf("pickBestItem should return nil when consumable already at max stock, got %v", result)
	}
}

func TestPickBestItem_PicksMostExpensiveTool(t *testing.T) {
	items := []agentstore.StoreItem{
		{ID: "cheap_tool", ItemType: agentstore.ItemTypeTool, XPCost: 50},
		{ID: "expensive_tool", ItemType: agentstore.ItemTypeTool, XPCost: 200},
	}
	agent := &agentprogression.Agent{}
	result := pickBestItem(agent, items, 300)
	if result == nil || result.ID != "expensive_tool" {
		t.Errorf("pickBestItem should pick most expensive affordable tool, got %v", result)
	}
}

func TestPickBestItem_RespectsBudgetLimit(t *testing.T) {
	items := []agentstore.StoreItem{
		{ID: "too_expensive", ItemType: agentstore.ItemTypeTool, XPCost: 500},
		{ID: "affordable", ItemType: agentstore.ItemTypeConsumable, XPCost: 50},
	}
	agent := &agentprogression.Agent{}
	result := pickBestItem(agent, items, 100)
	if result == nil || result.ID != "affordable" {
		t.Errorf("pickBestItem should skip items over budget, got %v", result)
	}
}

func TestPickBestItem_NothingAffordable(t *testing.T) {
	items := []agentstore.StoreItem{
		{ID: "expensive", ItemType: agentstore.ItemTypeTool, XPCost: 500},
	}
	agent := &agentprogression.Agent{}
	result := pickBestItem(agent, items, 100)
	if result != nil {
		t.Errorf("pickBestItem should return nil when nothing is affordable, got %v", result)
	}
}

func TestPickBestItem_EmptyItems(t *testing.T) {
	agent := &agentprogression.Agent{}
	result := pickBestItem(agent, nil, 300)
	if result != nil {
		t.Errorf("pickBestItem should return nil for empty items, got %v", result)
	}
}

func TestIntervalForStatus(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		status domain.AgentStatus
		want   time.Duration
	}{
		{domain.AgentIdle, 5 * time.Second},
		{domain.AgentOnQuest, 30 * time.Second},
		{domain.AgentInBattle, 60 * time.Second},
		{domain.AgentCooldown, 15 * time.Second},
		{domain.AgentRetired, 0},
	}

	for _, tt := range tests {
		got := cfg.IntervalForStatus(tt.status)
		if got != tt.want {
			t.Errorf("IntervalForStatus(%v) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.InitialDelayMs != 2000 {
		t.Errorf("InitialDelayMs = %d, want 2000", cfg.InitialDelayMs)
	}
	if cfg.IdleIntervalMs != 5000 {
		t.Errorf("IdleIntervalMs = %d, want 5000", cfg.IdleIntervalMs)
	}
	if cfg.OnQuestIntervalMs != 30000 {
		t.Errorf("OnQuestIntervalMs = %d, want 30000", cfg.OnQuestIntervalMs)
	}
	if cfg.InBattleIntervalMs != 60000 {
		t.Errorf("InBattleIntervalMs = %d, want 60000", cfg.InBattleIntervalMs)
	}
	if cfg.CooldownIntervalMs != 15000 {
		t.Errorf("CooldownIntervalMs = %d, want 15000", cfg.CooldownIntervalMs)
	}
	if cfg.MaxIntervalMs != 60000 {
		t.Errorf("MaxIntervalMs = %d, want 60000", cfg.MaxIntervalMs)
	}
	if cfg.BackoffFactor != 1.5 {
		t.Errorf("BackoffFactor = %f, want 1.5", cfg.BackoffFactor)
	}

	// Shopping config defaults
	if cfg.MinXPSurplusForShopping != 50 {
		t.Errorf("MinXPSurplusForShopping = %d, want 50", cfg.MinXPSurplusForShopping)
	}
	if cfg.MaxShopSpendRatio != 0.5 {
		t.Errorf("MaxShopSpendRatio = %f, want 0.5", cfg.MaxShopSpendRatio)
	}
	if cfg.CooldownShopMinXP != 25 {
		t.Errorf("CooldownShopMinXP = %d, want 25", cfg.CooldownShopMinXP)
	}
	if cfg.StrategicShopMaxCost != 200 {
		t.Errorf("StrategicShopMaxCost = %d, want 200", cfg.StrategicShopMaxCost)
	}
	if cfg.CooldownSkipMinRemainingMs != 30000 {
		t.Errorf("CooldownSkipMinRemainingMs = %d, want 30000", cfg.CooldownSkipMinRemainingMs)
	}

}

func TestBackoffOnlyIdle(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	idleInterval := cfg.IntervalForStatus(domain.AgentIdle)

	// Set up idle agent tracker
	comp.trackers["idle-agent"] = &agentTracker{
		agent:    &agentprogression.Agent{Status: domain.AgentIdle},
		interval: idleInterval,
	}

	// Set up on_quest agent tracker
	questInterval := cfg.IntervalForStatus(domain.AgentOnQuest)
	comp.trackers["quest-agent"] = &agentTracker{
		agent:    &agentprogression.Agent{Status: domain.AgentOnQuest},
		interval: questInterval,
	}

	// Backoff both
	comp.backoffHeartbeat("idle-agent")
	comp.backoffHeartbeat("quest-agent")

	// Idle agent should have grown interval
	idleTracker := comp.trackers["idle-agent"]
	expectedIdleInterval := time.Duration(float64(idleInterval) * cfg.BackoffFactor)
	if idleTracker.interval != expectedIdleInterval {
		t.Errorf("idle backoff: interval = %v, want %v", idleTracker.interval, expectedIdleInterval)
	}

	// On-quest agent should be unchanged
	questTracker := comp.trackers["quest-agent"]
	if questTracker.interval != questInterval {
		t.Errorf("quest backoff: interval = %v, want %v (unchanged)", questTracker.interval, questInterval)
	}
}

func TestBackoffCapsAtMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxIntervalMs = 10000 // 10s cap for easy testing
	comp := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	// Start with interval already near max
	comp.trackers["agent"] = &agentTracker{
		agent:    &agentprogression.Agent{Status: domain.AgentIdle},
		interval: 9 * time.Second,
	}

	comp.backoffHeartbeat("agent")

	maxInterval := time.Duration(cfg.MaxIntervalMs) * time.Millisecond
	tracker := comp.trackers["agent"]
	if tracker.interval > maxInterval {
		t.Errorf("backoff exceeded max: got %v, max %v", tracker.interval, maxInterval)
	}
	if tracker.interval != maxInterval {
		t.Errorf("backoff should cap at max: got %v, want %v", tracker.interval, maxInterval)
	}
}

// =============================================================================
// PORT AND SCHEMA TESTS - No infrastructure required
// =============================================================================

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) != 2 {
		t.Fatalf("InputPorts: got %d, want 2", len(inputs))
	}

	// Verify input port names
	portNames := map[string]bool{}
	for _, p := range inputs {
		portNames[p.Name] = true
	}
	if !portNames["agent-state-watch"] {
		t.Error("missing input port: agent-state-watch")
	}
	if !portNames["boid-suggestions"] {
		t.Error("missing input port: boid-suggestions")
	}

	outputs := comp.OutputPorts()
	wantOutputs := []string{
		"autonomy-evaluated", "autonomy-idle", "claim-state",
		"claim-intent", "shop-intent", "guild-intent", "use-intent", "guild-create-intent",
	}
	if len(outputs) != len(wantOutputs) {
		t.Fatalf("OutputPorts: got %d, want %d", len(outputs), len(wantOutputs))
	}

	outputNames := map[string]bool{}
	for _, p := range outputs {
		outputNames[p.Name] = true
	}
	for _, want := range wantOutputs {
		if !outputNames[want] {
			t.Errorf("missing output port: %s", want)
		}
	}
}

func TestComponent_ConfigSchema(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	// Check required fields
	for _, req := range []string{"org", "platform", "board"} {
		found := false
		for _, r := range schema.Required {
			if r == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required field %q not in ConfigSchema.Required", req)
		}
	}

	// Check heartbeat properties exist
	heartbeatProps := []string{
		"initial_delay_ms", "idle_interval_ms", "on_quest_interval_ms",
		"in_battle_interval_ms", "cooldown_interval_ms", "max_interval_ms",
		"backoff_factor",
	}
	for _, prop := range heartbeatProps {
		if _, ok := schema.Properties[prop]; !ok {
			t.Errorf("missing heartbeat property %q in ConfigSchema", prop)
		}
	}

	// Check shopping/consumable properties exist
	shoppingProps := []string{
		"min_xp_surplus_for_shopping", "max_shop_spend_ratio",
		"cooldown_shop_min_xp", "strategic_shop_max_cost",
		"cooldown_skip_min_remaining_ms",
	}
	for _, prop := range shoppingProps {
		if _, ok := schema.Properties[prop]; !ok {
			t.Errorf("missing shopping property %q in ConfigSchema", prop)
		}
	}

}

// =============================================================================
// PAYLOAD GRAPHABLE TESTS
// =============================================================================

func TestEvaluatedPayload_Graphable(t *testing.T) {
	now := time.Now()
	p := &EvaluatedPayload{
		AgentID:     "test.local.game.board1.agent.eval1",
		AgentStatus: "idle",
		ActionTaken: "none",
		Interval:    5 * time.Second,
		Timestamp:   now,
	}

	// EntityID returns the agent ID string.
	if got := p.EntityID(); got != "test.local.game.board1.agent.eval1" {
		t.Errorf("EntityID() = %q, want agent ID", got)
	}

	// Triples returns exactly 3 triples.
	triples := p.Triples()
	if len(triples) != 3 {
		t.Fatalf("Triples() returned %d triples, want 3", len(triples))
	}

	// Every triple must reference the agent as subject.
	for i, tr := range triples {
		if tr.Subject != "test.local.game.board1.agent.eval1" {
			t.Errorf("triple[%d].Subject = %q, want agent ID", i, tr.Subject)
		}
	}

	// All three expected predicates must be present.
	predicates := make(map[string]bool, len(triples))
	for _, tr := range triples {
		predicates[tr.Predicate] = true
	}
	for _, want := range []string{
		"agent.autonomy.status",
		"agent.autonomy.action",
		"agent.autonomy.interval_ms",
	} {
		if !predicates[want] {
			t.Errorf("missing triple predicate %q", want)
		}
	}

	// Schema returns the correct domain and category.
	s := p.Schema()
	if s.Domain != "semdragons" || s.Category != "autonomy.evaluated" {
		t.Errorf("Schema() = {Domain:%q Category:%q}, want semdragons/autonomy.evaluated", s.Domain, s.Category)
	}

	// Validate succeeds for a fully populated payload.
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}

	// Validate fails when AgentID is absent.
	missingID := &EvaluatedPayload{Timestamp: now}
	if err := missingID.Validate(); err == nil {
		t.Error("Validate() should fail with empty AgentID")
	}

	// Validate fails when Timestamp is the zero value.
	missingTS := &EvaluatedPayload{AgentID: "test.local.game.board1.agent.eval1"}
	if err := missingTS.Validate(); err == nil {
		t.Error("Validate() should fail with zero Timestamp")
	}
}

func TestIdlePayload_Graphable(t *testing.T) {
	now := time.Now()
	p := &IdlePayload{
		AgentID:       "test.local.game.board1.agent.idle1",
		IdleDuration:  30 * time.Second,
		HasSuggestion: false,
		BackoffMs:     7500,
		Timestamp:     now,
	}

	// EntityID returns the agent ID string.
	if got := p.EntityID(); got != "test.local.game.board1.agent.idle1" {
		t.Errorf("EntityID() = %q, want agent ID", got)
	}

	// Triples returns exactly 3 triples.
	triples := p.Triples()
	if len(triples) != 3 {
		t.Fatalf("Triples() returned %d triples, want 3", len(triples))
	}

	// Every triple must reference the agent as subject.
	for i, tr := range triples {
		if tr.Subject != "test.local.game.board1.agent.idle1" {
			t.Errorf("triple[%d].Subject = %q, want agent ID", i, tr.Subject)
		}
	}

	// All three expected predicates must be present.
	predicates := make(map[string]bool, len(triples))
	for _, tr := range triples {
		predicates[tr.Predicate] = true
	}
	for _, want := range []string{
		"agent.autonomy.idle_duration_ms",
		"agent.autonomy.has_suggestion",
		"agent.autonomy.backoff_ms",
	} {
		if !predicates[want] {
			t.Errorf("missing triple predicate %q", want)
		}
	}

	// Schema returns the correct domain and category.
	s := p.Schema()
	if s.Domain != "semdragons" || s.Category != "autonomy.idle" {
		t.Errorf("Schema() = {Domain:%q Category:%q}, want semdragons/autonomy.idle", s.Domain, s.Category)
	}

	// Validate succeeds for a fully populated payload.
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}

	// Validate fails when AgentID is absent.
	missingID := &IdlePayload{Timestamp: now}
	if err := missingID.Validate(); err == nil {
		t.Error("Validate() should fail with empty AgentID")
	}

	// Validate fails when Timestamp is the zero value.
	missingTS := &IdlePayload{AgentID: "test.local.game.board1.agent.idle1"}
	if err := missingTS.Validate(); err == nil {
		t.Error("Validate() should fail with zero Timestamp")
	}
}

func TestClaimIntentPayload_Graphable(t *testing.T) {
	now := time.Now()
	p := &ClaimIntentPayload{
		AgentID:        "test.local.game.board1.agent.claim1",
		QuestID:        "test.local.game.board1.quest.q1",
		Score:          0.85,
		SuggestionRank: 1,
		Timestamp:      now,
	}

	if got := p.EntityID(); got != "test.local.game.board1.agent.claim1" {
		t.Errorf("EntityID() = %q, want agent ID", got)
	}

	triples := p.Triples()
	if len(triples) != 3 {
		t.Fatalf("Triples() returned %d triples, want 3", len(triples))
	}
	predicates := make(map[string]bool, len(triples))
	for _, tr := range triples {
		if tr.Subject != "test.local.game.board1.agent.claim1" {
			t.Errorf("triple Subject = %q, want agent ID", tr.Subject)
		}
		predicates[tr.Predicate] = true
	}
	for _, want := range []string{
		"agent.autonomy.claim_quest",
		"agent.autonomy.claim_score",
		"agent.autonomy.claim_rank",
	} {
		if !predicates[want] {
			t.Errorf("missing triple predicate %q", want)
		}
	}

	s := p.Schema()
	if s.Domain != "semdragons" || s.Category != "autonomy.claimintent" {
		t.Errorf("Schema() = {Domain:%q Category:%q}, want semdragons/autonomy.claimintent", s.Domain, s.Category)
	}

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
	if err := (&ClaimIntentPayload{Timestamp: now}).Validate(); err == nil {
		t.Error("Validate() should fail with empty AgentID")
	}
	if err := (&ClaimIntentPayload{AgentID: "x"}).Validate(); err == nil {
		t.Error("Validate() should fail with zero Timestamp")
	}
}

func TestShopIntentPayload_Graphable(t *testing.T) {
	now := time.Now()
	p := &ShopIntentPayload{
		AgentID:   "test.local.game.board1.agent.shop1",
		ItemID:    "quality_shield",
		ItemName:  "Quality Shield",
		XPCost:    100,
		Budget:    500,
		Strategic: true,
		Timestamp: now,
	}

	if got := p.EntityID(); got != "test.local.game.board1.agent.shop1" {
		t.Errorf("EntityID() = %q, want agent ID", got)
	}

	triples := p.Triples()
	if len(triples) != 4 {
		t.Fatalf("Triples() returned %d triples, want 4", len(triples))
	}
	predicates := make(map[string]bool, len(triples))
	for _, tr := range triples {
		predicates[tr.Predicate] = true
	}
	for _, want := range []string{
		"agent.autonomy.shop_item",
		"agent.autonomy.shop_cost",
		"agent.autonomy.shop_budget",
		"agent.autonomy.shop_strategic",
	} {
		if !predicates[want] {
			t.Errorf("missing triple predicate %q", want)
		}
	}

	s := p.Schema()
	if s.Domain != "semdragons" || s.Category != "autonomy.shopintent" {
		t.Errorf("Schema() = {Domain:%q Category:%q}, want semdragons/autonomy.shopintent", s.Domain, s.Category)
	}

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
	if err := (&ShopIntentPayload{Timestamp: now}).Validate(); err == nil {
		t.Error("Validate() should fail with empty AgentID")
	}
	if err := (&ShopIntentPayload{AgentID: "x"}).Validate(); err == nil {
		t.Error("Validate() should fail with zero Timestamp")
	}
}

func TestGuildIntentPayload_Graphable(t *testing.T) {
	now := time.Now()
	p := &GuildIntentPayload{
		AgentID:          "test.local.game.board1.agent.gjoin1",
		GuildID:          "guild.warriors",
		GuildName:        "Warriors",
		Score:            0.72,
		ChoicesEvaluated: 5,
		Timestamp:        now,
	}

	if got := p.EntityID(); got != "test.local.game.board1.agent.gjoin1" {
		t.Errorf("EntityID() = %q, want agent ID", got)
	}

	triples := p.Triples()
	if len(triples) != 3 {
		t.Fatalf("Triples() returned %d triples, want 3", len(triples))
	}
	predicates := make(map[string]bool, len(triples))
	for _, tr := range triples {
		predicates[tr.Predicate] = true
	}
	for _, want := range []string{
		"agent.autonomy.guild_join",
		"agent.autonomy.guild_score",
		"agent.autonomy.guild_choices",
	} {
		if !predicates[want] {
			t.Errorf("missing triple predicate %q", want)
		}
	}

	s := p.Schema()
	if s.Domain != "semdragons" || s.Category != "autonomy.guildintent" {
		t.Errorf("Schema() = {Domain:%q Category:%q}, want semdragons/autonomy.guildintent", s.Domain, s.Category)
	}

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
	if err := (&GuildIntentPayload{Timestamp: now}).Validate(); err == nil {
		t.Error("Validate() should fail with empty AgentID")
	}
	if err := (&GuildIntentPayload{AgentID: "x"}).Validate(); err == nil {
		t.Error("Validate() should fail with zero Timestamp")
	}
}

func TestUseIntentPayload_Graphable(t *testing.T) {
	now := time.Now()
	questID := domain.QuestID("test.local.game.board1.quest.q1")
	p := &UseIntentPayload{
		AgentID:      "test.local.game.board1.agent.use1",
		ConsumableID: "xp_boost",
		AgentStatus:  "on_quest",
		QuestID:      &questID,
		Timestamp:    now,
	}

	if got := p.EntityID(); got != "test.local.game.board1.agent.use1" {
		t.Errorf("EntityID() = %q, want agent ID", got)
	}

	// With QuestID set, should produce 3 triples
	triples := p.Triples()
	if len(triples) != 3 {
		t.Fatalf("Triples() returned %d triples, want 3 (with QuestID)", len(triples))
	}
	predicates := make(map[string]bool, len(triples))
	for _, tr := range triples {
		predicates[tr.Predicate] = true
	}
	for _, want := range []string{
		"agent.autonomy.use_consumable",
		"agent.autonomy.use_status",
		"agent.autonomy.use_quest",
	} {
		if !predicates[want] {
			t.Errorf("missing triple predicate %q", want)
		}
	}

	// Without QuestID, should produce 2 triples
	pNoQuest := &UseIntentPayload{
		AgentID:      "test.local.game.board1.agent.use2",
		ConsumableID: "cooldown_skip",
		AgentStatus:  "cooldown",
		Timestamp:    now,
	}
	if len(pNoQuest.Triples()) != 2 {
		t.Errorf("Triples() without QuestID returned %d triples, want 2", len(pNoQuest.Triples()))
	}

	s := p.Schema()
	if s.Domain != "semdragons" || s.Category != "autonomy.useintent" {
		t.Errorf("Schema() = {Domain:%q Category:%q}, want semdragons/autonomy.useintent", s.Domain, s.Category)
	}

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
	if err := (&UseIntentPayload{Timestamp: now}).Validate(); err == nil {
		t.Error("Validate() should fail with empty AgentID")
	}
	if err := (&UseIntentPayload{AgentID: "x"}).Validate(); err == nil {
		t.Error("Validate() should fail with zero Timestamp")
	}
}

// =============================================================================
// HEARTBEAT RESET TESTS
// =============================================================================

func TestResetHeartbeatInterval_ResetsBackedOffInterval(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	idleInterval := cfg.IntervalForStatus(domain.AgentIdle)

	// Create a tracker whose interval has been backed off twice.
	backedOff := time.Duration(float64(idleInterval) * cfg.BackoffFactor * cfg.BackoffFactor)
	comp.trackers["agent"] = &agentTracker{
		agent:    &agentprogression.Agent{Status: domain.AgentIdle},
		interval: backedOff,
	}

	comp.resetHeartbeatInterval("agent")

	if got := comp.trackers["agent"].interval; got != idleInterval {
		t.Errorf("interval after reset = %v, want base idle interval %v", got, idleInterval)
	}
}

func TestResetHeartbeatInterval_NoopWhenAlreadyAtBase(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	idleInterval := cfg.IntervalForStatus(domain.AgentIdle)

	// Tracker already at the base idle interval — reset should leave it unchanged.
	comp.trackers["agent"] = &agentTracker{
		agent:    &agentprogression.Agent{Status: domain.AgentIdle},
		interval: idleInterval,
	}

	comp.resetHeartbeatInterval("agent")

	if got := comp.trackers["agent"].interval; got != idleInterval {
		t.Errorf("interval after no-op reset = %v, want %v (unchanged)", got, idleInterval)
	}
}

// =============================================================================
// COOLDOWN EXPIRY TESTS
// =============================================================================

func TestCheckCooldownExpiry_NilCooldownUntil(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config: &cfg,
	}

	// An agent in cooldown with no CooldownUntil set must return false
	// without touching the graph client (which is nil here).
	agent := &agentprogression.Agent{
		ID:            "test.local.game.board1.agent.nilcd",
		Status:        domain.AgentCooldown,
		CooldownUntil: nil,
	}

	ctx := context.Background()
	if comp.checkCooldownExpiry(ctx, agent) {
		t.Error("checkCooldownExpiry should return false when CooldownUntil is nil")
	}
}

func TestCheckCooldownExpiry_FutureCooldownUntil(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config: &cfg,
	}

	// Cooldown that expires one hour from now should not be cleared.
	future := time.Now().Add(1 * time.Hour)
	agent := &agentprogression.Agent{
		ID:            "test.local.game.board1.agent.futurecd",
		Status:        domain.AgentCooldown,
		CooldownUntil: &future,
	}

	ctx := context.Background()
	if comp.checkCooldownExpiry(ctx, agent) {
		t.Error("checkCooldownExpiry should return false when cooldown has not expired")
	}
}

// =============================================================================
// FACTORY / REGISTRATION TESTS
// =============================================================================

func TestFactory_NilNATSClient(t *testing.T) {
	deps := component.Dependencies{} // NATSClient is nil by default
	_, err := Factory(nil, deps)
	if err == nil {
		t.Error("Factory should return an error when NATSClient is nil")
	}
}

func TestNewFromConfig_NilNATSClient(t *testing.T) {
	cfg := DefaultConfig()
	deps := component.Dependencies{} // NATSClient is nil by default
	_, err := NewFromConfig(cfg, deps)
	if err == nil {
		t.Error("NewFromConfig should return an error when NATSClient is nil")
	}
}

// =============================================================================
// ACTIONS FOR STATE - UNKNOWN STATUS
// =============================================================================

func TestActionsForState_Unknown(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState("some_unknown_status")
	if actions != nil {
		t.Errorf("unknown status actions: got %v, want nil", actions)
	}
}

// =============================================================================
// GUILD JOINING TESTS
// =============================================================================

// newTestComponentWithGuilds creates a Component backed by a registry that contains a
// zero-value guildformation.Component. Only GetAgentGuild() and ListGuilds() are safe
// to call on it (they use sync.Map zero-value semantics).
func newTestComponentWithGuilds(t *testing.T) *Component {
	t.Helper()
	cfg := DefaultConfig()
	guilds := &guildformation.Component{}
	reg := newTestRegistry(t, nil, guilds, nil)
	c := &Component{
		config: &cfg,
		deps:   component.Dependencies{ComponentRegistry: reg},
		trackers: make(map[string]*agentTracker),
	}
	return c
}

func TestJoinGuild_ShouldExecute_WithBoidSuggestion(t *testing.T) {
	c := newTestComponentWithGuilds(t)
	act := c.joinGuildAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.guild1",
		Status: domain.AgentIdle,
		Level:  5,
	}
	tracker := &agentTracker{
		agent:           agent,
		guildSuggestion: &boidengine.GuildSuggestion{Type: "join", GuildID: "guild-1"},
	}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be true when boid suggests join and agent is unguilded")
	}
}

func TestJoinGuild_ShouldExecute_NoSuggestion(t *testing.T) {
	c := newTestComponentWithGuilds(t)
	act := c.joinGuildAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.guild2",
		Status: domain.AgentIdle,
		Level:  5,
	}
	tracker := &agentTracker{agent: agent} // no guildSuggestion

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when no boid guild suggestion")
	}
}

func TestJoinGuild_ShouldExecute_FormSuggestionIgnored(t *testing.T) {
	c := newTestComponentWithGuilds(t)
	act := c.joinGuildAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.guild3",
		Status: domain.AgentIdle,
		Level:  5,
	}
	tracker := &agentTracker{
		agent:           agent,
		guildSuggestion: &boidengine.GuildSuggestion{Type: "form"}, // wrong type
	}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when boid suggests form (not join)")
	}
}

func TestJoinGuild_ShouldExecute_NilGuilds(t *testing.T) {
	c := newTestComponent() // no guilds component
	act := c.joinGuildAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.guild5",
		Status: domain.AgentIdle,
		Level:  10,
	}
	tracker := &agentTracker{
		agent:           agent,
		guildSuggestion: &boidengine.GuildSuggestion{Type: "join", GuildID: "guild-1"},
	}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when guilds component is nil")
	}
}

func TestJoinGuild_ShouldExecute_AlreadyGuilded(t *testing.T) {
	c := newTestComponentWithGuilds(t)
	act := c.joinGuildAction()

	agent := &agentprogression.Agent{
		ID:     "test.local.game.board1.agent.guild8",
		Status: domain.AgentIdle,
		Level:  5,
		Guild:  domain.GuildID("guild1"), // already in a guild
	}
	tracker := &agentTracker{
		agent:           agent,
		guildSuggestion: &boidengine.GuildSuggestion{Type: "join", GuildID: "guild-2"},
	}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should be false when agent already in a guild")
	}
}

// =============================================================================
// APPROVAL GATE UNIT TESTS
// =============================================================================

func TestRequestApproval_FullAuto(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DMMode = domain.DMFullAuto
	cfg.SessionID = "session-1"
	c := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
		// approval is nil — but FullAuto short-circuits before checking
	}

	if !c.requestApproval(context.Background(), domain.ApprovalAutonomyClaim, "test", "details", nil) {
		t.Error("requestApproval should return true in FullAuto mode")
	}
}

func TestRequestApproval_Assisted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DMMode = domain.DMAssisted
	cfg.SessionID = "session-1"
	c := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	if !c.requestApproval(context.Background(), domain.ApprovalAutonomyShop, "test", "details", nil) {
		t.Error("requestApproval should return true in Assisted mode")
	}
}

func TestRequestApproval_NilApproval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DMMode = domain.DMSupervised
	cfg.SessionID = "session-1"
	c := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
		// registry is nil — resolveApproval returns nil, triggering auto-approve
	}

	if !c.requestApproval(context.Background(), domain.ApprovalAutonomyGuild, "test", "details", nil) {
		t.Error("requestApproval should return true when approval component is nil")
	}
}

func TestRequestApproval_EmptySession(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DMMode = domain.DMSupervised
	cfg.SessionID = "" // empty session
	c := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	if !c.requestApproval(context.Background(), domain.ApprovalAutonomyUse, "test", "details", nil) {
		t.Error("requestApproval should return true when SessionID is empty")
	}
}

func TestRequestApproval_Supervised_NotRunning_Denied(t *testing.T) {
	// When the approval component is not running, RequestApproval returns an error,
	// which requestApproval translates to denied (false).
	cfg := DefaultConfig()
	cfg.DMMode = domain.DMSupervised
	cfg.SessionID = "session-1"
	cfg.ApprovalTimeoutMs = 1000 // short timeout for test

	approvalComp := &dmapproval.Component{} // not started — RequestApproval will fail
	reg := newTestRegistry(t, nil, nil, approvalComp)

	c := &Component{
		config:   &cfg,
		deps:     component.Dependencies{ComponentRegistry: reg},
		trackers: make(map[string]*agentTracker),
		logger:   slog.Default(),
	}

	if c.requestApproval(context.Background(), domain.ApprovalAutonomyClaim, "test", "details", nil) {
		t.Error("requestApproval should return false when approval component returns error")
	}
}

func TestRequestApproval_Manual_NotRunning_Denied(t *testing.T) {
	// Manual mode behaves identically to Supervised — both require approval.
	cfg := DefaultConfig()
	cfg.DMMode = domain.DMManual
	cfg.SessionID = "session-1"
	cfg.ApprovalTimeoutMs = 1000

	approvalComp := &dmapproval.Component{} // not started — RequestApproval will fail
	reg := newTestRegistry(t, nil, nil, approvalComp)

	c := &Component{
		config:   &cfg,
		deps:     component.Dependencies{ComponentRegistry: reg},
		trackers: make(map[string]*agentTracker),
		logger:   slog.Default(),
	}

	if c.requestApproval(context.Background(), domain.ApprovalAutonomyClaim, "test", "details", nil) {
		t.Error("requestApproval should return false in Manual mode when approval component returns error")
	}
}

func TestInitialize_InvalidDMMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DMMode = "invalid_mode"

	c := &Component{
		config: &cfg,
		deps:   component.Dependencies{NATSClient: &natsclient.Client{}},
	}

	if err := c.Initialize(); err == nil {
		t.Error("Initialize should return error for invalid DMMode")
	}
}

func TestApprovalTimeout_Default(t *testing.T) {
	cfg := DefaultConfig()
	got := cfg.ApprovalTimeout()
	want := 5 * time.Minute
	if got != want {
		t.Errorf("ApprovalTimeout() = %v, want %v", got, want)
	}
}

func TestApprovalTimeout_Custom(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ApprovalTimeoutMs = 60000
	got := cfg.ApprovalTimeout()
	want := time.Minute
	if got != want {
		t.Errorf("ApprovalTimeout() = %v, want %v", got, want)
	}
}

func TestApprovalTimeout_Zero(t *testing.T) {
	cfg := Config{} // zero value
	got := cfg.ApprovalTimeout()
	want := 5 * time.Minute // should default
	if got != want {
		t.Errorf("ApprovalTimeout() with zero value = %v, want default %v", got, want)
	}
}

func TestConfigSchema_ApprovalProperties(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	approvalProps := []string{"dm_mode", "session_id", "approval_timeout_ms"}
	for _, prop := range approvalProps {
		if _, ok := schema.Properties[prop]; !ok {
			t.Errorf("missing approval property %q in ConfigSchema", prop)
		}
	}
}

