<script lang="ts">
	/**
	 * TabBar — reusable horizontal tab navigation.
	 *
	 * Uses the same filter-chip visual language as the rest of the dashboard
	 * so it blends naturally with existing status filter patterns.
	 */

	export interface Tab {
		id: string;
		label: string;
		count?: number;
	}

	let {
		tabs,
		activeTab = $bindable('')
	}: {
		tabs: Tab[];
		activeTab?: string;
	} = $props();
</script>

<div class="tab-bar" role="tablist">
	{#each tabs as tab}
		<button
			type="button"
			role="tab"
			class="tab-chip"
			class:active={activeTab === tab.id}
			aria-selected={activeTab === tab.id}
			aria-controls="tab-panel-{tab.id}"
			onclick={() => (activeTab = tab.id)}
		>
			{tab.label}
			{#if tab.count !== undefined}
				<span class="tab-count">{tab.count}</span>
			{/if}
		</button>
	{/each}
</div>

<style>
	.tab-bar {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		padding: var(--spacing-md) 0;
		border-bottom: 1px solid var(--ui-border-subtle);
		margin-bottom: var(--spacing-md);
	}

	.tab-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 3px 10px;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-full);
		background: var(--ui-surface-primary);
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
		cursor: pointer;
		transition: all 150ms ease;
	}

	.tab-chip:hover {
		border-color: var(--ui-border-interactive);
		color: var(--ui-text-secondary);
	}

	.tab-chip.active {
		background: var(--ui-surface-tertiary);
		border-color: var(--ui-border-interactive);
		color: var(--ui-text-primary);
		font-weight: 500;
	}

	.tab-count {
		font-size: 0.625rem;
		padding: 0 5px;
		border-radius: var(--radius-full);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-tertiary);
		min-width: 16px;
		text-align: center;
	}

	.tab-chip.active .tab-count {
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
	}
</style>
