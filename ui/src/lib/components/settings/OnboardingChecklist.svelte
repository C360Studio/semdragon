<script lang="ts">
	/**
	 * OnboardingChecklist - Guided first-run checklist showing prerequisite status.
	 */
	import type { ChecklistItem } from '$types';

	let { items }: { items: ChecklistItem[] } = $props();

	const metCount = $derived(items.filter((i) => i.met).length);
	const allMet = $derived(metCount === items.length);
</script>

<div class="checklist" data-testid="onboarding-checklist">
	{#if allMet}
		<div class="all-met">All prerequisites met. You're ready to go!</div>
	{/if}
	<ul class="checklist-items">
		{#each items as item}
			<li class="checklist-item" class:met={item.met} aria-label="{item.label}: {item.met ? 'complete' : 'incomplete'}">
				<span class="check-icon" aria-hidden="true">{item.met ? '\u2713' : '\u2717'}</span>
				<div class="check-content">
					<span class="check-label">{item.label}</span>
					{#if !item.met && item.help_text}
						<p class="check-help">{item.help_text}</p>
					{/if}
				</div>
			</li>
		{/each}
	</ul>
</div>

<style>
	.checklist {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.all-met {
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--status-success-container);
		color: var(--status-success);
		border-radius: var(--radius-md);
		font-size: 0.8125rem;
		font-weight: 500;
	}

	.checklist-items {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.checklist-item {
		display: flex;
		align-items: flex-start;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-primary);
		border-radius: var(--radius-md);
		border: 1px solid var(--ui-border-subtle);
	}

	.check-icon {
		width: 20px;
		height: 20px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.75rem;
		font-weight: 700;
		flex-shrink: 0;
		border-radius: var(--radius-full);
	}

	.checklist-item.met .check-icon {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.checklist-item:not(.met) .check-icon {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.check-content {
		flex: 1;
		min-width: 0;
	}

	.check-label {
		font-size: 0.8125rem;
		font-weight: 500;
	}

	.check-help {
		margin: var(--spacing-xs) 0 0;
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		line-height: 1.4;
	}
</style>
