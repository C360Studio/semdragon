<script lang="ts">
	/**
	 * ItemDetail - Full item information display
	 */

	import type { StoreItem, TrustTier } from '$types';
	import { TrustTierNames, ConsumableTypeDescriptions, type ConsumableType } from '$types';
	import PurchaseButton from './PurchaseButton.svelte';

	interface ItemDetailProps {
		/** The item to display */
		item: StoreItem;
		/** Agent's current XP */
		agentXP: number;
		/** Number already owned (for consumables) */
		owned?: number;
		/** Whether a purchase is in progress */
		purchasing?: boolean;
		/** Handler for purchase action */
		onPurchase?: () => void;
	}

	let {
		item,
		agentXP,
		owned = 0,
		purchasing = false,
		onPurchase
	}: ItemDetailProps = $props();

	// Derived values
	const formattedCost = $derived(item.xp_cost.toLocaleString());
	const tierDisplay = $derived(TrustTierNames[item.min_tier as TrustTier] ?? 'Any');
	const canAfford = $derived(agentXP >= item.xp_cost);
	const typeIcon = $derived(item.item_type === 'tool' ? '🔧' : '✨');

	// Get consumable description if applicable
	const consumableDescription = $derived.by(() => {
		if (item.item_type !== 'consumable' || !item.effect?.type) return null;
		return ConsumableTypeDescriptions[item.effect.type as ConsumableType] ?? null;
	});
</script>

<div class="item-detail" data-testid="item-detail">
	<div class="item-header">
		<span class="item-icon" aria-hidden="true">{typeIcon}</span>
		<h2 class="item-name">{item.name}</h2>
	</div>

	<div class="item-cost-display">
		<span class="cost-amount">{formattedCost}</span>
		<span class="cost-label">XP</span>
	</div>

	<p class="item-description">{item.description}</p>

	{#if consumableDescription}
		<p class="consumable-effect">{consumableDescription}</p>
	{/if}

	<dl class="item-details">
		<dt>Type</dt>
		<dd>{item.item_type === 'tool' ? 'Tool' : 'Consumable'}</dd>

		<dt>Tier Required</dt>
		<dd>
			<span class="tier-badge" data-tier={item.min_tier}>{tierDisplay}</span>
		</dd>

		{#if item.purchase_type === 'rental' && item.rental_uses}
			<dt>Uses</dt>
			<dd>{item.rental_uses} uses per purchase</dd>
		{/if}

		{#if item.min_level}
			<dt>Min Level</dt>
			<dd>Level {item.min_level}</dd>
		{/if}

		{#if item.guild_discount && item.guild_discount > 0}
			<dt>Guild Discount</dt>
			<dd>{Math.round(item.guild_discount * 100)}% off for guild members</dd>
		{/if}

		{#if owned > 0}
			<dt>You own</dt>
			<dd>{owned}</dd>
		{/if}
	</dl>

	<div class="purchase-section">
		<PurchaseButton
			cost={item.xp_cost}
			{canAfford}
			inStock={item.in_stock}
			{purchasing}
			{onPurchase}
		/>

		{#if !canAfford && item.in_stock}
			<p class="shortfall-message">
				Need {(item.xp_cost - agentXP).toLocaleString()} more XP
			</p>
		{/if}
	</div>
</div>

<style>
	.item-detail {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
		padding: var(--spacing-md);
	}

	.item-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.item-icon {
		font-size: 1.5rem;
	}

	.item-name {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
	}

	.item-cost-display {
		display: flex;
		align-items: baseline;
		gap: var(--spacing-xs);
	}

	.cost-amount {
		font-size: 2rem;
		font-weight: 700;
		color: var(--xp-color, #fbbf24);
	}

	.cost-label {
		font-size: 1rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
	}

	.item-description {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		margin: 0;
	}

	.consumable-effect {
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		background: var(--ui-surface-tertiary);
		padding: var(--spacing-sm);
		border-radius: var(--radius-sm);
		margin: 0;
	}

	.item-details {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
		margin: 0;
	}

	.item-details dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.item-details dd {
		margin: 0;
		font-size: 0.875rem;
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

	.purchase-section {
		margin-top: var(--spacing-md);
		padding-top: var(--spacing-md);
		border-top: 1px solid var(--ui-border-subtle);
	}

	.shortfall-message {
		margin: var(--spacing-sm) 0 0;
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		text-align: center;
	}
</style>
