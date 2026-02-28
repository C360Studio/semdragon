<script lang="ts">
	/**
	 * PurchaseButton - Buy button with confirmation and loading state
	 */

	interface PurchaseButtonProps {
		/** XP cost of the item */
		cost: number;
		/** Whether the agent can afford this item */
		canAfford: boolean;
		/** Whether the item is in stock */
		inStock: boolean;
		/** Whether a purchase is in progress */
		purchasing?: boolean;
		/** Handler for purchase action */
		onPurchase?: () => void;
	}

	let {
		cost,
		canAfford,
		inStock,
		purchasing = false,
		onPurchase
	}: PurchaseButtonProps = $props();

	let confirming = $state(false);

	// Format cost
	const formattedCost = $derived(cost.toLocaleString());

	// Button state
	const disabled = $derived(!canAfford || !inStock || purchasing);

	// Auto-cancel confirmation after 3 seconds with proper cleanup
	$effect(() => {
		if (confirming) {
			const timeoutId = setTimeout(() => {
				confirming = false;
			}, 3000);

			return () => clearTimeout(timeoutId);
		}
	});

	function handleClick() {
		if (confirming) {
			confirming = false;
			onPurchase?.();
		} else {
			confirming = true;
		}
	}

	function handleCancel() {
		confirming = false;
	}
</script>

<div class="purchase-button-container" data-testid="purchase-button">
	{#if confirming}
		<div class="confirm-row" role="status" aria-live="polite">
			<button
				class="confirm-btn"
				onclick={handleClick}
				disabled={purchasing}
				aria-label="Confirm purchase for {formattedCost} XP"
			>
				{#if purchasing}
					<span class="spinner" aria-hidden="true"></span>
					Purchasing...
				{:else}
					Confirm Purchase
				{/if}
			</button>
			<button class="cancel-btn" onclick={handleCancel} disabled={purchasing} type="button">
				Cancel
			</button>
		</div>
	{:else}
		<button class="purchase-btn" onclick={handleClick} {disabled}>
			{#if !inStock}
				Out of Stock
			{:else if !canAfford}
				Need More XP
			{:else}
				<span class="btn-icon">⚡</span>
				Buy for {formattedCost} XP
			{/if}
		</button>
	{/if}
</div>

<style>
	.purchase-button-container {
		width: 100%;
	}

	.purchase-btn {
		width: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--spacing-xs);
		padding: var(--spacing-sm) var(--spacing-md);
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--ui-text-on-primary);
		background: var(--ui-interactive-primary);
		border: none;
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: background-color 150ms ease, opacity 150ms ease;
	}

	.purchase-btn:hover:not(:disabled) {
		background: var(--ui-interactive-primary-hover);
	}

	.purchase-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-icon {
		font-size: 1rem;
	}

	.confirm-row {
		display: flex;
		gap: var(--spacing-sm);
	}

	.confirm-btn {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--spacing-xs);
		padding: var(--spacing-sm) var(--spacing-md);
		font-size: 0.875rem;
		font-weight: 600;
		color: white;
		background: var(--ui-success, #10b981);
		border: none;
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: background-color 150ms ease;
	}

	.confirm-btn:hover:not(:disabled) {
		background: var(--ui-success-hover, #059669);
	}

	.confirm-btn:disabled {
		opacity: 0.7;
		cursor: wait;
	}

	.cancel-btn {
		padding: var(--spacing-sm) var(--spacing-md);
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		background: var(--ui-surface-tertiary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: background-color 150ms ease;
	}

	.cancel-btn:hover:not(:disabled) {
		background: var(--ui-surface-secondary);
	}

	.cancel-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.spinner {
		width: 14px;
		height: 14px;
		border: 2px solid rgba(255, 255, 255, 0.3);
		border-top-color: white;
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
