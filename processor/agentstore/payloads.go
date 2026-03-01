package agentstore

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// STORE PAYLOADS
// =============================================================================
// Event payloads for store operations. Each implements graph.Graphable
// for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*StoreItemListedPayload)(nil)
	_ graph.Graphable = (*StorePurchasePayload)(nil)
	_ graph.Graphable = (*StoreItemUsedPayload)(nil)
	_ graph.Graphable = (*StoreItemExpiredPayload)(nil)
	_ graph.Graphable = (*ConsumableUsedPayload)(nil)
	_ graph.Graphable = (*ConsumableExpiredPayload)(nil)
	_ graph.Graphable = (*InventoryUpdatedPayload)(nil)
)

// Typed subjects for store lifecycle events.
var (
	SubjectStoreItemListed    = natsclient.NewSubject[StoreItemListedPayload](domain.PredicateStoreItemListed)
	SubjectStoreItemPurchased = natsclient.NewSubject[StorePurchasePayload](domain.PredicateStoreItemPurchased)
	SubjectStoreItemUsed      = natsclient.NewSubject[StoreItemUsedPayload](domain.PredicateStoreItemUsed)
	SubjectStoreItemExpired   = natsclient.NewSubject[StoreItemExpiredPayload](domain.PredicateStoreItemExpired)
	SubjectConsumableUsed     = natsclient.NewSubject[ConsumableUsedPayload](domain.PredicateConsumableUsed)
	SubjectConsumableExpired  = natsclient.NewSubject[ConsumableExpiredPayload](domain.PredicateConsumableExpired)
	SubjectInventoryUpdated   = natsclient.NewSubject[InventoryUpdatedPayload](domain.PredicateInventoryUpdated)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// STORE ITEM LISTED PAYLOAD
// =============================================================================

// StoreItemListedPayload contains data for store.item.listed events.
type StoreItemListedPayload struct {
	Item     StoreItem `json:"item"`
	ListedBy string    `json:"listed_by,omitempty"`
	ListedAt time.Time `json:"listed_at"`
	Trace    TraceInfo `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *StoreItemListedPayload) EntityID() string { return p.Item.EntityID() }

// Triples returns semantic triples for this event.
func (p *StoreItemListedPayload) Triples() []message.Triple {
	return p.Item.Triples()
}

// Schema returns the type schema for this payload.
func (p *StoreItemListedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "store.listed", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *StoreItemListedPayload) Validate() error {
	if p.Item.ID == "" {
		return errors.New("item_id required")
	}
	if p.ListedAt.IsZero() {
		return errors.New("listed_at required")
	}
	return nil
}

// =============================================================================
// STORE PURCHASE PAYLOAD
// =============================================================================

// StorePurchasePayload contains data for store.item.purchased events.
type StorePurchasePayload struct {
	AgentID     domain.AgentID `json:"agent_id"`
	ItemID      string         `json:"item_id"`
	ItemName    string         `json:"item_name"`
	ItemType    ItemType       `json:"item_type"`
	XPSpent     int64          `json:"xp_spent"`
	XPBefore    int64          `json:"xp_before"`
	XPAfter     int64          `json:"xp_after"`
	LevelBefore int            `json:"level_before"`
	Timestamp   time.Time      `json:"timestamp"`
	Trace       TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *StorePurchasePayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *StorePurchasePayload) Triples() []message.Triple {
	source := "agent_store"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.purchase.item_id", Object: p.ItemID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.purchase.xp_spent", Object: p.XPSpent, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.current", Object: p.XPAfter, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *StorePurchasePayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "store.purchased", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *StorePurchasePayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.ItemID == "" {
		return errors.New("item_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// STORE ITEM USED PAYLOAD
// =============================================================================

// StoreItemUsedPayload contains data for store.item.used events.
type StoreItemUsedPayload struct {
	AgentID       domain.AgentID  `json:"agent_id"`
	ItemID        string          `json:"item_id"`
	ItemName      string          `json:"item_name"`
	UsesRemaining int             `json:"uses_remaining"`
	UsedFor       string          `json:"used_for,omitempty"`
	QuestID       *domain.QuestID `json:"quest_id,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
	Trace         TraceInfo       `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *StoreItemUsedPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *StoreItemUsedPayload) Triples() []message.Triple {
	source := "agent_store"
	entityID := string(p.AgentID)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "agent.item.used", Object: p.ItemID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.item.uses_remaining", Object: p.UsesRemaining, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}

	if p.QuestID != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.item.used_for_quest", Object: string(*p.QuestID),
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the type schema for this payload.
func (p *StoreItemUsedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "store.used", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *StoreItemUsedPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.ItemID == "" {
		return errors.New("item_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// STORE ITEM EXPIRED PAYLOAD
// =============================================================================

// StoreItemExpiredPayload contains data for store.item.expired events.
type StoreItemExpiredPayload struct {
	AgentID   domain.AgentID `json:"agent_id"`
	ItemID    string         `json:"item_id"`
	ItemName  string         `json:"item_name"`
	Reason    string         `json:"reason"`
	Timestamp time.Time      `json:"timestamp"`
	Trace     TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *StoreItemExpiredPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *StoreItemExpiredPayload) Triples() []message.Triple {
	source := "agent_store"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.item.expired", Object: p.ItemID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.item.expired_reason", Object: p.Reason, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *StoreItemExpiredPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "store.expired", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *StoreItemExpiredPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.ItemID == "" {
		return errors.New("item_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// CONSUMABLE USED PAYLOAD
// =============================================================================

// ConsumableUsedPayload contains data for store.consumable.used events.
type ConsumableUsedPayload struct {
	AgentID      domain.AgentID   `json:"agent_id"`
	ConsumableID string           `json:"consumable_id"`
	Effect       ConsumableEffect `json:"effect"`
	Remaining    int              `json:"remaining"`
	QuestID      *domain.QuestID  `json:"quest_id,omitempty"`
	Timestamp    time.Time        `json:"timestamp"`
	Trace        TraceInfo        `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *ConsumableUsedPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *ConsumableUsedPayload) Triples() []message.Triple {
	source := "agent_store"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.consumable.used", Object: p.ConsumableID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.consumable.remaining", Object: p.Remaining, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.consumable.effect_type", Object: string(p.Effect.Type), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *ConsumableUsedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "consumable.used", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *ConsumableUsedPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.ConsumableID == "" {
		return errors.New("consumable_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// CONSUMABLE EXPIRED PAYLOAD
// =============================================================================

// ConsumableExpiredPayload contains data for store.consumable.expired events.
type ConsumableExpiredPayload struct {
	AgentID      domain.AgentID   `json:"agent_id"`
	ConsumableID string           `json:"consumable_id"`
	Effect       ConsumableEffect `json:"effect"`
	Reason       string           `json:"reason"`
	Timestamp    time.Time        `json:"timestamp"`
	Trace        TraceInfo        `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *ConsumableExpiredPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *ConsumableExpiredPayload) Triples() []message.Triple {
	source := "agent_store"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.consumable.expired", Object: p.ConsumableID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.consumable.expired_reason", Object: p.Reason, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *ConsumableExpiredPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "consumable.expired", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *ConsumableExpiredPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.ConsumableID == "" {
		return errors.New("consumable_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// INVENTORY UPDATED PAYLOAD
// =============================================================================

// InventoryUpdatedPayload contains data for agent.inventory.updated events.
type InventoryUpdatedPayload struct {
	AgentID    domain.AgentID `json:"agent_id"`
	ChangeType string         `json:"change_type"` // purchase, use, expire, grant
	ItemID     string         `json:"item_id"`
	ItemName   string         `json:"item_name,omitempty"`
	Quantity   int            `json:"quantity"`
	TotalSpent int64          `json:"total_spent"`
	Timestamp  time.Time      `json:"timestamp"`
	Trace      TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *InventoryUpdatedPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *InventoryUpdatedPayload) Triples() []message.Triple {
	source := "agent_store"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.inventory.change_type", Object: p.ChangeType, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.inventory.item_id", Object: p.ItemID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.inventory.quantity_change", Object: p.Quantity, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.inventory.total_spent", Object: p.TotalSpent, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *InventoryUpdatedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "inventory.updated", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *InventoryUpdatedPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.ItemID == "" {
		return errors.New("item_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
