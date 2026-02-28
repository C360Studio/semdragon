<script lang="ts">
	/**
	 * XPBalance - Displays agent's spendable XP
	 */

	interface XPBalanceProps {
		/** Current XP amount */
		xp: number;
		/** Whether to show large format */
		large?: boolean;
	}

	let { xp, large = false }: XPBalanceProps = $props();

	// Format large numbers with commas
	const formattedXP = $derived(xp.toLocaleString());
</script>

<div class="xp-balance" class:large data-testid="xp-balance">
	<span class="xp-icon" aria-hidden="true">⚡</span>
	<span class="xp-amount">{formattedXP}</span>
	<span class="xp-label">XP</span>
</div>

<style>
	.xp-balance {
		display: inline-flex;
		align-items: center;
		gap: var(--spacing-xs);
		padding: var(--spacing-xs) var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		font-size: 0.875rem;
	}

	.xp-balance.large {
		padding: var(--spacing-sm) var(--spacing-md);
		font-size: 1.25rem;
	}

	.xp-icon {
		color: var(--xp-color, #fbbf24);
	}

	.xp-amount {
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.xp-label {
		color: var(--ui-text-tertiary);
		font-size: 0.75em;
		text-transform: uppercase;
	}

	.large .xp-label {
		font-size: 0.625em;
	}
</style>
