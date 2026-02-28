package semdragons

import (
	"context"
	"time"
)

// =============================================================================
// STORE - The agent marketplace
// =============================================================================
// Agents spend XP to purchase tool access and consumables.
// Creates strategic trade-offs: level up OR acquire capabilities.
// Trust tier gates what items are visible and purchasable.
// =============================================================================

// -----------------------------------------------------------------------------
// Item Types & Categories
// -----------------------------------------------------------------------------

// ItemType categorizes store offerings.
type ItemType string

const (
	// ItemTypeTool represents permanent or rental tool access.
	ItemTypeTool ItemType = "tool"
	// ItemTypeConsumable represents one-time use items.
	ItemTypeConsumable ItemType = "consumable"
)

// PurchaseType defines the ownership model for store items.
type PurchaseType string

const (
	// PurchasePermanent indicates the item is owned forever.
	PurchasePermanent PurchaseType = "permanent"
	// PurchaseRental indicates the item has limited uses.
	PurchaseRental PurchaseType = "rental"
)

// ConsumableType identifies specific consumable effects.
type ConsumableType string

const (
	// ConsumableRetryToken allows retrying a failed quest without penalty.
	ConsumableRetryToken ConsumableType = "retry_token"
	// ConsumableCooldownSkip clears cooldown immediately.
	ConsumableCooldownSkip ConsumableType = "cooldown_skip"
	// ConsumableXPBoost provides 2x XP on next quest.
	ConsumableXPBoost ConsumableType = "xp_boost"
	// ConsumableQualityShield ignores one failed review criterion.
	ConsumableQualityShield ConsumableType = "quality_shield"
	// ConsumableInsightScroll reveals quest difficulty hints before claiming.
	ConsumableInsightScroll ConsumableType = "insight_scroll"
)

// -----------------------------------------------------------------------------
// Store Item
// -----------------------------------------------------------------------------

// StoreItem represents something available for purchase in the store.
type StoreItem struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	ItemType     ItemType     `json:"item_type"`
	PurchaseType PurchaseType `json:"purchase_type"`

	// Pricing
	XPCost     int64 `json:"xp_cost"`
	RentalUses int   `json:"rental_uses,omitempty"` // If rental, how many uses

	// Gating
	MinTier  TrustTier `json:"min_tier"`            // Must be this tier to see/buy
	MinLevel int       `json:"min_level,omitempty"` // Optional level requirement

	// For tools
	ToolID string `json:"tool_id,omitempty"` // Reference to Tool definition

	// For consumables
	Effect *ConsumableEffect `json:"effect,omitempty"`

	// Availability
	InStock       bool    `json:"in_stock"`
	GuildDiscount float64 `json:"guild_discount,omitempty"` // 0.0-1.0 discount for guild members
}

// ConsumableEffect defines what a consumable does when used.
type ConsumableEffect struct {
	Type      ConsumableType         `json:"type"`               // retry_token, cooldown_skip, xp_boost, quality_shield
	Magnitude float64                `json:"magnitude,omitempty"` // e.g., 2.0 for 2x XP boost
	Duration  int                    `json:"duration,omitempty"`  // Number of quests it applies to
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// -----------------------------------------------------------------------------
// Inventory & Ownership
// -----------------------------------------------------------------------------

// OwnedItem represents an agent's purchased item.
type OwnedItem struct {
	ItemID       string       `json:"item_id"`
	ItemName     string       `json:"item_name"`
	PurchaseType PurchaseType `json:"purchase_type"`
	PurchasedAt  time.Time    `json:"purchased_at"`
	XPSpent      int64        `json:"xp_spent"`

	// For rentals
	UsesRemaining int `json:"uses_remaining,omitempty"`
}

// AgentInventory tracks what an agent owns.
type AgentInventory struct {
	AgentID     AgentID              `json:"agent_id"`
	OwnedTools  map[string]OwnedItem `json:"owned_tools"`  // tool_id -> ownership
	Consumables map[string]int       `json:"consumables"`  // consumable_id -> quantity
	TotalSpent  int64                `json:"total_spent"`  // Lifetime XP spent in store
}

// NewAgentInventory creates an empty inventory for an agent.
func NewAgentInventory(agentID AgentID) *AgentInventory {
	return &AgentInventory{
		AgentID:     agentID,
		OwnedTools:  make(map[string]OwnedItem),
		Consumables: make(map[string]int),
		TotalSpent:  0,
	}
}

// HasTool checks if the agent has a specific tool (permanent or rental with uses).
func (inv *AgentInventory) HasTool(toolID string) bool {
	owned, ok := inv.OwnedTools[toolID]
	if !ok {
		return false
	}
	if owned.PurchaseType == PurchaseRental {
		return owned.UsesRemaining > 0
	}
	return true
}

// HasConsumable checks if the agent has at least one of a consumable.
func (inv *AgentInventory) HasConsumable(consumableID string) bool {
	return inv.Consumables[consumableID] > 0
}

// ConsumableCount returns how many of a consumable the agent has.
func (inv *AgentInventory) ConsumableCount(consumableID string) int {
	return inv.Consumables[consumableID]
}

// -----------------------------------------------------------------------------
// Active Effects
// -----------------------------------------------------------------------------

// ActiveEffect tracks a consumable effect currently in use.
type ActiveEffect struct {
	ConsumableID    string           `json:"consumable_id"`
	Effect          ConsumableEffect `json:"effect"`
	ActivatedAt     time.Time        `json:"activated_at"`
	QuestsRemaining int              `json:"quests_remaining"` // How many quests until expired
	QuestID         *QuestID         `json:"quest_id,omitempty"` // If applied to specific quest
}

// -----------------------------------------------------------------------------
// Store Interface
// -----------------------------------------------------------------------------

// Store manages the agent marketplace.
type Store interface {
	// --- Browsing ---

	// ListItems returns items available to an agent (filtered by tier).
	ListItems(ctx context.Context, agentID AgentID) ([]StoreItem, error)

	// GetItem returns a specific item by ID.
	GetItem(ctx context.Context, itemID string) (*StoreItem, error)

	// --- Purchasing ---

	// Purchase buys an item for an agent. Returns the owned item.
	Purchase(ctx context.Context, agentID AgentID, itemID string) (*OwnedItem, error)

	// CanAfford checks if an agent can afford an item.
	// Returns (can_afford, xp_shortfall, error).
	CanAfford(ctx context.Context, agentID AgentID, itemID string) (bool, int64, error)

	// --- Inventory ---

	// GetInventory returns an agent's inventory.
	GetInventory(ctx context.Context, agentID AgentID) (*AgentInventory, error)

	// HasTool checks if an agent has access to a tool.
	HasTool(ctx context.Context, agentID AgentID, toolID string) (bool, error)

	// --- Consumables ---

	// UseConsumable activates a consumable for an agent.
	UseConsumable(ctx context.Context, agentID AgentID, consumableID string, questID *QuestID) error

	// GetActiveEffects returns consumable effects currently active for an agent.
	GetActiveEffects(ctx context.Context, agentID AgentID) ([]ActiveEffect, error)

	// ConsumeEffect marks an effect as used (decrements quests remaining).
	ConsumeEffect(ctx context.Context, agentID AgentID, effectType ConsumableType) error

	// --- Admin ---

	// AddItem adds a new item to the store catalog.
	AddItem(ctx context.Context, item StoreItem) error

	// UpdateItem updates an existing item in the store catalog.
	UpdateItem(ctx context.Context, item StoreItem) error

	// SetStock sets whether an item is in stock.
	SetStock(ctx context.Context, itemID string, inStock bool) error

	// --- Catalog ---

	// Catalog returns all items in the store (admin view, no tier filtering).
	Catalog(ctx context.Context) ([]StoreItem, error)
}

// -----------------------------------------------------------------------------
// Store Item Builder
// -----------------------------------------------------------------------------

// StoreItemBuilder provides a fluent API for creating store items.
type StoreItemBuilder struct {
	item StoreItem
}

// NewStoreItem creates a new store item builder.
func NewStoreItem(id, name string) *StoreItemBuilder {
	return &StoreItemBuilder{
		item: StoreItem{
			ID:       id,
			Name:     name,
			InStock:  true,
			MinTier:  TierApprentice,
		},
	}
}

// Description sets the item description.
func (b *StoreItemBuilder) Description(desc string) *StoreItemBuilder {
	b.item.Description = desc
	return b
}

// AsTool marks the item as a tool.
func (b *StoreItemBuilder) AsTool(toolID string) *StoreItemBuilder {
	b.item.ItemType = ItemTypeTool
	b.item.ToolID = toolID
	return b
}

// AsConsumable marks the item as a consumable.
func (b *StoreItemBuilder) AsConsumable(effect ConsumableEffect) *StoreItemBuilder {
	b.item.ItemType = ItemTypeConsumable
	b.item.PurchaseType = PurchasePermanent // Consumables are "owned" until used
	b.item.Effect = &effect
	return b
}

// Permanent marks the item as permanently owned.
func (b *StoreItemBuilder) Permanent() *StoreItemBuilder {
	b.item.PurchaseType = PurchasePermanent
	return b
}

// Rental marks the item as rental with limited uses.
func (b *StoreItemBuilder) Rental(uses int) *StoreItemBuilder {
	b.item.PurchaseType = PurchaseRental
	b.item.RentalUses = uses
	return b
}

// Cost sets the XP cost.
func (b *StoreItemBuilder) Cost(xp int64) *StoreItemBuilder {
	b.item.XPCost = xp
	return b
}

// RequireTier sets the minimum tier to see/buy this item.
func (b *StoreItemBuilder) RequireTier(tier TrustTier) *StoreItemBuilder {
	b.item.MinTier = tier
	return b
}

// RequireLevel sets the minimum level to see/buy this item.
func (b *StoreItemBuilder) RequireLevel(level int) *StoreItemBuilder {
	b.item.MinLevel = level
	return b
}

// GuildDiscount sets the discount for guild members (0.0-1.0).
func (b *StoreItemBuilder) GuildDiscount(discount float64) *StoreItemBuilder {
	b.item.GuildDiscount = discount
	return b
}

// OutOfStock marks the item as unavailable.
func (b *StoreItemBuilder) OutOfStock() *StoreItemBuilder {
	b.item.InStock = false
	return b
}

// Build returns the constructed store item.
func (b *StoreItemBuilder) Build() StoreItem {
	return b.item
}

// -----------------------------------------------------------------------------
// Default Consumables Catalog
// -----------------------------------------------------------------------------

// DefaultConsumables returns the standard set of consumable items.
func DefaultConsumables() []StoreItem {
	return []StoreItem{
		NewStoreItem("retry_token", "Retry Token").
			Description("Retry a failed quest without suffering the failure penalty").
			AsConsumable(ConsumableEffect{
				Type:      ConsumableRetryToken,
				Duration:  1,
			}).
			Cost(50).
			RequireTier(TierApprentice).
			Build(),

		NewStoreItem("cooldown_skip", "Cooldown Skip").
			Description("Clear cooldown immediately and get back to questing").
			AsConsumable(ConsumableEffect{
				Type:      ConsumableCooldownSkip,
			}).
			Cost(75).
			RequireTier(TierApprentice).
			Build(),

		NewStoreItem("xp_boost", "XP Boost").
			Description("Earn 2x XP on your next completed quest").
			AsConsumable(ConsumableEffect{
				Type:      ConsumableXPBoost,
				Magnitude: 2.0,
				Duration:  1,
			}).
			Cost(100).
			RequireTier(TierJourneyman).
			Build(),

		NewStoreItem("quality_shield", "Quality Shield").
			Description("Ignore one failed review criterion during boss battle").
			AsConsumable(ConsumableEffect{
				Type:      ConsumableQualityShield,
				Duration:  1,
			}).
			Cost(150).
			RequireTier(TierJourneyman).
			Build(),

		NewStoreItem("insight_scroll", "Insight Scroll").
			Description("See detailed difficulty hints before claiming a quest").
			AsConsumable(ConsumableEffect{
				Type:      ConsumableInsightScroll,
				Duration:  3,
			}).
			Cost(50).
			RequireTier(TierApprentice).
			Build(),
	}
}
