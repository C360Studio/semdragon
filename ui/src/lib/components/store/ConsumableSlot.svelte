<script lang="ts">
	/**
	 * ConsumableSlot - Display a consumable with use button
	 */

	import { ConsumableTypeNames, type ConsumableType } from '$types';

	interface ConsumableSlotProps {
		/** Consumable type ID */
		consumableId: string;
		/** Quantity owned */
		quantity: number;
		/** Whether the use action is in progress */
		using?: boolean;
		/** Handler for use action */
		onUse?: () => void;
	}

	let { consumableId, quantity, using = false, onUse }: ConsumableSlotProps = $props();

	// Get display name
	const displayName = $derived(
		ConsumableTypeNames[consumableId as ConsumableType] ?? consumableId
	);

	// Get icon based on type
	const icon = $derived.by(() => {
		switch (consumableId) {
			case 'retry_token':
				return '🔄';
			case 'cooldown_skip':
				return '⏭️';
			case 'xp_boost':
				return '✨';
			case 'quality_shield':
				return '🛡️';
			case 'insight_scroll':
				return '📜';
			default:
				return '✨';
		}
	});
</script>

<div class="consumable-slot" data-testid="consumable-slot">
	<div class="slot-content">
		<span class="slot-icon" aria-hidden="true">{icon}</span>
		<div class="slot-info">
			<span class="slot-name">{displayName}</span>
			<span class="slot-quantity">x{quantity}</span>
		</div>
	</div>

	<button
		class="use-btn"
		onclick={() => onUse?.()}
		disabled={quantity === 0 || using}
		aria-label="Use {displayName}"
	>
		{#if using}
			<span class="spinner" aria-hidden="true"></span>
		{:else}
			Use
		{/if}
	</button>
</div>

<style>
	.consumable-slot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--spacing-sm);
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
	}

	.slot-content {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.slot-icon {
		font-size: 1.25rem;
	}

	.slot-info {
		display: flex;
		flex-direction: column;
	}

	.slot-name {
		font-size: 0.875rem;
		font-weight: 500;
	}

	.slot-quantity {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.use-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--spacing-xs) var(--spacing-sm);
		min-width: 48px;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--ui-text-on-primary);
		background: var(--ui-interactive-primary);
		border: none;
		border-radius: var(--radius-sm);
		cursor: pointer;
		transition: background-color 150ms ease, opacity 150ms ease;
	}

	.use-btn:hover:not(:disabled) {
		background: var(--ui-interactive-primary-hover);
	}

	.use-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.spinner {
		width: 12px;
		height: 12px;
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
