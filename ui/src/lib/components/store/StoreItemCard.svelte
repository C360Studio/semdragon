<script lang="ts">
	/**
	 * StoreItemCard - Display a store item in a card format
	 */

	import type { StoreItem, TrustTier } from '$types';
	import { TrustTierNames } from '$types';

	interface StoreItemCardProps {
		/** The store item to display */
		item: StoreItem;
		/** Whether this card is selected */
		selected?: boolean;
		/** Whether the agent can afford this item */
		canAfford?: boolean;
		/** Click handler */
		onclick?: () => void;
	}

	let { item, selected = false, canAfford = true, onclick }: StoreItemCardProps = $props();

	// Format XP cost with commas
	const formattedCost = $derived(item.xp_cost.toLocaleString());

	// Get tier display
	const tierDisplay = $derived(TrustTierNames[item.min_tier as TrustTier] ?? 'Any');

	// Item type icon
	const typeIcon = $derived(item.item_type === 'tool' ? '🔧' : '✨');
</script>

<button
	class="store-item-card"
	class:selected
	class:out-of-stock={!item.in_stock}
	class:cannot-afford={!canAfford}
	aria-label="{item.name}, {formattedCost} XP, requires {tierDisplay} tier"
	aria-pressed={selected}
	disabled={!item.in_stock}
	{onclick}
	data-testid="store-item-card"
>
	<div class="item-header">
		<span class="item-icon" aria-hidden="true">{typeIcon}</span>
		<span class="item-cost">
			<span class="cost-amount">{formattedCost}</span>
			<span class="cost-icon">⚡</span>
		</span>
	</div>

	<h3 class="item-name">{item.name}</h3>

	<div class="item-meta">
		{#if item.purchase_type === 'rental' && item.rental_uses}
			<span class="rental-badge">{item.rental_uses} uses</span>
		{/if}
		<span class="tier-badge" data-tier={item.min_tier}>{tierDisplay}</span>
	</div>

	{#if !item.in_stock}
		<div class="out-of-stock-overlay">
			<span>Out of Stock</span>
		</div>
	{/if}
</button>

<style>
	.store-item-card {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
		padding: var(--spacing-md);
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		text-align: left;
		cursor: pointer;
		transition:
			border-color 150ms ease,
			box-shadow 150ms ease,
			transform 100ms ease;
	}

	.store-item-card:hover:not(:disabled) {
		border-color: var(--ui-border-interactive);
		transform: translateY(-1px);
	}

	.store-item-card.selected {
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 1px var(--ui-interactive-primary);
	}

	.store-item-card.cannot-afford {
		opacity: 0.7;
	}

	.store-item-card:disabled {
		cursor: not-allowed;
	}

	.item-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.item-icon {
		font-size: 1.25rem;
	}

	.item-cost {
		display: flex;
		align-items: center;
		gap: 2px;
		font-weight: 600;
		color: var(--xp-color, #fbbf24);
	}

	.cost-amount {
		font-size: 0.875rem;
	}

	.cost-icon {
		font-size: 0.75rem;
	}

	.item-name {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--ui-text-primary);
	}

	.item-meta {
		display: flex;
		gap: var(--spacing-xs);
		flex-wrap: wrap;
	}

	.rental-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
		text-transform: uppercase;
	}

	.tier-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		text-transform: uppercase;
		font-weight: 600;
	}

	.tier-badge[data-tier='0'] {
		background: var(--tier-apprentice-container, #e0f2fe);
		color: var(--tier-apprentice, #0284c7);
	}
	.tier-badge[data-tier='1'] {
		background: var(--tier-journeyman-container, #d1fae5);
		color: var(--tier-journeyman, #059669);
	}
	.tier-badge[data-tier='2'] {
		background: var(--tier-expert-container, #fef3c7);
		color: var(--tier-expert, #d97706);
	}
	.tier-badge[data-tier='3'] {
		background: var(--tier-master-container, #ede9fe);
		color: var(--tier-master, #7c3aed);
	}
	.tier-badge[data-tier='4'] {
		background: var(--tier-grandmaster-container, #fce7f3);
		color: var(--tier-grandmaster, #db2777);
	}

	.out-of-stock {
		opacity: 0.5;
	}

	.out-of-stock-overlay {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background: rgba(0, 0, 0, 0.5);
		border-radius: var(--radius-md);
		color: var(--ui-text-primary);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
	}
</style>
