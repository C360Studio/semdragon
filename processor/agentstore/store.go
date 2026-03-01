package agentstore

import (
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// STORE - The agent marketplace
// =============================================================================
// Agents spend XP to purchase tool access and consumables.
// Creates strategic trade-offs: level up OR acquire capabilities.
// Trust tier gates what items are visible and purchasable.
// =============================================================================

// ItemType categorizes store offerings.
type ItemType string

// Store item category values.
const (
	ItemTypeTool       ItemType = "tool"
	ItemTypeConsumable ItemType = "consumable"
)

// PurchaseType defines the ownership model for store items.
type PurchaseType string

// Item ownership model values.
const (
	PurchasePermanent PurchaseType = "permanent"
	PurchaseRental    PurchaseType = "rental"
)

// ConsumableType identifies specific consumable effects.
type ConsumableType string

// Consumable effect kind values.
const (
	ConsumableRetryToken    ConsumableType = "retry_token"
	ConsumableCooldownSkip  ConsumableType = "cooldown_skip"
	ConsumableXPBoost       ConsumableType = "xp_boost"
	ConsumableQualityShield ConsumableType = "quality_shield"
	ConsumableInsightScroll ConsumableType = "insight_scroll"
)

// =============================================================================
// STORE ITEM
// =============================================================================

// StoreItem represents something available for purchase in the store.
type StoreItem struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	ItemType     ItemType     `json:"item_type"`
	PurchaseType PurchaseType `json:"purchase_type"`

	// Pricing
	XPCost     int64 `json:"xp_cost"`
	RentalUses int   `json:"rental_uses,omitempty"`

	// Gating
	MinTier  domain.TrustTier `json:"min_tier"`
	MinLevel int              `json:"min_level,omitempty"`

	// For tools
	ToolID string `json:"tool_id,omitempty"`

	// For consumables
	Effect *ConsumableEffect `json:"effect,omitempty"`

	// Availability
	InStock       bool    `json:"in_stock"`
	GuildDiscount float64 `json:"guild_discount,omitempty"`
}

// ConsumableEffect defines what a consumable does when used.
type ConsumableEffect struct {
	Type      ConsumableType `json:"type"`
	Magnitude float64        `json:"magnitude,omitempty"`
	Duration  int            `json:"duration,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// =============================================================================
// INVENTORY & OWNERSHIP
// =============================================================================

// OwnedItem represents an agent's purchased item.
type OwnedItem struct {
	ItemID        string       `json:"item_id"`
	ItemName      string       `json:"item_name"`
	PurchaseType  PurchaseType `json:"purchase_type"`
	PurchasedAt   time.Time    `json:"purchased_at"`
	XPSpent       int64        `json:"xp_spent"`
	UsesRemaining int          `json:"uses_remaining,omitempty"`
}

// AgentInventory tracks what an agent owns.
type AgentInventory struct {
	AgentID     domain.AgentID       `json:"agent_id"`
	OwnedTools  map[string]OwnedItem `json:"owned_tools"`
	Consumables map[string]int       `json:"consumables"`
	TotalSpent  int64                `json:"total_spent"`
}

// NewAgentInventory creates an empty inventory for an agent.
func NewAgentInventory(agentID domain.AgentID) *AgentInventory {
	return &AgentInventory{
		AgentID:     agentID,
		OwnedTools:  make(map[string]OwnedItem),
		Consumables: make(map[string]int),
		TotalSpent:  0,
	}
}

// HasTool checks if the agent has a specific tool.
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

// =============================================================================
// ACTIVE EFFECTS
// =============================================================================

// ActiveEffect tracks a consumable effect currently in use.
type ActiveEffect struct {
	ConsumableID    string           `json:"consumable_id"`
	Effect          ConsumableEffect `json:"effect"`
	ActivatedAt     time.Time        `json:"activated_at"`
	QuestsRemaining int              `json:"quests_remaining"`
	QuestID         *domain.QuestID  `json:"quest_id,omitempty"`
}

// =============================================================================
// GRAPHABLE IMPLEMENTATIONS
// =============================================================================

// EntityID returns the entity ID for this store item.
func (s *StoreItem) EntityID() string {
	return "store.item." + s.ID
}

// Triples returns semantic facts about this store item.
func (s *StoreItem) Triples() []message.Triple {
	now := time.Now()
	source := "agent_store"
	entityID := s.EntityID()

	triples := []message.Triple{
		{Subject: entityID, Predicate: "store.item.name", Object: s.Name, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "store.item.description", Object: s.Description, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "store.item.type", Object: string(s.ItemType), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "store.item.purchase_type", Object: string(s.PurchaseType), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "store.item.xp_cost", Object: s.XPCost, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "store.item.min_tier", Object: int(s.MinTier), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "store.item.in_stock", Object: s.InStock, Source: source, Timestamp: now, Confidence: 1.0},
	}

	if s.ToolID != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "store.item.tool_id", Object: s.ToolID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// EntityID returns the entity ID for this inventory.
func (inv *AgentInventory) EntityID() string {
	return string(inv.AgentID)
}

// Triples returns semantic facts about this inventory.
func (inv *AgentInventory) Triples() []message.Triple {
	now := time.Now()
	source := "agent_store"
	entityID := inv.EntityID()

	triples := []message.Triple{
		{Subject: entityID, Predicate: "agent.inventory.total_spent", Object: inv.TotalSpent, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.inventory.tools_count", Object: len(inv.OwnedTools), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.inventory.consumables_count", Object: len(inv.Consumables), Source: source, Timestamp: now, Confidence: 1.0},
	}

	for toolID := range inv.OwnedTools {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.inventory.tool", Object: toolID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// =============================================================================
// STORE ITEM BUILDER
// =============================================================================

// StoreItemBuilder provides a fluent API for creating store items.
type StoreItemBuilder struct {
	item StoreItem
}

// NewStoreItem creates a new store item builder.
func NewStoreItem(id, name string) *StoreItemBuilder {
	return &StoreItemBuilder{
		item: StoreItem{
			ID:      id,
			Name:    name,
			InStock: true,
			MinTier: domain.TierApprentice,
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
	b.item.PurchaseType = PurchasePermanent
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

// RequireTier sets the minimum tier.
func (b *StoreItemBuilder) RequireTier(tier domain.TrustTier) *StoreItemBuilder {
	b.item.MinTier = tier
	return b
}

// RequireLevel sets the minimum level.
func (b *StoreItemBuilder) RequireLevel(level int) *StoreItemBuilder {
	b.item.MinLevel = level
	return b
}

// GuildDiscount sets the discount for guild members.
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

// =============================================================================
// DEFAULT CONSUMABLES CATALOG
// =============================================================================

// DefaultConsumables returns the standard set of consumable items.
func DefaultConsumables() []StoreItem {
	return []StoreItem{
		NewStoreItem("retry_token", "Retry Token").
			Description("Retry a failed quest without suffering the failure penalty").
			AsConsumable(ConsumableEffect{Type: ConsumableRetryToken, Duration: 1}).
			Cost(50).RequireTier(domain.TierApprentice).Build(),

		NewStoreItem("cooldown_skip", "Cooldown Skip").
			Description("Clear cooldown immediately and get back to questing").
			AsConsumable(ConsumableEffect{Type: ConsumableCooldownSkip}).
			Cost(75).RequireTier(domain.TierApprentice).Build(),

		NewStoreItem("xp_boost", "XP Boost").
			Description("Earn 2x XP on your next completed quest").
			AsConsumable(ConsumableEffect{Type: ConsumableXPBoost, Magnitude: 2.0, Duration: 1}).
			Cost(100).RequireTier(domain.TierJourneyman).Build(),

		NewStoreItem("quality_shield", "Quality Shield").
			Description("Ignore one failed review criterion during boss battle").
			AsConsumable(ConsumableEffect{Type: ConsumableQualityShield, Duration: 1}).
			Cost(150).RequireTier(domain.TierJourneyman).Build(),

		NewStoreItem("insight_scroll", "Insight Scroll").
			Description("See detailed difficulty hints before claiming a quest").
			AsConsumable(ConsumableEffect{Type: ConsumableInsightScroll, Duration: 3}).
			Cost(50).RequireTier(domain.TierApprentice).Build(),
	}
}
