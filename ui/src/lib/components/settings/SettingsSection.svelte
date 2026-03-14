<script lang="ts">
	/**
	 * SettingsSection - Collapsible card wrapper for settings groups.
	 */
	import type { Snippet } from 'svelte';

	let {
		title,
		open = true,
		badge = '',
		badgeVariant = 'neutral',
		onfocus,
		children
	}: {
		title: string;
		open?: boolean;
		badge?: string;
		badgeVariant?: 'neutral' | 'success' | 'warning' | 'error';
		onfocus?: () => void;
		children: Snippet;
	} = $props();

	let toggleCount = $state(0);
	let expanded = $derived((toggleCount % 2 === 0) ? open : !open);
</script>

<section class="settings-section" data-testid="settings-section-{title.toLowerCase().replace(/\s+/g, '-')}">
	<button
		class="section-toggle"
		onclick={() => (toggleCount++)}
		onfocus={onfocus}
		onmouseenter={onfocus}
		aria-expanded={expanded}
		aria-label="{expanded ? 'Collapse' : 'Expand'} {title}"
	>
		<span class="toggle-icon" aria-hidden="true">{expanded ? '\u25BC' : '\u25B6'}</span>
		<span class="section-title">{title}</span>
		{#if badge}
			<span class="section-badge" data-variant={badgeVariant}>{badge}</span>
		{/if}
	</button>
	{#if expanded}
		<div class="section-body">
			{@render children()}
		</div>
	{/if}
</section>

<style>
	.settings-section {
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		background: var(--ui-surface-secondary);
		overflow: hidden;
	}

	.section-toggle {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		width: 100%;
		padding: var(--spacing-md) var(--spacing-lg);
		background: none;
		border: none;
		cursor: pointer;
		color: var(--ui-text-primary);
		text-align: left;
	}

	.section-toggle:hover {
		background: var(--ui-surface-tertiary);
	}

	.toggle-icon {
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
		width: 12px;
	}

	.section-title {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		flex: 1;
	}

	.section-badge {
		font-size: 0.6875rem;
		font-weight: 500;
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.section-badge[data-variant='neutral'] {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
	}

	.section-badge[data-variant='success'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.section-badge[data-variant='warning'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.section-badge[data-variant='error'] {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.section-body {
		padding: 0 var(--spacing-lg) var(--spacing-lg);
	}
</style>
