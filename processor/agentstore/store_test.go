package agentstore

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
)

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

func TestStoreItem_EntityID_FallbackWithoutBoardConfig(t *testing.T) {
	item := StoreItem{
		ID:   "web_search",
		Name: "Web Search",
	}

	entityID := item.EntityID()
	want := "store.item.web_search"
	if entityID != want {
		t.Errorf("EntityID() without BoardConfig = %q, want %q", entityID, want)
	}
}

func TestStoreItem_Triples_AllFields(t *testing.T) {
	bc := &domain.BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "board1",
	}

	item := StoreItem{
		ID:           "xp_boost",
		Name:         "XP Boost",
		Description:  "Doubles XP",
		ItemType:     ItemTypeConsumable,
		PurchaseType: PurchasePermanent,
		XPCost:       100,
		RentalUses:   0,
		MinTier:      domain.TierJourneyman,
		MinLevel:     6,
		InStock:      true,
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
		"store.item.purchase_type":    false,
		"store.item.xp_cost":         false,
		"store.item.min_tier":        false,
		"store.item.in_stock":        false,
		"store.item.rental_uses":     false,
		"store.item.min_level":       false,
		"store.item.guild_discount":  false,
		"store.item.effect_type":     false,
		"store.item.effect_magnitude": false,
		"store.item.effect_duration": false,
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
		if triple.Predicate == "store.item.tool_id" {
			foundToolID = true
			if got := triple.Object.(string); got != "web_search" {
				t.Errorf("store.item.tool_id = %q, want %q", got, "web_search")
			}
		}
	}
	if !foundToolID {
		t.Error("store.item.tool_id triple not found")
	}
}

func TestStoreItemFromEntityState(t *testing.T) {
	bc := &domain.BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "board1",
	}

	original := StoreItem{
		ID:           "xp_boost",
		Name:         "XP Boost",
		Description:  "Doubles XP on next quest",
		ItemType:     ItemTypeConsumable,
		PurchaseType: PurchasePermanent,
		XPCost:       100,
		MinTier:      domain.TierJourneyman,
		MinLevel:     6,
		InStock:      true,
		GuildDiscount: 0.15,
		RentalUses:   0,
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

func TestStoreItemFromEntityState_ToolRoundTrip(t *testing.T) {
	bc := &domain.BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "board1",
	}

	original := StoreItem{
		ID:           "deploy_access",
		Name:         "Deploy Access",
		Description:  "Permission to deploy to staging",
		ItemType:     ItemTypeTool,
		PurchaseType: PurchasePermanent,
		XPCost:       500,
		MinTier:      domain.TierExpert,
		MinLevel:     11,
		ToolID:       "deploy_access",
		InStock:      true,
		GuildDiscount: 0.20,
		BoardConfig:  bc,
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
