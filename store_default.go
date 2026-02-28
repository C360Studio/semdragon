package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// DEFAULT STORE - NATS KV-backed implementation
// =============================================================================
// The store uses the same KV bucket as other semdragons state.
// Key patterns:
//   store.item.{item_id}                  - Store item catalog entry
//   inventory.{agent_instance}            - Agent inventory
//   effects.{agent_instance}              - Active consumable effects
// =============================================================================

// DefaultStore implements the Store interface backed by NATS KV.
type DefaultStore struct {
	storage   *Storage
	publisher *EventPublisher
	xpEngine  XPEngine
	logger    *slog.Logger
}

// NewDefaultStore creates a new store with the given storage backend.
func NewDefaultStore(storage *Storage, publisher *EventPublisher, xpEngine XPEngine) *DefaultStore {
	return &DefaultStore{
		storage:   storage,
		publisher: publisher,
		xpEngine:  xpEngine,
		logger:    slog.Default(),
	}
}

// WithLogger sets a custom logger.
func (s *DefaultStore) WithLogger(l *slog.Logger) *DefaultStore {
	s.logger = l
	return s
}

// --- Key Generation ---

// StoreItemKey returns the KV key for a store item.
func (s *DefaultStore) StoreItemKey(itemID string) string {
	return fmt.Sprintf("store.item.%s", itemID)
}

// InventoryKey returns the KV key for an agent's inventory.
func (s *DefaultStore) InventoryKey(agentInstance string) string {
	return fmt.Sprintf("inventory.%s", agentInstance)
}

// ActiveEffectsKey returns the KV key for an agent's active effects.
func (s *DefaultStore) ActiveEffectsKey(agentInstance string) string {
	return fmt.Sprintf("effects.%s", agentInstance)
}

// --- Catalog Operations ---

// AddItem adds a new item to the store catalog.
func (s *DefaultStore) AddItem(ctx context.Context, item StoreItem) error {
	key := s.StoreItemKey(item.ID)
	data, err := json.Marshal(item)
	if err != nil {
		return errs.Wrap(err, "DefaultStore", "AddItem", "marshal")
	}
	_, err = s.storage.KV().Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "DefaultStore", "AddItem", "put")
	}

	// Publish event
	if s.publisher != nil {
		if err := s.publisher.PublishStoreItemListed(ctx, StoreItemListedPayload{
			Item:     item,
			ListedBy: "system",
			ListedAt: time.Now(),
		}); err != nil {
			s.logger.Warn("failed to publish store.item.listed event", "item_id", item.ID, "error", err)
		}
	}

	return nil
}

// UpdateItem updates an existing item in the store catalog.
func (s *DefaultStore) UpdateItem(ctx context.Context, item StoreItem) error {
	key := s.StoreItemKey(item.ID)
	data, err := json.Marshal(item)
	if err != nil {
		return errs.Wrap(err, "DefaultStore", "UpdateItem", "marshal")
	}
	_, err = s.storage.KV().Put(ctx, key, data)
	if err != nil {
		return errs.Wrap(err, "DefaultStore", "UpdateItem", "put")
	}
	return nil
}

// SetStock sets whether an item is in stock.
func (s *DefaultStore) SetStock(ctx context.Context, itemID string, inStock bool) error {
	key := s.StoreItemKey(itemID)
	return s.storage.KV().UpdateWithRetry(ctx, key, func(current []byte) ([]byte, error) {
		if len(current) == 0 {
			return nil, fmt.Errorf("item not found: %s", itemID)
		}
		var item StoreItem
		if err := json.Unmarshal(current, &item); err != nil {
			return nil, err
		}
		item.InStock = inStock
		return json.Marshal(&item)
	})
}

// GetItem returns a specific item by ID.
func (s *DefaultStore) GetItem(ctx context.Context, itemID string) (*StoreItem, error) {
	key := s.StoreItemKey(itemID)
	entry, err := s.storage.KV().Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return nil, fmt.Errorf("item not found: %s", itemID)
		}
		return nil, errs.Wrap(err, "DefaultStore", "GetItem", "get")
	}

	var item StoreItem
	if err := json.Unmarshal(entry.Value, &item); err != nil {
		return nil, errs.Wrap(err, "DefaultStore", "GetItem", "unmarshal")
	}
	return &item, nil
}

// Catalog returns all items in the store.
func (s *DefaultStore) Catalog(ctx context.Context) ([]StoreItem, error) {
	keys, err := s.storage.ListIndexKeys(ctx, "store.item.")
	if err != nil {
		return nil, errs.Wrap(err, "DefaultStore", "Catalog", "list keys")
	}

	items := make([]StoreItem, 0, len(keys))
	for _, key := range keys {
		entry, err := s.storage.KV().Get(ctx, key)
		if err != nil {
			s.logger.Debug("failed to load store item", "key", key, "error", err)
			continue
		}
		var item StoreItem
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			s.logger.Debug("failed to unmarshal store item", "key", key, "error", err)
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// ListItems returns items available to an agent (filtered by tier).
func (s *DefaultStore) ListItems(ctx context.Context, agentID AgentID) ([]StoreItem, error) {
	// Get agent to check tier
	agentInstance := s.extractInstance(string(agentID))
	agent, err := s.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		return nil, errs.Wrap(err, "DefaultStore", "ListItems", "get agent")
	}

	allItems, err := s.Catalog(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by tier and availability
	filtered := make([]StoreItem, 0)
	for _, item := range allItems {
		if item.MinTier > agent.Tier {
			continue // Agent can't see this item
		}
		if item.MinLevel > 0 && agent.Level < item.MinLevel {
			continue // Agent doesn't meet level requirement
		}
		if !item.InStock {
			continue // Out of stock
		}
		filtered = append(filtered, item)
	}

	return filtered, nil
}

// --- Purchasing ---

// CanAfford checks if an agent can afford an item.
func (s *DefaultStore) CanAfford(ctx context.Context, agentID AgentID, itemID string) (bool, int64, error) {
	agentInstance := s.extractInstance(string(agentID))
	agent, err := s.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		return false, 0, errs.Wrap(err, "DefaultStore", "CanAfford", "get agent")
	}

	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return false, 0, err
	}

	// Calculate effective cost (with guild discount)
	effectiveCost := s.calculateCost(item, agent)

	if agent.XP >= effectiveCost {
		return true, 0, nil
	}
	return false, effectiveCost - agent.XP, nil
}

// calculateCost returns the effective cost considering guild discounts.
func (s *DefaultStore) calculateCost(item *StoreItem, agent *Agent) int64 {
	if item.GuildDiscount == 0 {
		return item.XPCost
	}

	// Check if agent is in any guild that would apply the discount
	// For now, apply discount if agent is in any guild
	if len(agent.Guilds) > 0 {
		discounted := float64(item.XPCost) * (1.0 - item.GuildDiscount)
		return int64(discounted)
	}
	return item.XPCost
}

// Purchase buys an item for an agent.
// Uses UpdateAgent for atomic XP deduction to prevent race conditions.
func (s *DefaultStore) Purchase(ctx context.Context, agentID AgentID, itemID string) (*OwnedItem, error) {
	agentInstance := s.extractInstance(string(agentID))

	// Get item first (read-only, can be done outside transaction)
	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}

	// Get current inventory for validation (needed for duplicate check)
	inv, err := s.GetInventory(ctx, agentID)
	if err != nil {
		return nil, errs.Wrap(err, "DefaultStore", "Purchase", "get inventory")
	}

	// Track values for event publishing
	var xpBefore, xpAfter int64
	var levelBefore int
	var cost int64

	// Atomically update agent XP using UpdateWithRetry
	err = s.storage.UpdateAgent(ctx, agentInstance, func(agent *Agent) error {
		// Validate purchase (tier, level, stock, duplicate check)
		if err := s.validatePurchase(agent, item, inv); err != nil {
			return err
		}

		// Calculate cost (may include guild discounts)
		cost = s.calculateCost(item, agent)

		// Check affordability
		if agent.XP < cost {
			return fmt.Errorf("insufficient XP: have %d, need %d", agent.XP, cost)
		}

		// Record state for events
		xpBefore = agent.XP
		levelBefore = agent.Level

		// Deduct XP atomically
		if err := s.spendXP(agent, cost, "store purchase: "+item.ID); err != nil {
			return err
		}
		xpAfter = agent.XP

		return nil
	})
	if err != nil {
		return nil, errs.Wrap(err, "DefaultStore", "Purchase", "update agent")
	}

	// Create owned item
	owned := &OwnedItem{
		ItemID:       item.ID,
		ItemName:     item.Name,
		PurchaseType: item.PurchaseType,
		PurchasedAt:  time.Now(),
		XPSpent:      cost,
	}
	if item.PurchaseType == PurchaseRental {
		owned.UsesRemaining = item.RentalUses
	}

	// Update inventory (separate KV operation, but idempotent)
	if err := s.addToInventory(ctx, agentInstance, item, owned); err != nil {
		// XP was already deducted - log error but don't fail
		// In production, would want compensation or retry queue
		s.logger.Error("failed to update inventory after XP deduction",
			"agent", agentID, "item", itemID, "cost", cost, "error", err)
		return nil, errs.Wrap(err, "DefaultStore", "Purchase", "add to inventory")
	}

	// Publish events (best effort)
	if s.publisher != nil {
		if err := s.publisher.PublishStorePurchase(ctx, StorePurchasePayload{
			AgentID:     agentID,
			ItemID:      item.ID,
			ItemName:    item.Name,
			ItemType:    item.ItemType,
			XPSpent:     cost,
			XPBefore:    xpBefore,
			XPAfter:     xpAfter,
			LevelBefore: levelBefore,
			Timestamp:   time.Now(),
		}); err != nil {
			s.logger.Warn("failed to publish store.item.purchased event", "agent", agentID, "item", itemID, "error", err)
		}

		if err := s.publisher.PublishInventoryUpdated(ctx, InventoryUpdatedPayload{
			AgentID:    agentID,
			ChangeType: "purchase",
			ItemID:     item.ID,
			ItemName:   item.Name,
			Quantity:   1,
			TotalSpent: cost,
			Timestamp:  time.Now(),
		}); err != nil {
			s.logger.Warn("failed to publish agent.inventory.updated event", "agent", agentID, "item", itemID, "error", err)
		}
	}

	return owned, nil
}

// validatePurchase checks if an agent can purchase an item.
func (s *DefaultStore) validatePurchase(agent *Agent, item *StoreItem, inv *AgentInventory) error {
	if !item.InStock {
		return fmt.Errorf("item %s is out of stock", item.ID)
	}
	if item.MinTier > agent.Tier {
		return fmt.Errorf("agent tier %d insufficient for item (requires %d)", agent.Tier, item.MinTier)
	}
	if item.MinLevel > 0 && agent.Level < item.MinLevel {
		return fmt.Errorf("agent level %d insufficient for item (requires %d)", agent.Level, item.MinLevel)
	}
	// Prevent duplicate permanent tool purchases
	if item.ItemType == ItemTypeTool && item.PurchaseType == PurchasePermanent {
		if _, exists := inv.OwnedTools[item.ToolID]; exists {
			return fmt.Errorf("agent already owns tool %s", item.ToolID)
		}
	}
	return nil
}

// spendXP deducts XP from an agent. Does not affect level.
func (s *DefaultStore) spendXP(agent *Agent, amount int64, reason string) error {
	return s.xpEngine.SpendXP(agent, amount, reason)
}

// addToInventory adds an item to an agent's inventory.
func (s *DefaultStore) addToInventory(ctx context.Context, agentInstance string, item *StoreItem, owned *OwnedItem) error {
	key := s.InventoryKey(agentInstance)

	return s.storage.KV().UpdateWithRetry(ctx, key, func(current []byte) ([]byte, error) {
		var inv AgentInventory
		if len(current) > 0 {
			if err := json.Unmarshal(current, &inv); err != nil {
				return nil, err
			}
		} else {
			// Initialize new inventory
			inv = AgentInventory{
				AgentID:     AgentID(s.storage.Config().AgentEntityID(agentInstance)),
				OwnedTools:  make(map[string]OwnedItem),
				Consumables: make(map[string]int),
			}
		}

		// Add item based on type
		switch item.ItemType {
		case ItemTypeTool:
			inv.OwnedTools[item.ToolID] = *owned
		case ItemTypeConsumable:
			inv.Consumables[item.ID]++
		}

		inv.TotalSpent += owned.XPSpent

		return json.Marshal(&inv)
	})
}

// --- Inventory ---

// GetInventory returns an agent's inventory.
func (s *DefaultStore) GetInventory(ctx context.Context, agentID AgentID) (*AgentInventory, error) {
	agentInstance := s.extractInstance(string(agentID))
	key := s.InventoryKey(agentInstance)

	entry, err := s.storage.KV().Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			// Return empty inventory
			return NewAgentInventory(agentID), nil
		}
		return nil, errs.Wrap(err, "DefaultStore", "GetInventory", "get")
	}

	var inv AgentInventory
	if err := json.Unmarshal(entry.Value, &inv); err != nil {
		return nil, errs.Wrap(err, "DefaultStore", "GetInventory", "unmarshal")
	}
	return &inv, nil
}

// HasTool checks if an agent has access to a tool.
func (s *DefaultStore) HasTool(ctx context.Context, agentID AgentID, toolID string) (bool, error) {
	inv, err := s.GetInventory(ctx, agentID)
	if err != nil {
		return false, err
	}
	return inv.HasTool(toolID), nil
}

// --- Consumables ---

// UseConsumable activates a consumable for an agent.
func (s *DefaultStore) UseConsumable(ctx context.Context, agentID AgentID, consumableID string, questID *QuestID) error {
	agentInstance := s.extractInstance(string(agentID))

	// Get the consumable item definition
	item, err := s.GetItem(ctx, consumableID)
	if err != nil {
		return errs.Wrap(err, "DefaultStore", "UseConsumable", "get item")
	}
	if item.Effect == nil {
		return fmt.Errorf("item %s has no consumable effect", consumableID)
	}

	// Deduct from inventory
	var remaining int
	invKey := s.InventoryKey(agentInstance)
	err = s.storage.KV().UpdateWithRetry(ctx, invKey, func(current []byte) ([]byte, error) {
		if len(current) == 0 {
			return nil, fmt.Errorf("no inventory for agent")
		}
		var inv AgentInventory
		if err := json.Unmarshal(current, &inv); err != nil {
			return nil, err
		}
		if inv.Consumables[consumableID] <= 0 {
			return nil, fmt.Errorf("agent has no %s consumable", consumableID)
		}
		inv.Consumables[consumableID]--
		remaining = inv.Consumables[consumableID]
		return json.Marshal(&inv)
	})
	if err != nil {
		return errs.Wrap(err, "DefaultStore", "UseConsumable", "deduct from inventory")
	}

	// Add to active effects if it has duration
	if item.Effect.Duration > 0 {
		if err := s.addActiveEffect(ctx, agentInstance, item, questID); err != nil {
			s.logger.Warn("failed to add active effect", "agent", agentID, "consumable", consumableID, "error", err)
		}
	}

	// Publish event
	if s.publisher != nil {
		if err := s.publisher.PublishConsumableUsed(ctx, ConsumableUsedPayload{
			AgentID:      agentID,
			ConsumableID: consumableID,
			Effect:       *item.Effect,
			Remaining:    remaining,
			QuestID:      questID,
			Timestamp:    time.Now(),
		}); err != nil {
			s.logger.Warn("failed to publish store.consumable.used event", "agent", agentID, "consumable", consumableID, "error", err)
		}
	}

	return nil
}

// addActiveEffect adds a consumable effect to an agent's active effects.
func (s *DefaultStore) addActiveEffect(ctx context.Context, agentInstance string, item *StoreItem, questID *QuestID) error {
	key := s.ActiveEffectsKey(agentInstance)

	return s.storage.KV().UpdateWithRetry(ctx, key, func(current []byte) ([]byte, error) {
		var effects []ActiveEffect
		if len(current) > 0 {
			if err := json.Unmarshal(current, &effects); err != nil {
				return nil, err
			}
		}

		effect := ActiveEffect{
			ConsumableID:    item.ID,
			Effect:          *item.Effect,
			ActivatedAt:     time.Now(),
			QuestsRemaining: item.Effect.Duration,
			QuestID:         questID,
		}
		effects = append(effects, effect)

		return json.Marshal(effects)
	})
}

// GetActiveEffects returns consumable effects currently active for an agent.
func (s *DefaultStore) GetActiveEffects(ctx context.Context, agentID AgentID) ([]ActiveEffect, error) {
	agentInstance := s.extractInstance(string(agentID))
	key := s.ActiveEffectsKey(agentInstance)

	entry, err := s.storage.KV().Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return []ActiveEffect{}, nil
		}
		return nil, errs.Wrap(err, "DefaultStore", "GetActiveEffects", "get")
	}

	var effects []ActiveEffect
	if err := json.Unmarshal(entry.Value, &effects); err != nil {
		return nil, errs.Wrap(err, "DefaultStore", "GetActiveEffects", "unmarshal")
	}
	return effects, nil
}

// ConsumeEffect marks an effect as used (decrements quests remaining).
func (s *DefaultStore) ConsumeEffect(ctx context.Context, agentID AgentID, effectType ConsumableType) error {
	agentInstance := s.extractInstance(string(agentID))
	key := s.ActiveEffectsKey(agentInstance)

	var expiredEffect *ActiveEffect

	err := s.storage.KV().UpdateWithRetry(ctx, key, func(current []byte) ([]byte, error) {
		if len(current) == 0 {
			return nil, fmt.Errorf("no active effects for agent")
		}
		var effects []ActiveEffect
		if err := json.Unmarshal(current, &effects); err != nil {
			return nil, err
		}

		// Find and consume matching effect
		newEffects := make([]ActiveEffect, 0, len(effects))
		consumed := false
		for i := range effects {
			if !consumed && effects[i].Effect.Type == effectType {
				effects[i].QuestsRemaining--
				if effects[i].QuestsRemaining <= 0 {
					// Effect expired
					expiredEffect = &effects[i]
					consumed = true
					continue // Don't add to new effects
				}
				consumed = true
			}
			newEffects = append(newEffects, effects[i])
		}

		if !consumed {
			return nil, fmt.Errorf("no active effect of type %s", effectType)
		}

		return json.Marshal(newEffects)
	})

	if err != nil {
		return errs.Wrap(err, "DefaultStore", "ConsumeEffect", "update")
	}

	// Publish expiration event if effect expired
	if expiredEffect != nil && s.publisher != nil {
		if err := s.publisher.PublishConsumableExpired(ctx, ConsumableExpiredPayload{
			AgentID:      agentID,
			ConsumableID: expiredEffect.ConsumableID,
			Effect:       expiredEffect.Effect,
			Reason:       "quests_exhausted",
			Timestamp:    time.Now(),
		}); err != nil {
			s.logger.Warn("failed to publish store.consumable.expired event", "agent", agentID, "error", err)
		}
	}

	return nil
}

// --- Helpers ---

// extractInstance extracts the instance ID from a full entity ID.
// Entity IDs are: org.platform.domain.system.type.instance
func (s *DefaultStore) extractInstance(entityID string) string {
	parts := strings.Split(entityID, ".")
	if len(parts) >= 6 {
		return parts[len(parts)-1]
	}
	return entityID // Fall back to using full ID as instance
}

// InitializeCatalog adds the default consumables to the store.
func (s *DefaultStore) InitializeCatalog(ctx context.Context) error {
	for _, item := range DefaultConsumables() {
		if err := s.AddItem(ctx, item); err != nil {
			return errs.Wrap(err, "DefaultStore", "InitializeCatalog", "add item")
		}
	}
	return nil
}
