<script lang="ts">
	/**
	 * GraphSummary - Default right-panel view when no graph entity is selected.
	 *
	 * Shows what agents see when they call the graph_summary tool:
	 * - Section A: raw formatted text from the backend (agent view)
	 * - Section B: structured source cards with domain breakdowns
	 *
	 * Fetches once on mount and caches the result; re-renders do not re-fetch.
	 */

	import { getGraphSummary } from '$lib/services/api';
	import type { GraphSummaryResponse, GraphSummarySource } from '$lib/services/api';

	// ---------------------------------------------------------------------------
	// State
	// ---------------------------------------------------------------------------
	let data = $state<GraphSummaryResponse | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// ---------------------------------------------------------------------------
	// Fetch on mount — cache in component-local state, no re-fetch on re-render
	// ---------------------------------------------------------------------------
	$effect(() => {
		// Only fetch once; data stays populated across re-renders
		if (data !== null) return;

		loading = true;
		error = null;

		getGraphSummary()
			.then((result) => {
				data = result;
			})
			.catch((err: unknown) => {
				error = err instanceof Error ? err.message : 'Failed to load graph summary';
			})
			.finally(() => {
				loading = false;
			});
	});

	// ---------------------------------------------------------------------------
	// Helpers
	// ---------------------------------------------------------------------------
	function sourceTypeBadgeClass(type: string): string {
		return type === 'semsource' ? 'badge-semsource' : 'badge-local';
	}
</script>

<div class="detail-panel" data-testid="graph-summary-panel">
	<!-- Panel header -->
	<div class="panel-header">
		<div class="header-icon" aria-hidden="true">G</div>
		<div class="header-title">
			<h3 class="title-text">Graph Summary</h3>
			<span class="title-sub">Agent knowledge view</span>
		</div>
	</div>

	{#if loading}
		<div class="loading-state" aria-live="polite" aria-label="Loading graph summary">
			<span class="loading-dot"></span>
			<span class="loading-dot"></span>
			<span class="loading-dot"></span>
			<span class="loading-label">Loading summary...</span>
		</div>
	{:else if error}
		<div class="section" role="alert">
			<h4 class="section-title">Error</h4>
			<p class="error-message">{error}</p>
			<button
				class="retry-button"
				onclick={() => {
					loading = true;
					error = null;
					data = null;
				}}
			>
				Retry
			</button>
		</div>
	{:else if data}
		<!-- Section A: Agent View — raw text agents receive from graph_summary -->
		<section class="section" aria-label="Agent view of graph summary">
			<h4 class="section-title">graph_summary output</h4>
			<pre class="agent-text" tabindex="0" role="region" aria-label="graph_summary tool output">{data.text}</pre>
		</section>

		<!-- Section B: Sources — structured breakdown per knowledge source -->
		{#if data.sources && data.sources.length > 0}
			<section class="section" aria-label="Knowledge sources">
				<h4 class="section-title">Sources ({data.sources.length})</h4>
				<div class="sources-list">
					{#each data.sources as source (source.name)}
						<div class="source-card" data-testid="source-card-{source.name}">
							<!-- Source header row -->
							<div class="source-header">
								<span class="source-name">{source.name}</span>
								<span class="source-badge {sourceTypeBadgeClass(source.type)}">{source.type}</span>
								<span
									class="source-ready"
									class:ready={source.ready}
									aria-label={source.ready ? 'Ready' : 'Not ready'}
								>
									{source.ready ? '●' : '○'}
								</span>
							</div>

							<!-- Entity prefix -->
							<div class="source-prefix" title="Entity prefix">
								<span class="prefix-label">prefix</span>
								<code class="prefix-value">{source.entity_prefix}</code>
							</div>

							<!-- Total entity count -->
							<div class="source-count">
								<span class="count-label">entities</span>
								<span class="count-value">{source.total_entities.toLocaleString()}</span>
							</div>

							<!-- Domain breakdown -->
							{#if source.domains && source.domains.length > 0}
								<div class="domains-list">
									{#each source.domains as domain (domain.domain)}
										<div class="domain-row">
											<span class="domain-name">{domain.domain}</span>
											<span class="domain-count">{domain.entity_count}</span>
											<div class="type-chips">
												{#each domain.types as t (t.type)}
													<span class="type-chip" title="{t.type}: {t.count}">
														{t.type}
														<span class="chip-count">{t.count}</span>
													</span>
												{/each}
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			</section>
		{/if}
	{:else}
		<div class="section">
			<p class="empty-message">No summary data available.</p>
		</div>
	{/if}
</div>

<style>
	/* Panel shell — mirrors GraphDetail.detail-panel */
	.detail-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow-y: auto;
		background: var(--ui-surface-secondary);
	}

	/* Header — mirrors GraphDetail.panel-header */
	.panel-header {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 12px;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-primary);
		flex-shrink: 0;
	}

	.header-icon {
		width: 36px;
		height: 36px;
		border-radius: 50%;
		background: var(--ui-border-strong, #444);
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--ui-text-secondary);
		font-weight: 600;
		font-size: 16px;
		flex-shrink: 0;
	}

	.header-title {
		flex: 1;
		min-width: 0;
	}

	.title-text {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.title-sub {
		font-size: 11px;
		color: var(--ui-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	/* Sections — mirrors GraphDetail.section */
	.section {
		padding: 12px;
		border-bottom: 1px solid var(--ui-border-subtle);
		flex-shrink: 0;
	}

	.section-title {
		margin: 0 0 8px 0;
		font-size: 11px;
		font-weight: 600;
		color: var(--ui-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	/* Agent text block */
	.agent-text {
		margin: 0;
		padding: 10px 12px;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		font-family: var(--font-mono, monospace);
		font-size: 11px;
		line-height: 1.6;
		color: var(--ui-text-primary);
		white-space: pre-wrap;
		word-break: break-word;
		overflow-x: auto;
		max-height: 320px;
		overflow-y: auto;
		outline: none;
	}

	.agent-text:focus {
		border-color: var(--ui-border-strong);
	}

	/* Sources */
	.sources-list {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.source-card {
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		padding: 8px 10px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.source-header {
		display: flex;
		align-items: center;
		gap: 6px;
	}

	.source-name {
		font-size: 12px;
		font-weight: 600;
		color: var(--ui-text-primary);
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.source-badge {
		font-size: 9px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		padding: 1px 5px;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.badge-semsource {
		background: rgba(99, 102, 241, 0.2);
		color: #a5b4fc;
	}

	.badge-local {
		background: rgba(34, 197, 94, 0.15);
		color: #86efac;
	}

	.source-ready {
		font-size: 11px;
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.source-ready.ready {
		color: #4ade80;
	}

	.source-prefix {
		display: flex;
		align-items: baseline;
		gap: 6px;
	}

	.prefix-label {
		font-size: 9px;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		flex-shrink: 0;
	}

	.prefix-value {
		font-family: var(--font-mono, monospace);
		font-size: 11px;
		color: var(--ui-text-secondary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.source-count {
		display: flex;
		align-items: baseline;
		gap: 6px;
	}

	.count-label {
		font-size: 9px;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		flex-shrink: 0;
	}

	.count-value {
		font-size: 12px;
		font-weight: 600;
		color: var(--ui-text-primary);
		font-family: var(--font-mono, monospace);
	}

	/* Domain breakdown */
	.domains-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
		border-top: 1px solid var(--ui-border-subtle);
		padding-top: 6px;
		margin-top: 2px;
	}

	.domain-row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 4px;
		font-size: 11px;
	}

	.domain-name {
		color: var(--ui-text-secondary);
		font-weight: 500;
		min-width: 60px;
		flex-shrink: 0;
	}

	.domain-count {
		font-family: var(--font-mono, monospace);
		font-size: 10px;
		color: var(--ui-text-tertiary);
		min-width: 28px;
		text-align: right;
		flex-shrink: 0;
	}

	.type-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 3px;
		flex: 1;
	}

	.type-chip {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		padding: 1px 5px;
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 3px;
		font-size: 10px;
		color: var(--ui-text-secondary);
		white-space: nowrap;
	}

	.chip-count {
		font-family: var(--font-mono, monospace);
		color: var(--ui-text-tertiary);
		font-size: 9px;
	}

	/* Loading state */
	.loading-state {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 20px 12px;
		color: var(--ui-text-tertiary);
		font-size: 12px;
	}

	.loading-dot {
		width: 5px;
		height: 5px;
		border-radius: 50%;
		background: var(--ui-text-tertiary);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-dot:nth-child(2) {
		animation-delay: 0.2s;
	}

	.loading-dot:nth-child(3) {
		animation-delay: 0.4s;
	}

	.loading-label {
		margin-left: 4px;
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	/* Error state */
	.error-message {
		font-size: 12px;
		color: var(--status-error, #f87171);
		margin: 0 0 8px 0;
	}

	.retry-button {
		padding: 4px 10px;
		background: transparent;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		font-size: 11px;
		color: var(--ui-text-secondary);
		cursor: pointer;
	}

	.retry-button:hover {
		border-color: var(--ui-border-strong);
		color: var(--ui-text-primary);
	}

	/* Empty fallback */
	.empty-message {
		font-size: 12px;
		color: var(--ui-text-tertiary);
		margin: 0;
		font-style: italic;
	}
</style>
