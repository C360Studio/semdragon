package agentstore

import (
	"context"
	"errors"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
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
	instance := domain.ExtractInstance(key)
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
	agent := agentprogression.AgentFromEntityState(entityState)
	if agent == nil {
		c.logger.Warn("failed to reconstruct agent from entity state", "instance", instance)
		return
	}

	// Always sync inventory/effects from KV — catches mutations by other
	// processors (autonomy, bossbattle) and rehydrates on first observation.
	c.syncInventoryFromAgent(agent)

	// Detect XP changes by diffing against the cached value.
	prevXP, hadPrev := c.agentXPCache.Load(key)
	c.agentXPCache.Store(key, agent.XP)

	if !hadPrev || prevXP.(int64) == agent.XP {
		return // First observation or no XP change — no affordability check needed.
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

// loadInitialInventories loads all agent entities from KV and populates the
// inventory, active effects, and XP caches. Follows the boidengine
// loadInitialState() pattern — called once during Start() so caches survive
// restarts.
func (c *Component) loadInitialInventories(ctx context.Context) error {
	const agentLimit = 500
	agentEntities, err := c.graph.ListAgentsByPrefix(ctx, agentLimit)
	if err != nil {
		return err
	}

	if len(agentEntities) == agentLimit {
		c.logger.Warn("agent limit reached during initial inventory load; some agents may not be rehydrated until watcher catches up",
			"limit", agentLimit)
	}

	loaded := 0
	for _, entity := range agentEntities {
		agent := agentprogression.AgentFromEntityState(&entity)
		if agent == nil {
			continue
		}

		// Seed XP cache (keyed by full entity ID, same as the watcher)
		c.agentXPCache.Store(string(agent.ID), agent.XP)

		// Populate inventory and effects caches from agent entity state
		c.syncInventoryFromAgent(agent)
		loaded++
	}

	c.logger.Debug("loaded initial inventories from KV", "agents_loaded", loaded)
	return nil
}

// syncInventoryFromAgent merges inventory and active effects from a
// reconstructed Agent into the local caches. Uses merge (not replace) because
// the watcher may reflect our own stale KV writes back — a full replacement
// would overwrite more recent local state from Purchase/UseConsumable calls.
func (c *Component) syncInventoryFromAgent(agent *agentprogression.Agent) {
	hasInventory := len(agent.OwnedTools) > 0 || len(agent.Consumables) > 0 || agent.TotalSpent > 0
	hasEffects := len(agent.ActiveEffects) > 0

	if !hasInventory && !hasEffects {
		return
	}

	if hasInventory {
		inv := c.getOrCreateInventory(agent.ID)
		inv.mu.Lock()

		// Merge tools — add/update from agent entity, preserve local-only entries.
		for id, tool := range agent.OwnedTools {
			// -1 = permanent (domain convention); 0+ = rental (may be exhausted)
			purchaseType := PurchasePermanent
			if tool.UsesRemaining >= 0 {
				purchaseType = PurchaseRental
			}
			var itemName string
			if val, ok := c.catalog.Load(id); ok {
				itemName = val.(*StoreItem).Name
			}
			inv.OwnedTools[id] = OwnedItem{
				ItemID:        id,
				ItemName:      itemName,
				PurchaseType:  purchaseType,
				PurchasedAt:   tool.PurchasedAt,
				XPSpent:       tool.XPSpent,
				UsesRemaining: tool.UsesRemaining,
			}
		}

		// Merge consumables — use max of local vs agent entity count to avoid
		// overwriting a recent local Purchase with a stale reflected write.
		for id, count := range agent.Consumables {
			if count > inv.Consumables[id] {
				inv.Consumables[id] = count
			}
		}

		// TotalSpent: use the higher value (local may have a more recent purchase).
		if agent.TotalSpent > inv.TotalSpent {
			inv.TotalSpent = agent.TotalSpent
		}
		inv.mu.Unlock()
	}

	if hasEffects {
		var effects []ActiveEffect
		for _, ae := range agent.ActiveEffects {
			effect := ActiveEffect{
				ConsumableID:    ae.EffectType, // effect type doubles as consumable ID
				QuestsRemaining: ae.QuestsRemaining,
				QuestID:         ae.QuestID,
			}
			// Look up catalog for full effect details
			if val, ok := c.catalog.Load(ae.EffectType); ok {
				item := val.(*StoreItem)
				if item.Effect != nil {
					effect.Effect = *item.Effect
				}
			}
			effects = append(effects, effect)
		}
		c.activeEffects.Store(agent.ID, effects)
	}
}

// logNewlyAffordableItems logs store items that became affordable after an XP change.
// This is observability — the actual purchase remains an explicit caller action.
func (c *Component) logNewlyAffordableItems(agent *agentprogression.Agent) {
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
	instance := domain.ExtractInstance(string(agentID))
	var found int64
	var ok bool
	c.agentXPCache.Range(func(k, v any) bool {
		key := k.(string)
		if domain.ExtractInstance(key) == instance {
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
	agent := agentprogression.AgentFromEntityState(entity)
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

	item.BoardConfig = c.boardConfig
	c.catalog.Store(item.ID, &item)
	now := time.Now()

	// Write item to KV as a storeitem entity
	if err := c.graph.EmitEntityUpdate(ctx, &item, "store.item.listed"); err != nil {
		c.logger.Warn("failed to write store item to KV", "item_id", item.ID, "error", err)
	}

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

// SeedCatalog loads items into the in-memory catalog without requiring the
// component to be running. Useful for tests and pre-start configuration.
func (c *Component) SeedCatalog(items []StoreItem) {
	for i := range items {
		item := items[i]
		item.BoardConfig = c.boardConfig
		c.catalog.Store(item.ID, &item)
	}
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

// Purchase buys an item for an agent. Reads agent entity from KV, mutates
// inventory + XP, and writes back via EmitEntityUpdate. If the agent is in any
// guild and the item has a GuildDiscount > 0, the effective cost is reduced.
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

	// Update in-memory inventory cache (fast path for local reads)
	inv := c.getOrCreateInventory(agentID)
	inv.mu.Lock()
	if item.ItemType == ItemTypeTool {
		inv.OwnedTools[itemID] = *owned
	} else if item.ItemType == ItemTypeConsumable {
		inv.Consumables[itemID]++
	}
	inv.TotalSpent += effectiveCost
	totalSpent := inv.TotalSpent // capture under lock for event payload
	inv.mu.Unlock()

	// Read-modify-write agent entity to KV (source of truth)
	agentEntity, agentErr := c.graph.GetAgent(ctx, agentID)
	if agentErr == nil && agentEntity != nil {
		agent := agentprogression.AgentFromEntityState(agentEntity)
		if agent != nil {
			// Ensure maps are initialized
			if agent.OwnedTools == nil {
				agent.OwnedTools = make(map[string]agentprogression.OwnedTool)
			}
			if agent.Consumables == nil {
				agent.Consumables = make(map[string]int)
			}

			// Mutate inventory on agent entity
			if item.ItemType == ItemTypeTool {
				usesRemaining := -1 // permanent
				if item.PurchaseType == PurchaseRental {
					usesRemaining = item.RentalUses
				}
				agent.OwnedTools[itemID] = agentprogression.OwnedTool{
					StoreItemID:   item.EntityID(),
					XPSpent:       effectiveCost,
					UsesRemaining: usesRemaining,
					PurchasedAt:   now,
				}
			} else if item.ItemType == ItemTypeConsumable {
				agent.Consumables[itemID]++
			}

			// Mutate XP and stats
			agent.XP -= effectiveCost
			agent.TotalSpent += effectiveCost
			agent.Stats.TotalXPSpent += effectiveCost
			agent.UpdatedAt = now

			if writeErr := c.graph.EmitEntityUpdate(ctx, agent, "agent.inventory.purchased"); writeErr != nil {
				c.errorsCount.Add(1)
				c.logger.Error("failed to write agent entity after purchase", "error", writeErr)
			}
		}
	}

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
		TotalSpent: totalSpent,
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
// Uses LoadOrStore to avoid TOCTOU races when the watcher goroutine and
// API handlers access the same agent concurrently.
func (c *Component) getOrCreateInventory(agentID domain.AgentID) *AgentInventory {
	if val, ok := c.inventories.Load(agentID); ok {
		return val.(*AgentInventory)
	}
	inv := NewAgentInventory(agentID)
	val, _ := c.inventories.LoadOrStore(agentID, inv)
	return val.(*AgentInventory)
}

// =============================================================================
// CONSUMABLE HANDLERS
// =============================================================================

// UseConsumable activates a consumable for an agent. Reads agent entity from KV,
// decrements consumable count, adds active effect, and writes back.
func (c *Component) UseConsumable(ctx context.Context, agentID domain.AgentID, consumableID string, questID *domain.QuestID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	inv := c.getOrCreateInventory(agentID)

	// Check and decrement under lock to prevent races with the watcher.
	inv.mu.Lock()
	if inv.Consumables[consumableID] <= 0 {
		inv.mu.Unlock()
		return errors.New("consumable not owned")
	}
	inv.Consumables[consumableID]--
	remaining := inv.Consumables[consumableID]
	inv.mu.Unlock()

	// Get consumable item for effect info
	val, ok := c.catalog.Load(consumableID)
	if !ok {
		return errors.New("consumable not found in catalog")
	}
	item := val.(*StoreItem)

	// Create active effect (in-memory cache)
	effect := ActiveEffect{
		ConsumableID:    consumableID,
		Effect:          *item.Effect,
		ActivatedAt:     time.Now(),
		QuestsRemaining: item.Effect.Duration,
		QuestID:         questID,
	}
	c.addActiveEffect(agentID, effect)

	now := time.Now()

	// Read-modify-write agent entity to KV (source of truth)
	agentEntity, agentErr := c.graph.GetAgent(ctx, agentID)
	if agentErr == nil && agentEntity != nil {
		agent := agentprogression.AgentFromEntityState(agentEntity)
		if agent != nil {
			if agent.Consumables == nil {
				agent.Consumables = make(map[string]int)
			}

			// Decrement consumable count on agent entity
			agent.Consumables[consumableID]--
			if agent.Consumables[consumableID] <= 0 {
				delete(agent.Consumables, consumableID)
			}

			// Add active effect to agent entity
			agent.ActiveEffects = append(agent.ActiveEffects, agentprogression.AgentEffect{
				EffectType:      string(item.Effect.Type),
				QuestsRemaining: item.Effect.Duration,
				QuestID:         questID,
			})

			// Handle cooldown_skip: transition agent back to idle
			if item.Effect.Type == ConsumableCooldownSkip && agent.Status == domain.AgentCooldown {
				agent.Status = domain.AgentIdle
				agent.CooldownUntil = nil
			}

			agent.UpdatedAt = now
			eventType := "agent.consumable.used"
			if item.Effect.Type == ConsumableCooldownSkip && agent.Status == domain.AgentIdle {
				eventType = "agent.status.idle"
			}
			if writeErr := c.graph.EmitEntityUpdate(ctx, agent, eventType); writeErr != nil {
				c.errorsCount.Add(1)
				c.logger.Error("failed to write agent entity after consumable use", "error", writeErr)
			}
		}
	}

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
