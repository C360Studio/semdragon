<script lang="ts">
	/**
	 * InventoryPanel - Display agent's owned items and consumables
	 */

	import type { AgentInventory, OwnedItem } from '$types';
	import ConsumableSlot from './ConsumableSlot.svelte';

	interface InventoryPanelProps {
		/** Agent's inventory */
		inventory: AgentInventory | null;
		/** Handler for using a consumable */
		onUseConsumable?: (consumableId: string) => void;
	}

	let { inventory, onUseConsumable }: InventoryPanelProps = $props();

	// Convert owned tools to array
	const ownedTools = $derived(
		inventory ? Object.values(inventory.owned_tools) : []
	);

	// Convert consumables to array of [id, quantity] pairs
	const consumableList = $derived(
		inventory ? Object.entries(inventory.consumables).filter(([_, qty]) => qty > 0) : []
	);

	// Check if inventory is empty
	const isEmpty = $derived(ownedTools.length === 0 && consumableList.length === 0);
</script>

<div class="inventory-panel" data-testid="inventory-panel">
	{#if !inventory}
		<div class="loading-state">
			<p>Loading inventory...</p>
		</div>
	{:else if isEmpty}
		<div class="empty-state">
			<p>Your inventory is empty</p>
			<p class="hint">Purchase items from the store to see them here</p>
		</div>
	{:else}
		{#if consumableList.length > 0}
			<section class="inventory-section" aria-labelledby="consumables-heading">
				<h3 id="consumables-heading" class="section-title">Consumables</h3>
				<div class="consumables-list">
					{#each consumableList as [consumableId, quantity]}
						<ConsumableSlot
							{consumableId}
							{quantity}
							onUse={() => onUseConsumable?.(consumableId)}
						/>
					{/each}
				</div>
			</section>
		{/if}

		{#if ownedTools.length > 0}
			<section class="inventory-section" aria-labelledby="tools-heading">
				<h3 id="tools-heading" class="section-title">Tools</h3>
				<div class="tools-list">
					{#each ownedTools as tool (tool.item_id)}
						<div class="tool-item">
							<div class="tool-info">
								<span class="tool-icon" aria-hidden="true">🔧</span>
								<span class="tool-name">{tool.item_name}</span>
							</div>
							<div class="tool-meta">
								{#if tool.purchase_type === 'rental' && tool.uses_remaining !== undefined}
									<span class="uses-badge">{tool.uses_remaining} uses left</span>
								{:else}
									<span class="owned-badge">Owned</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</section>
		{/if}

		<div class="inventory-footer">
			<span class="total-spent">
				Total XP spent: {inventory.total_spent.toLocaleString()}
			</span>
		</div>
	{/if}
</div>

<style>
	.inventory-panel {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
		height: 100%;
	}

	.loading-state,
	.empty-state {
		text-align: center;
		padding: var(--spacing-xl);
		color: var(--ui-text-tertiary);
	}

	.empty-state .hint {
		font-size: 0.75rem;
		margin-top: var(--spacing-sm);
	}

	.inventory-section {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.section-title {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: 0;
	}

	.consumables-list,
	.tools-list {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
	}

	.tool-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--spacing-sm);
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
	}

	.tool-info {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.tool-icon {
		font-size: 1rem;
	}

	.tool-name {
		font-size: 0.875rem;
		font-weight: 500;
	}

	.uses-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		background: var(--ui-warning-container, #fef3c7);
		color: var(--ui-warning, #d97706);
		text-transform: uppercase;
		font-weight: 600;
	}

	.owned-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		background: var(--ui-success-container, #d1fae5);
		color: var(--ui-success, #059669);
		text-transform: uppercase;
		font-weight: 600;
	}

	.inventory-footer {
		margin-top: auto;
		padding-top: var(--spacing-md);
		border-top: 1px solid var(--ui-border-subtle);
	}

	.total-spent {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}
</style>
