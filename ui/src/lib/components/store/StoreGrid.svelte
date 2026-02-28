<script lang="ts">
	/**
	 * StoreGrid - Grid display of store items
	 */

	import type { StoreItem } from '$types';
	import StoreItemCard from './StoreItemCard.svelte';

	interface StoreGridProps {
		/** Items to display */
		items: StoreItem[];
		/** Currently selected item ID */
		selectedId?: string | null;
		/** Agent's current XP (for affordability check) */
		agentXP?: number;
		/** Handler for item selection */
		onSelect?: (item: StoreItem) => void;
	}

	let { items, selectedId = null, agentXP = 0, onSelect }: StoreGridProps = $props();
</script>

<div class="store-grid" data-testid="store-grid">
	{#if items.length === 0}
		<div class="empty-state">
			<p>No items available</p>
		</div>
	{:else}
		{#each items as item (item.id)}
			<StoreItemCard
				{item}
				selected={selectedId === item.id}
				canAfford={agentXP >= item.xp_cost}
				onclick={() => onSelect?.(item)}
			/>
		{/each}
	{/if}
</div>

<style>
	.store-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
		gap: var(--spacing-md);
		padding: var(--spacing-md);
	}

	.empty-state {
		grid-column: 1 / -1;
		text-align: center;
		padding: var(--spacing-xl);
		color: var(--ui-text-tertiary);
	}
</style>
