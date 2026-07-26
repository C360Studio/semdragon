package agentstore

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
)

var agentStorePredicateRenames = map[string]string{
	"store.item.purchase_type":          "store.item.purchase-type",
	"store.item.xp_cost":                "store.item.xp-cost",
	"store.item.min_tier":               "store.item.min-tier",
	"store.item.in_stock":               "store.item.in-stock",
	"store.item.rental_uses":            "store.item.rental-uses",
	"store.item.min_level":              "store.item.min-level",
	"store.item.guild_discount":         "store.item.guild-discount",
	"store.item.tool_id":                "store.item.tool-id",
	"store.item.effect_type":            "store.item.effect-type",
	"store.item.effect_magnitude":       "store.item.effect-magnitude",
	"store.item.effect_duration":        "store.item.effect-duration",
	"agent.inventory.total_spent":       "agent.inventory.total-spent",
	"agent.inventory.tools_count":       "agent.inventory.tools-count",
	"agent.inventory.consumables_count": "agent.inventory.consumables-count",
}

func TestStoreItem_EntityID_SixPart(t *testing.T) {
	bc := &domain.BoardConfig{
		Org:      "c360",
		Platform: "prod",
		Board:    "board1",
	}

	item := StoreItem{
		ID:          "web_search",
		Name:        "Web Search",
		BoardConfig: bc,
	}

	entityID := item.EntityID()
	want := "c360.prod.game.board1.storeitem.web_search"
	if entityID != want {
		t.Errorf("EntityID() = %q, want %q", entityID, want)
	}

	if !domain.IsValidEntityID(entityID) {
		t.Errorf("EntityID() %q is not a valid 6-part entity ID", entityID)
	}

	if !domain.IsStoreItemID(entityID) {
		t.Errorf("IsStoreItemID(%q) = false, want true", entityID)
	}
}

func TestStoreItem_EntityIDRequiresIDAndBoardConfig(t *testing.T) {
	t.Parallel()

	boardConfig := &domain.BoardConfig{Org: "c360", Platform: "prod", Board: "board1"}
	tests := []struct {
		name string
		item *StoreItem
	}{
		{name: "nil receiver"},
		{name: "nil board config", item: &StoreItem{ID: "web_search", Name: "Web Search"}},
		{name: "empty item id", item: &StoreItem{BoardConfig: boardConfig, Name: "Web Search"}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.item.EntityID(); got != "" {
				t.Errorf("EntityID() = %q, want empty", got)
			}
			if got := tt.item.Triples(); got != nil {
				t.Errorf("Triples() = %#v, want nil", got)
			}
		})
	}
}

func TestDefaultCatalogEntityIDAliasesAreValidWithBoardConfig(t *testing.T) {
	t.Parallel()

	boardConfig := &domain.BoardConfig{Org: "c360", Platform: "prod", Board: "board1"}
	for _, catalogItem := range DefaultCatalog() {
		item := catalogItem
		t.Run(item.ID, func(t *testing.T) {
			t.Parallel()
			item.BoardConfig = boardConfig
			got := item.EntityID()
			want := boardConfig.StoreItemEntityID(item.ID)
			if got != want {
				t.Errorf("EntityID() = %q, want %q", got, want)
			}
			if !domain.IsValidEntityID(got) || !domain.IsStoreItemID(got) {
				t.Errorf("catalog alias %q produced invalid store-item ID %q", item.ID, got)
			}
			if parts := strings.Split(got, "."); len(parts) != 6 {
				t.Errorf("entity ID positions = %d, want 6: %q", len(parts), got)
			}
		})
	}
}

func TestStoreItem_Triples_AllFields(t *testing.T) {
	bc := &domain.BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "board1",
	}

	item := StoreItem{
		ID:            "xp_boost",
		Name:          "XP Boost",
		Description:   "Doubles XP",
		ItemType:      ItemTypeConsumable,
		PurchaseType:  PurchasePermanent,
		XPCost:        100,
		RentalUses:    0,
		MinTier:       domain.TierJourneyman,
		MinLevel:      6,
		InStock:       true,
		GuildDiscount: 0.15,
		Effect: &ConsumableEffect{
			Type:      ConsumableXPBoost,
			Magnitude: 2.0,
			Duration:  1,
		},
		BoardConfig: bc,
	}

	triples := item.Triples()

	expected := map[string]bool{
		"store.item.name":             false,
		"store.item.description":      false,
		"store.item.type":             false,
		"store.item.purchase-type":    false,
		"store.item.xp-cost":          false,
		"store.item.min-tier":         false,
		"store.item.in-stock":         false,
		"store.item.rental-uses":      false,
		"store.item.min-level":        false,
		"store.item.guild-discount":   false,
		"store.item.effect-type":      false,
		"store.item.effect-magnitude": false,
		"store.item.effect-duration":  false,
	}

	for _, triple := range triples {
		if _, ok := expected[triple.Predicate]; ok {
			expected[triple.Predicate] = true
		}
	}

	for pred, found := range expected {
		if !found {
			t.Errorf("expected predicate %q not found in store item triples", pred)
		}
	}
}

func TestStoreItem_Triples_ToolWithToolID(t *testing.T) {
	bc := &domain.BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "board1",
	}

	item := StoreItem{
		ID:           "web_search",
		Name:         "Web Search",
		ItemType:     ItemTypeTool,
		PurchaseType: PurchasePermanent,
		ToolID:       "web_search",
		BoardConfig:  bc,
	}

	triples := item.Triples()

	var foundToolID bool
	for _, triple := range triples {
		if triple.Predicate == "store.item.tool-id" {
			foundToolID = true
			if got := triple.Object.(string); got != "web_search" {
				t.Errorf("store.item.tool-id = %q, want %q", got, "web_search")
			}
		}
	}
	if !foundToolID {
		t.Error("store.item.tool-id triple not found")
	}
}

func TestAgentInventoryTriplesUseCanonicalAggregatesAndScalarToolAliases(t *testing.T) {
	t.Parallel()

	inventory := &AgentInventory{
		AgentID: "c360.prod.game.board1.agent.dragon",
		OwnedTools: map[string]OwnedItem{
			"web_search":    {ItemID: "web_search"},
			"code_reviewer": {ItemID: "code_reviewer"},
		},
		Consumables: map[string]int{"retry_token": 2},
		TotalSpent:  350,
	}
	triples := inventory.Triples()

	for predicate, want := range map[string]int64{
		"agent.inventory.total-spent":       350,
		"agent.inventory.tools-count":       2,
		"agent.inventory.consumables-count": 1,
	} {
		triple, ok := findStoreTriple(triples, predicate)
		if !ok {
			t.Fatalf("missing %q", predicate)
		}
		if got := tripleInt64(triple.Object); got != want {
			t.Errorf("%s = %d, want %d", predicate, got, want)
		}
	}

	var toolAliases []string
	for _, triple := range triples {
		if triple.Predicate != "agent.inventory.tool" {
			continue
		}
		if triple.Datatype != "" {
			t.Errorf("agent.inventory.tool datatype = %q, want empty scalar alias", triple.Datatype)
		}
		toolAliases = append(toolAliases, tripleString(triple.Object))
	}
	slices.Sort(toolAliases)
	if !slices.Equal(toolAliases, []string{"code_reviewer", "web_search"}) {
		t.Errorf("tool aliases = %v", toolAliases)
	}
}

func TestAgentStorePredicatesAreCanonicalOnly(t *testing.T) {
	t.Parallel()

	if len(agentStorePredicateRenames) != 14 {
		t.Fatalf("predicate rename contract has %d entries, want 14", len(agentStorePredicateRenames))
	}
	source, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("read store.go: %v", err)
	}
	text := string(source)
	for stale, canonical := range agentStorePredicateRenames {
		if strings.Contains(text, `"`+stale+`"`) {
			t.Errorf("store.go retains stale predicate %q", stale)
		}
		if !strings.Contains(text, `"`+canonical+`"`) {
			t.Errorf("store.go is missing canonical predicate %q", canonical)
		}
	}
	if strings.Contains(text, `"store.item." + s.ID`) {
		t.Error("store.go retains synthetic legacy store item entity ID")
	}
}

func TestStoreItemFromEntityState(t *testing.T) {
	bc := &domain.BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "board1",
	}

	original := StoreItem{
		ID:            "xp_boost",
		Name:          "XP Boost",
		Description:   "Doubles XP on next quest",
		ItemType:      ItemTypeConsumable,
		PurchaseType:  PurchasePermanent,
		XPCost:        100,
		MinTier:       domain.TierJourneyman,
		MinLevel:      6,
		InStock:       true,
		GuildDiscount: 0.15,
		RentalUses:    0,
		Effect: &ConsumableEffect{
			Type:      ConsumableXPBoost,
			Magnitude: 2.0,
			Duration:  1,
		},
		BoardConfig: bc,
	}

	// Serialize to triples and reconstruct
	entity := &graph.EntityState{
		ID:      original.EntityID(),
		Triples: original.Triples(),
	}

	r := StoreItemFromEntityState(entity)
	if r == nil {
		t.Fatal("StoreItemFromEntityState returned nil")
	}

	if r.ID != original.ID {
		t.Errorf("ID = %q, want %q", r.ID, original.ID)
	}
	if r.Name != original.Name {
		t.Errorf("Name = %q, want %q", r.Name, original.Name)
	}
	if r.Description != original.Description {
		t.Errorf("Description = %q, want %q", r.Description, original.Description)
	}
	if r.ItemType != original.ItemType {
		t.Errorf("ItemType = %q, want %q", r.ItemType, original.ItemType)
	}
	if r.PurchaseType != original.PurchaseType {
		t.Errorf("PurchaseType = %q, want %q", r.PurchaseType, original.PurchaseType)
	}
	if r.XPCost != original.XPCost {
		t.Errorf("XPCost = %d, want %d", r.XPCost, original.XPCost)
	}
	if r.MinTier != original.MinTier {
		t.Errorf("MinTier = %v, want %v", r.MinTier, original.MinTier)
	}
	if r.MinLevel != original.MinLevel {
		t.Errorf("MinLevel = %d, want %d", r.MinLevel, original.MinLevel)
	}
	if r.InStock != original.InStock {
		t.Errorf("InStock = %v, want %v", r.InStock, original.InStock)
	}
	if r.GuildDiscount != original.GuildDiscount {
		t.Errorf("GuildDiscount = %v, want %v", r.GuildDiscount, original.GuildDiscount)
	}
	if r.RentalUses != original.RentalUses {
		t.Errorf("RentalUses = %d, want %d", r.RentalUses, original.RentalUses)
	}

	// Effect
	if r.Effect == nil {
		t.Fatal("Effect is nil")
	}
	if r.Effect.Type != original.Effect.Type {
		t.Errorf("Effect.Type = %q, want %q", r.Effect.Type, original.Effect.Type)
	}
	if r.Effect.Magnitude != original.Effect.Magnitude {
		t.Errorf("Effect.Magnitude = %v, want %v", r.Effect.Magnitude, original.Effect.Magnitude)
	}
	if r.Effect.Duration != original.Effect.Duration {
		t.Errorf("Effect.Duration = %d, want %d", r.Effect.Duration, original.Effect.Duration)
	}
}

func TestStoreItemFromEntityState_NilReturnsNil(t *testing.T) {
	if got := StoreItemFromEntityState(nil); got != nil {
		t.Errorf("StoreItemFromEntityState(nil) = %v, want nil", got)
	}
}

func TestStoreItemFromEntityStateDoesNotReadLegacyAliases(t *testing.T) {
	t.Parallel()

	var triples []message.Triple
	for stale := range agentStorePredicateRenames {
		if !strings.HasPrefix(stale, "store.item.") {
			continue
		}
		var object any = 99
		switch stale {
		case "store.item.purchase_type":
			object = "rental"
		case "store.item.in_stock":
			object = true
		case "store.item.tool_id":
			object = "legacy-tool"
		case "store.item.effect_type":
			object = "xp_boost"
		}
		triples = append(triples, message.Triple{
			Subject:   "c360.prod.game.board1.storeitem.legacy",
			Predicate: stale,
			Object:    object,
		})
	}
	got := StoreItemFromEntityState(&graph.EntityState{
		ID:      "c360.prod.game.board1.storeitem.legacy",
		Triples: triples,
	})
	if got == nil {
		t.Fatal("StoreItemFromEntityState returned nil")
	}
	if got.PurchaseType != "" ||
		got.XPCost != 0 ||
		got.MinTier != 0 ||
		got.InStock ||
		got.RentalUses != 0 ||
		got.MinLevel != 0 ||
		got.GuildDiscount != 0 ||
		got.ToolID != "" ||
		got.Effect != nil {
		t.Errorf("legacy aliases changed canonical-only reconstruction: %#v", got)
	}
}

func TestStoreItemFromEntityState_ToolRoundTrip(t *testing.T) {
	bc := &domain.BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "board1",
	}

	original := StoreItem{
		ID:            "deploy_access",
		Name:          "Deploy Access",
		Description:   "Permission to deploy to staging",
		ItemType:      ItemTypeTool,
		PurchaseType:  PurchasePermanent,
		XPCost:        500,
		MinTier:       domain.TierExpert,
		MinLevel:      11,
		ToolID:        "deploy_access",
		InStock:       true,
		GuildDiscount: 0.20,
		BoardConfig:   bc,
	}

	entity := &graph.EntityState{
		ID:      original.EntityID(),
		Triples: original.Triples(),
	}

	r := StoreItemFromEntityState(entity)

	if r.ToolID != original.ToolID {
		t.Errorf("ToolID = %q, want %q", r.ToolID, original.ToolID)
	}
	if r.Effect != nil {
		t.Error("Tool item should not have an Effect")
	}
}

func findStoreTriple(triples []message.Triple, predicate string) (message.Triple, bool) {
	for _, triple := range triples {
		if triple.Predicate == predicate {
			return triple, true
		}
	}
	return message.Triple{}, false
}
