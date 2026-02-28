package semdragons

import (
	"context"
	"errors"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// STORE EVENTS - Typed subjects for store operations
// =============================================================================
// All store event subjects use three-part vocabulary predicates.
// This enables NATS wildcard subscriptions like "store.item.>" for all
// store item events.
// =============================================================================

// --- Typed Subjects ---

var (
	// SubjectStoreItemListed is the typed subject for store.item.listed events.
	SubjectStoreItemListed = natsclient.NewSubject[StoreItemListedPayload](PredicateStoreItemListed)
	// SubjectStoreItemPurchased is the typed subject for store.item.purchased events.
	SubjectStoreItemPurchased = natsclient.NewSubject[StorePurchasePayload](PredicateStoreItemPurchased)
	// SubjectStoreItemUsed is the typed subject for store.item.used events (rental use).
	SubjectStoreItemUsed = natsclient.NewSubject[StoreItemUsedPayload](PredicateStoreItemUsed)
	// SubjectStoreItemExpired is the typed subject for store.item.expired events.
	SubjectStoreItemExpired = natsclient.NewSubject[StoreItemExpiredPayload](PredicateStoreItemExpired)

	// SubjectConsumableUsed is the typed subject for store.consumable.used events.
	SubjectConsumableUsed = natsclient.NewSubject[ConsumableUsedPayload](PredicateConsumableUsed)
	// SubjectConsumableExpired is the typed subject for store.consumable.expired events.
	SubjectConsumableExpired = natsclient.NewSubject[ConsumableExpiredPayload](PredicateConsumableExpired)

	// SubjectInventoryUpdated is the typed subject for agent.inventory.updated events.
	SubjectInventoryUpdated = natsclient.NewSubject[InventoryUpdatedPayload](PredicateInventoryUpdated)
)

// =============================================================================
// STORE ITEM PAYLOADS
// =============================================================================

// StoreItemListedPayload contains data for store.item.listed events.
type StoreItemListedPayload struct {
	Item     StoreItem `json:"item"`
	ListedBy string    `json:"listed_by,omitempty"` // Admin or system identifier
	ListedAt time.Time `json:"listed_at"`
	Trace    TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *StoreItemListedPayload) Validate() error {
	if p.Item.ID == "" {
		return errors.New("item_id required")
	}
	if p.ListedAt.IsZero() {
		return errors.New("listed_at required")
	}
	return nil
}

// StorePurchasePayload contains data for store.item.purchased events.
type StorePurchasePayload struct {
	AgentID     AgentID   `json:"agent_id"`
	ItemID      string    `json:"item_id"`
	ItemName    string    `json:"item_name"`
	ItemType    ItemType  `json:"item_type"`
	XPSpent     int64     `json:"xp_spent"`
	XPBefore    int64     `json:"xp_before"`
	XPAfter     int64     `json:"xp_after"`
	LevelBefore int       `json:"level_before"`
	Timestamp   time.Time `json:"timestamp"`
	Trace       TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// StoreItemUsedPayload contains data for store.item.used events (rental use consumed).
type StoreItemUsedPayload struct {
	AgentID       AgentID   `json:"agent_id"`
	ItemID        string    `json:"item_id"`
	ItemName      string    `json:"item_name"`
	UsesRemaining int       `json:"uses_remaining"`
	UsedFor       string    `json:"used_for,omitempty"` // What the use was for
	QuestID       *QuestID  `json:"quest_id,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Trace         TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// StoreItemExpiredPayload contains data for store.item.expired events.
type StoreItemExpiredPayload struct {
	AgentID   AgentID   `json:"agent_id"`
	ItemID    string    `json:"item_id"`
	ItemName  string    `json:"item_name"`
	Reason    string    `json:"reason"` // "uses_exhausted", "expired", etc.
	Timestamp time.Time `json:"timestamp"`
	Trace     TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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
// CONSUMABLE PAYLOADS
// =============================================================================

// ConsumableUsedPayload contains data for store.consumable.used events.
type ConsumableUsedPayload struct {
	AgentID      AgentID          `json:"agent_id"`
	ConsumableID string           `json:"consumable_id"`
	Effect       ConsumableEffect `json:"effect"`
	Remaining    int              `json:"remaining"`          // How many of this consumable left
	QuestID      *QuestID         `json:"quest_id,omitempty"` // If used on specific quest
	Timestamp    time.Time        `json:"timestamp"`
	Trace        TraceInfo        `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// ConsumableExpiredPayload contains data for store.consumable.expired events.
type ConsumableExpiredPayload struct {
	AgentID      AgentID          `json:"agent_id"`
	ConsumableID string           `json:"consumable_id"`
	Effect       ConsumableEffect `json:"effect"`
	Reason       string           `json:"reason"` // "quests_exhausted", "duration_expired", etc.
	Timestamp    time.Time        `json:"timestamp"`
	Trace        TraceInfo        `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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
// INVENTORY PAYLOADS
// =============================================================================

// InventoryUpdatedPayload contains data for agent.inventory.updated events.
type InventoryUpdatedPayload struct {
	AgentID    AgentID   `json:"agent_id"`
	ChangeType string    `json:"change_type"` // "purchase", "use", "expire", "grant"
	ItemID     string    `json:"item_id"`
	ItemName   string    `json:"item_name,omitempty"`
	Quantity   int       `json:"quantity"` // Change in quantity (negative for removals)
	TotalSpent int64     `json:"total_spent"`
	Timestamp  time.Time `json:"timestamp"`
	Trace      TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// =============================================================================
// EVENT PUBLISHER EXTENSIONS
// =============================================================================

// PublishStoreItemListed publishes a store.item.listed event.
func (ep *EventPublisher) PublishStoreItemListed(ctx context.Context, payload StoreItemListedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectStoreItemListed.Publish(ctx, ep.client, payload)
}

// PublishStorePurchase publishes a store.item.purchased event.
func (ep *EventPublisher) PublishStorePurchase(ctx context.Context, payload StorePurchasePayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectStoreItemPurchased.Publish(ctx, ep.client, payload)
}

// PublishStoreItemUsed publishes a store.item.used event.
func (ep *EventPublisher) PublishStoreItemUsed(ctx context.Context, payload StoreItemUsedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectStoreItemUsed.Publish(ctx, ep.client, payload)
}

// PublishStoreItemExpired publishes a store.item.expired event.
func (ep *EventPublisher) PublishStoreItemExpired(ctx context.Context, payload StoreItemExpiredPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectStoreItemExpired.Publish(ctx, ep.client, payload)
}

// PublishConsumableUsed publishes a store.consumable.used event.
func (ep *EventPublisher) PublishConsumableUsed(ctx context.Context, payload ConsumableUsedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectConsumableUsed.Publish(ctx, ep.client, payload)
}

// PublishConsumableExpired publishes a store.consumable.expired event.
func (ep *EventPublisher) PublishConsumableExpired(ctx context.Context, payload ConsumableExpiredPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectConsumableExpired.Publish(ctx, ep.client, payload)
}

// PublishInventoryUpdated publishes an agent.inventory.updated event.
func (ep *EventPublisher) PublishInventoryUpdated(ctx context.Context, payload InventoryUpdatedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectInventoryUpdated.Publish(ctx, ep.client, payload)
}
