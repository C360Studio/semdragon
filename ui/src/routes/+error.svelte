<script lang="ts">
	/**
	 * Global error page — catches unmatched routes (404) and unexpected errors.
	 *
	 * SvelteKit renders this inside the root +layout.svelte, so the app shell
	 * (nav, chat panel, status bar) remains visible.
	 */

	import { page } from '$app/state';
</script>

<div class="error-page" data-testid="error-page">
	<div class="error-code">{page.status}</div>
	<h2 class="error-title">
		{#if page.status === 404}
			Page not found
		{:else}
			Something went wrong
		{/if}
	</h2>
	<p class="error-message">
		{#if page.status === 404}
			The path <code>{page.url.pathname}</code> doesn't exist.
		{:else if page.error?.message}
			{page.error.message}
		{:else}
			An unexpected error occurred.
		{/if}
	</p>
	<a href="/" class="error-link">Back to Dashboard</a>
</div>

<style>
	.error-page {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		padding: var(--spacing-xl);
		min-height: 60vh;
	}

	.error-code {
		font-size: 4rem;
		font-weight: 700;
		color: var(--ui-text-tertiary);
		line-height: 1;
		margin-bottom: var(--spacing-sm);
	}

	.error-title {
		font-size: 1.25rem;
		margin-bottom: var(--spacing-md);
	}

	.error-message {
		color: var(--ui-text-secondary);
		margin-bottom: var(--spacing-lg);
		max-width: 400px;
	}

	.error-message code {
		font-size: 0.875rem;
		padding: 2px 6px;
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
	}

	.error-link {
		padding: var(--spacing-xs) var(--spacing-md);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		color: var(--ui-interactive-primary);
		text-decoration: none;
		font-size: 0.875rem;
		transition: all 150ms ease;
	}

	.error-link:hover {
		background: var(--ui-surface-secondary);
		border-color: var(--ui-interactive-primary);
	}
</style>
