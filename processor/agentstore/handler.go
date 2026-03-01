package agentstore

import (
	"context"
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// CATALOG HANDLERS
// =============================================================================

// AddItem adds a new item to the store catalog.
func (c *Component) AddItem(ctx context.Context, item StoreItem) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.catalog.Store(item.ID, &item)
	now := time.Now()

	// Publish event - downstream graph-ingest will persist if configured
	if err := SubjectStoreItemListed.Publish(ctx, c.deps.NATSClient, StoreItemListedPayload{
		Item:     item,
		ListedAt: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "AgentStore", "AddItem", "publish item listed")
	}

	c.itemsListed.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("item added to store",
		"item_id", item.ID,
		"item_name", item.Name,
		"xp_cost", item.XPCost)

	return nil
}

// GetItem returns a specific item by ID.
func (c *Component) GetItem(itemID string) (*StoreItem, bool) {
	val, ok := c.catalog.Load(itemID)
	if !ok {
		return nil, false
	}
	return val.(*StoreItem), true
}

// ListItems returns all items available to an agent (filtered by tier).
func (c *Component) ListItems(agentTier domain.TrustTier) []StoreItem {
	var items []StoreItem
	c.catalog.Range(func(_, value any) bool {
		item := value.(*StoreItem)
		if item.InStock && item.MinTier <= agentTier {
			items = append(items, *item)
		}
		return true
	})
	return items
}

// Catalog returns all items in the store (admin view, no tier filtering).
func (c *Component) Catalog() []StoreItem {
	var items []StoreItem
	c.catalog.Range(func(_, value any) bool {
		items = append(items, *value.(*StoreItem))
		return true
	})
	return items
}

// SetStock sets whether an item is in stock.
func (c *Component) SetStock(itemID string, inStock bool) error {
	val, ok := c.catalog.Load(itemID)
	if !ok {
		return errors.New("item not found")
	}

	item := val.(*StoreItem)
	item.InStock = inStock
	c.catalog.Store(itemID, item)

	return nil
}

// =============================================================================
// PURCHASE HANDLERS
// =============================================================================

// Purchase buys an item for an agent.
func (c *Component) Purchase(ctx context.Context, agentID domain.AgentID, itemID string, currentXP int64, currentLevel int) (*OwnedItem, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	// Get item
	val, ok := c.catalog.Load(itemID)
	if !ok {
		return nil, errors.New("item not found")
	}
	item := val.(*StoreItem)

	// Check if in stock
	if !item.InStock {
		return nil, errors.New("item out of stock")
	}

	// Check affordability
	if currentXP < item.XPCost {
		return nil, errors.New("insufficient XP")
	}

	now := time.Now()
	newXP := currentXP - item.XPCost

	// Create owned item
	owned := &OwnedItem{
		ItemID:       itemID,
		ItemName:     item.Name,
		PurchaseType: item.PurchaseType,
		PurchasedAt:  now,
		XPSpent:      item.XPCost,
	}

	if item.PurchaseType == PurchaseRental {
		owned.UsesRemaining = item.RentalUses
	}

	// Update inventory
	inv := c.getOrCreateInventory(agentID)
	if item.ItemType == ItemTypeTool {
		inv.OwnedTools[itemID] = *owned
	} else if item.ItemType == ItemTypeConsumable {
		inv.Consumables[itemID]++
	}
	inv.TotalSpent += item.XPCost

	// Publish purchase event
	if err := SubjectStoreItemPurchased.Publish(ctx, c.deps.NATSClient, StorePurchasePayload{
		AgentID:     agentID,
		ItemID:      itemID,
		ItemName:    item.Name,
		ItemType:    item.ItemType,
		XPSpent:     item.XPCost,
		XPBefore:    currentXP,
		XPAfter:     newXP,
		LevelBefore: currentLevel,
		Timestamp:   now,
	}); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "AgentStore", "Purchase", "publish purchase")
	}

	// Publish inventory update
	if err := SubjectInventoryUpdated.Publish(ctx, c.deps.NATSClient, InventoryUpdatedPayload{
		AgentID:    agentID,
		ChangeType: "purchase",
		ItemID:     itemID,
		ItemName:   item.Name,
		Quantity:   1,
		TotalSpent: inv.TotalSpent,
		Timestamp:  now,
	}); err != nil {
		c.errorsCount.Add(1)
		// Don't fail the purchase for inventory event failure
	}

	c.purchasesComplete.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("item purchased",
		"agent_id", agentID,
		"item_id", itemID,
		"xp_spent", item.XPCost)

	return owned, nil
}

// CanAfford checks if an agent can afford an item.
func (c *Component) CanAfford(itemID string, currentXP int64) (bool, int64) {
	val, ok := c.catalog.Load(itemID)
	if !ok {
		return false, 0
	}

	item := val.(*StoreItem)
	if currentXP >= item.XPCost {
		return true, 0
	}
	return false, item.XPCost - currentXP
}

// =============================================================================
// INVENTORY HANDLERS
// =============================================================================

// GetInventory returns an agent's inventory.
func (c *Component) GetInventory(agentID domain.AgentID) *AgentInventory {
	return c.getOrCreateInventory(agentID)
}

// HasTool checks if an agent has access to a tool.
func (c *Component) HasTool(agentID domain.AgentID, toolID string) bool {
	inv := c.getOrCreateInventory(agentID)
	return inv.HasTool(toolID)
}

// getOrCreateInventory gets or creates an inventory for an agent.
func (c *Component) getOrCreateInventory(agentID domain.AgentID) *AgentInventory {
	val, ok := c.inventories.Load(agentID)
	if ok {
		return val.(*AgentInventory)
	}

	inv := NewAgentInventory(agentID)
	c.inventories.Store(agentID, inv)
	return inv
}

// =============================================================================
// CONSUMABLE HANDLERS
// =============================================================================

// UseConsumable activates a consumable for an agent.
func (c *Component) UseConsumable(ctx context.Context, agentID domain.AgentID, consumableID string, questID *domain.QuestID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	inv := c.getOrCreateInventory(agentID)

	// Check if has consumable
	if !inv.HasConsumable(consumableID) {
		return errors.New("consumable not owned")
	}

	// Get consumable item for effect info
	val, ok := c.catalog.Load(consumableID)
	if !ok {
		return errors.New("consumable not found in catalog")
	}
	item := val.(*StoreItem)

	// Decrement count
	inv.Consumables[consumableID]--
	remaining := inv.Consumables[consumableID]

	// Create active effect
	effect := ActiveEffect{
		ConsumableID:    consumableID,
		Effect:          *item.Effect,
		ActivatedAt:     time.Now(),
		QuestsRemaining: item.Effect.Duration,
		QuestID:         questID,
	}

	// Store active effect
	c.addActiveEffect(agentID, effect)

	now := time.Now()

	// Publish consumable used event
	if err := SubjectConsumableUsed.Publish(ctx, c.deps.NATSClient, ConsumableUsedPayload{
		AgentID:      agentID,
		ConsumableID: consumableID,
		Effect:       *item.Effect,
		Remaining:    remaining,
		QuestID:      questID,
		Timestamp:    now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "AgentStore", "UseConsumable", "publish consumable used")
	}

	c.consumablesUsed.Add(1)
	c.lastActivity.Store(now)

	c.logger.Info("consumable used",
		"agent_id", agentID,
		"consumable_id", consumableID,
		"effect_type", item.Effect.Type)

	return nil
}

// GetActiveEffects returns consumable effects currently active for an agent.
func (c *Component) GetActiveEffects(agentID domain.AgentID) []ActiveEffect {
	val, ok := c.activeEffects.Load(agentID)
	if !ok {
		return nil
	}
	return val.([]ActiveEffect)
}

// ConsumeEffect marks an effect as used (decrements quests remaining).
func (c *Component) ConsumeEffect(ctx context.Context, agentID domain.AgentID, effectType ConsumableType) error {
	val, ok := c.activeEffects.Load(agentID)
	if !ok {
		return nil // No active effects
	}

	effects := val.([]ActiveEffect)
	var remaining []ActiveEffect

	for _, effect := range effects {
		if effect.Effect.Type == effectType {
			effect.QuestsRemaining--
			if effect.QuestsRemaining > 0 {
				remaining = append(remaining, effect)
			} else {
				// Effect expired
				if err := SubjectConsumableExpired.Publish(ctx, c.deps.NATSClient, ConsumableExpiredPayload{
					AgentID:      agentID,
					ConsumableID: effect.ConsumableID,
					Effect:       effect.Effect,
					Reason:       "quests_exhausted",
					Timestamp:    time.Now(),
				}); err != nil {
					c.errorsCount.Add(1)
					// Don't fail for event failure
				}
			}
		} else {
			remaining = append(remaining, effect)
		}
	}

	if len(remaining) > 0 {
		c.activeEffects.Store(agentID, remaining)
	} else {
		c.activeEffects.Delete(agentID)
	}

	return nil
}

// addActiveEffect adds an effect to an agent's active effects.
func (c *Component) addActiveEffect(agentID domain.AgentID, effect ActiveEffect) {
	val, ok := c.activeEffects.Load(agentID)
	var effects []ActiveEffect
	if ok {
		effects = val.([]ActiveEffect)
	}
	effects = append(effects, effect)
	c.activeEffects.Store(agentID, effects)
}

// HasActiveEffect checks if an agent has an active effect of a specific type.
func (c *Component) HasActiveEffect(agentID domain.AgentID, effectType ConsumableType) bool {
	effects := c.GetActiveEffects(agentID)
	for _, e := range effects {
		if e.Effect.Type == effectType {
			return true
		}
	}
	return false
}
