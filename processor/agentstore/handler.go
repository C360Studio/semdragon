package agentstore

import (
	"context"
	"errors"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// KV WATCH HANDLER - Entity-centric agent state monitoring
// =============================================================================
// Replaces the NATS Subscribe pattern for detecting agent XP changes.
// "Agent XP changed" is a fact about the world — it lives in KV, not in
// JetStream request subjects.
//
// The watch goroutine caches each agent's last-known XP and logs whenever
// XP changes unlock new store items.  Purchase and consumable-use flows
// remain request-driven (callers invoke Purchase / UseConsumable directly).
// =============================================================================

// processAgentWatchUpdates handles agent entity state changes from KV.
func (c *Component) processAgentWatchUpdates() {
	defer close(c.watchDoneCh)

	for {
		select {
		case <-c.stopChan:
			return
		case entry, ok := <-c.agentWatch.Updates():
			if !ok {
				return
			}
			if entry == nil {
				continue // Initial sync complete
			}
			c.handleAgentUpdate(entry)
		}
	}
}

// handleAgentUpdate processes an agent entity state change from KV.
// Keys in the ENTITY_STATES bucket use the full 6-part entity ID format:
// org.platform.game.board.agent.instance (e.g., test.integration.game.board1.agent.abc123)
func (c *Component) handleAgentUpdate(entry jetstream.KeyValueEntry) {
	if !c.running.Load() {
		return
	}

	key := entry.Key()
	instance := semdragons.ExtractInstance(key)
	if instance == "" || instance == key {
		// Key did not contain a dot separator — not a valid entity ID.
		c.logger.Warn("agent watch entry has unexpected key format", "key", key)
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.agentXPCache.Delete(key)
		c.logger.Debug("agent removed from XP cache", "instance", instance)
		return
	}

	// Decode entity state and reconstruct the Agent from its triples.
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		c.logger.Warn("failed to decode agent entity state", "instance", instance, "error", err)
		return
	}
	agent := semdragons.AgentFromEntityState(entityState)
	if agent == nil {
		c.logger.Warn("failed to reconstruct agent from entity state", "instance", instance)
		return
	}

	// Detect XP changes by diffing against the cached value.
	prevXP, hadPrev := c.agentXPCache.Load(key)
	c.agentXPCache.Store(key, agent.XP)

	if !hadPrev {
		// First time we see this agent — seed the cache, no action needed.
		return
	}

	if prevXP.(int64) == agent.XP {
		return // No XP change, nothing to react to.
	}

	c.lastActivity.Store(time.Now())
	c.logger.Debug("agent XP updated",
		"instance", instance,
		"xp_before", prevXP.(int64),
		"xp_after", agent.XP,
		"level", agent.Level,
		"tier", agent.Tier)

	// Log newly affordable items so the agent (or a DM) can act on them.
	c.logNewlyAffordableItems(agent)
}

// logNewlyAffordableItems logs store items that became affordable after an XP change.
// This is observability — the actual purchase remains an explicit caller action.
func (c *Component) logNewlyAffordableItems(agent *semdragons.Agent) {
	affordable := c.ListItems(agent.Tier)
	for _, item := range affordable {
		if agent.XP >= item.XPCost {
			c.logger.Debug("item now affordable",
				"agent", agent.ID,
				"item_id", item.ID,
				"item_name", item.Name,
				"xp_cost", item.XPCost,
				"agent_xp", agent.XP)
		}
	}
}

// AgentXP returns the last-known XP for an agent from the KV cache.
// Returns 0 and false if the agent has not been observed yet.
func (c *Component) AgentXP(agentID domain.AgentID) (int64, bool) {
	// The cache key is the full entity ID stored in the KV bucket.
	// We need to search by instance suffix since we cache by full key.
	instance := semdragons.ExtractInstance(string(agentID))
	var found int64
	var ok bool
	c.agentXPCache.Range(func(k, v any) bool {
		key := k.(string)
		if semdragons.ExtractInstance(key) == instance {
			found = v.(int64)
			ok = true
			return false // Stop iteration
		}
		return true
	})
	return found, ok
}

// AgentXPFromGraph reads the current XP for an agent directly from KV.
// This is useful when the watch cache may not yet have a value for new agents.
func (c *Component) AgentXPFromGraph(ctx context.Context, agentID domain.AgentID) (int64, int, error) {
	entity, err := c.graph.GetAgent(ctx, agentID)
	if err != nil {
		return 0, 0, errs.Wrap(err, "agent_store", "AgentXPFromGraph", "get agent")
	}
	agent := semdragons.AgentFromEntityState(entity)
	if agent == nil {
		return 0, 0, errors.New("agent not found")
	}
	return agent.XP, agent.Level, nil
}

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

// Purchase buys an item for an agent. If the agent is in any guild and the item
// has a GuildDiscount > 0, the effective cost is reduced accordingly.
func (c *Component) Purchase(ctx context.Context, agentID domain.AgentID, itemID string, currentXP int64, currentLevel int, agentGuilds []domain.GuildID) (*OwnedItem, error) {
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

	// Apply guild discount if the agent is in any guild and the item offers one.
	effectiveCost := item.XPCost
	if item.GuildDiscount > 0 && len(agentGuilds) > 0 {
		effectiveCost = int64(float64(item.XPCost) * (1.0 - item.GuildDiscount))
		if effectiveCost < 0 {
			effectiveCost = 0
		}
	}

	// Check affordability
	if currentXP < effectiveCost {
		return nil, errors.New("insufficient XP")
	}

	now := time.Now()
	newXP := currentXP - effectiveCost

	// Create owned item
	owned := &OwnedItem{
		ItemID:       itemID,
		ItemName:     item.Name,
		PurchaseType: item.PurchaseType,
		PurchasedAt:  now,
		XPSpent:      effectiveCost,
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
	inv.TotalSpent += effectiveCost

	// Publish purchase event
	if err := SubjectStoreItemPurchased.Publish(ctx, c.deps.NATSClient, StorePurchasePayload{
		AgentID:     agentID,
		ItemID:      itemID,
		ItemName:    item.Name,
		ItemType:    item.ItemType,
		XPSpent:     effectiveCost,
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
		"xp_spent", effectiveCost,
		"guild_discount_applied", item.GuildDiscount > 0 && len(agentGuilds) > 0)

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
