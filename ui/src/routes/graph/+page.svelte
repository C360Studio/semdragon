<script lang="ts">
	/**
	 * Graph Explorer - Knowledge graph visualization page
	 *
	 * Three-panel layout:
	 * - Left:   ExplorerNav
	 * - Center: GraphFilters + GraphMetrics + SigmaCanvas (full-height WebGL canvas)
	 * - Right:  GraphDetail for the selected entity
	 *
	 * Data flow:
	 * - On mount, calls graphStore.loadInitialGraph via a graphApiAdapter
	 *   that bridges the graphApi service to the GraphStoreAdapter interface.
	 * - Selection, hover, and expand events update graphStore state directly.
	 * - Filtered entities/relationships from graphStore feed SigmaCanvas.
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import SigmaCanvas from '$lib/components/graph/SigmaCanvas.svelte';
	import GraphMetrics from '$lib/components/graph/GraphMetrics.svelte';
	import GraphDetail from '$lib/components/graph/GraphDetail.svelte';
	import GraphSummary from '$lib/components/graph/GraphSummary.svelte';
	import GraphFilters from '$lib/components/graph/GraphFilters.svelte';

	import { graphStore } from '$lib/stores/graphStore.svelte';
	import { graphApi } from '$lib/services/graphApi';
	import { transformPathSearchResult, transformGlobalSearchResult } from '$lib/services/graphTransform';
	import type { GraphStoreAdapter } from '$lib/stores/graphStore.svelte';
	import { getGraphSummary } from '$lib/services/api';
	import type { GraphSummaryResponse, GraphSummarySource } from '$lib/services/api';

	// ---------------------------------------------------------------------------
	// Panel state — matches the pattern used in trajectories/quests pages
	// ---------------------------------------------------------------------------
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// ---------------------------------------------------------------------------
	// Source selector state
	// empty string = all sources (no prefix filter)
	// ---------------------------------------------------------------------------
	let selectedSource = $state<string>('');
	let availableSources = $state<GraphSummarySource[]>([]);

	// ---------------------------------------------------------------------------
	// Summary data — fetched once here and passed as props to GraphSummary.
	// Hoisted to the page so GraphSummary is purely props-driven and the source
	// list and summary panel share a single fetch with no duplicate requests.
	// ---------------------------------------------------------------------------
	let summaryData = $state<GraphSummaryResponse | null>(null);
	let summaryLoading = $state(true);
	let summaryError = $state<string | null>(null);

	$effect(() => {
		if (summaryData !== null) return;

		getGraphSummary()
			.then((result) => {
				summaryData = result;
				availableSources = result.sources;
			})
			.catch((err) => {
				summaryError = err instanceof Error ? err.message : 'Failed to load graph summary';
			})
			.finally(() => {
				summaryLoading = false;
			});
	});

	// ---------------------------------------------------------------------------
	// GraphStoreAdapter adapter
	//
	// graphStore expects listEntities / getEntityNeighbors / searchEntities.
	// graphApi exposes getEntitiesByPrefix / pathSearch / globalSearch.
	// This adapter bridges the two without modifying either file.
	// ---------------------------------------------------------------------------
	const graphApiAdapter: GraphStoreAdapter = {
		async listEntities({ prefix = '', limit = 200 }) {
			// Use the selected source prefix when set; otherwise fall through to
			// whatever prefix the caller supplied (empty string = all entities).
			// Strip trailing dot — GraphQL prefix matching treats it as a literal.
			const rawPrefix = selectedSource || prefix;
			const queryPrefix = rawPrefix.endsWith('.') ? rawPrefix.slice(0, -1) : rawPrefix;
			const backendEntities = await graphApi.getEntitiesByPrefix(queryPrefix, limit);
			const entities = transformPathSearchResult({ entities: backendEntities, edges: [] });
			return { entities };
		},

		async getEntityNeighbors(entityId: string) {
			const result = await graphApi.pathSearch(entityId, 2, 50);
			const entities = transformPathSearchResult(result);
			return { entities };
		},

		async searchEntities({ query, limit = 50 }) {
			const result = await graphApi.globalSearch(query);
			// Use transformGlobalSearchResult to preserve explicit relationships from NLQ
			const allEntities = transformGlobalSearchResult(result);
			return { entities: allEntities.slice(0, limit) };
		}
	};

	// ---------------------------------------------------------------------------
	// Load (or reload) the graph whenever selectedSource changes.
	// On first run (mount), selectedSource is '' — loads all entities.
	// When the user picks a different source from the dropdown, $effect re-runs
	// automatically because selectedSource is $state-tracked.
	// ---------------------------------------------------------------------------
	$effect(() => {
		// Track selectedSource so the effect re-runs on every dropdown change.
		const _prefix = selectedSource;
		graphStore.clearEntities();
		void graphStore.loadInitialGraph(graphApiAdapter);
	});

	// ---------------------------------------------------------------------------
	// Derived passthrough values from the store
	// ---------------------------------------------------------------------------
	const filteredEntities = $derived(graphStore.filteredEntities);
	const filteredRelationships = $derived(graphStore.filteredRelationships);
	const selectedEntity = $derived(graphStore.selectedEntity);

	/** Entity counts per type — computed from all entities (not filtered) so
	 *  counts stay stable when toggling type visibility. */
	const typeCounts = $derived.by(() => {
		const counts = new Map<string, number>();
		for (const e of graphStore.entities.values()) {
			const t = e.idParts.type || 'unknown';
			counts.set(t, (counts.get(t) ?? 0) + 1);
		}
		return counts;
	});

	// ---------------------------------------------------------------------------
	// Event handlers — delegate straight to graphStore mutations
	// ---------------------------------------------------------------------------
	function handleEntitySelect(entityId: string | null) {
		graphStore.selectEntity(entityId);
		// Auto-open the right panel when something is selected
		if (entityId && !rightPanelOpen) {
			rightPanelOpen = true;
		}
	}

	function handleEntityHover(entityId: string | null) {
		graphStore.setHoveredEntity(entityId);
	}

	async function handleEntityExpand(entityId: string) {
		await graphStore.expandEntity(graphApiAdapter, entityId);
	}

	async function handleRefresh() {
		graphStore.clearEntities();
		await graphStore.loadInitialGraph(graphApiAdapter);
	}

	function handleToggleType(type: string) {
		graphStore.toggleEntityType(type);
	}

	function handleSearchChange(search: string) {
		graphStore.setFilters({ search });
	}

	// Navigate to a related entity from the detail panel — select it and
	// expand it so its neighbors are visible in the graph.
	function handleRelatedEntitySelect(entityId: string) {
		graphStore.selectEntity(entityId);
		void graphStore.expandEntity(graphApiAdapter, entityId);
	}

</script>

<svelte:head>
	<title>Graph Explorer - Semdragons</title>
</svelte:head>

<ThreePanelLayout
	{leftPanelOpen}
	{rightPanelOpen}
	{leftPanelWidth}
	{rightPanelWidth}
	onLeftWidthChange={(w) => (leftPanelWidth = w)}
	onRightWidthChange={(w) => (rightPanelWidth = w)}
	onToggleLeft={() => (leftPanelOpen = !leftPanelOpen)}
	onToggleRight={() => (rightPanelOpen = !rightPanelOpen)}
>
	{#snippet leftPanel()}
		<ExplorerNav />
	{/snippet}

	{#snippet centerPanel()}
		<div class="graph-page" data-testid="graph-page">
			<!-- Top bar: filters + metrics in a single horizontal row -->
			<div class="graph-toolbar">
				<div class="source-selector">
					<select
						bind:value={selectedSource}
						aria-label="Filter by knowledge source"
					>
						<option value="">All sources</option>
						{#each availableSources as source (source.name)}
							<option value={source.entity_prefix}>
								{source.name}{source.total_entities > 0 ? ` (${source.total_entities})` : ''}
							</option>
						{/each}
					</select>
				</div>
				<GraphFilters
					visibleTypes={graphStore.visibleTypes}
					presentTypes={graphStore.presentEntityTypes}
					{typeCounts}
					search={graphStore.filters.search}
					onToggleType={handleToggleType}
					onSearchChange={handleSearchChange}
					onShowAll={() => graphStore.showAllTypes()}
					onHideAll={() => graphStore.hideAllTypes()}
				/>
				<div class="toolbar-spacer"></div>
				<GraphMetrics
					entityCount={filteredEntities.length}
					relationships={filteredRelationships}
				/>
			</div>
			{#if graphStore.error}
				<div class="error-banner" role="alert" data-testid="graph-error">
					<span class="error-icon" aria-hidden="true">!</span>
					{graphStore.error}
					<button
						class="error-dismiss"
						onclick={() => graphStore.setError(null)}
						aria-label="Dismiss error"
					>
						×
					</button>
				</div>
			{/if}

			<!-- Graph canvas — fills remaining height -->
			<div class="canvas-wrapper">
				<SigmaCanvas
					entities={filteredEntities}
					relationships={filteredRelationships}
					selectedEntityId={graphStore.selectedEntityId}
					hoveredEntityId={graphStore.hoveredEntityId}
					onEntitySelect={handleEntitySelect}
					onEntityHover={handleEntityHover}
					onEntityExpand={handleEntityExpand}
					onRefresh={handleRefresh}
					loading={graphStore.loading}
				/>
			</div>
		</div>
	{/snippet}

	{#snippet rightPanel()}
		{#if selectedEntity}
			<GraphDetail
				entity={selectedEntity}
				onEntitySelect={handleRelatedEntitySelect}
			/>
		{:else}
			<GraphSummary
					activeSource={selectedSource}
					data={summaryData}
					loading={summaryLoading}
					error={summaryError}
					onRefresh={() => { summaryData = null; summaryLoading = true; summaryError = null; }}
				/>
		{/if}
	{/snippet}
</ThreePanelLayout>

<style>
	.graph-page {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	/* Toolbar: single horizontal row across the top of center panel */
	.graph-toolbar {
		display: flex;
		align-items: center;
		flex-shrink: 0;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-secondary);
		overflow-x: auto;
	}

	.toolbar-spacer {
		flex: 1;
	}

	/* Source selector dropdown — compact, matching toolbar aesthetic */
	.source-selector {
		display: flex;
		align-items: center;
		flex-shrink: 0;
		padding: 0 4px 0 8px;
		border-right: 1px solid var(--ui-border-subtle);
	}

	.source-selector select {
		font-size: 11px;
		padding: 4px 8px;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		color: var(--ui-text-primary);
		cursor: pointer;
		outline: none;
		max-width: 200px;
	}

	.source-selector select:hover {
		border-color: var(--ui-border-strong);
	}

	.source-selector select:focus-visible {
		outline: 2px solid var(--ui-border-strong);
		outline-offset: 1px;
	}

	/* Canvas fills all remaining height below the toolbar */
	.canvas-wrapper {
		flex: 1;
		min-height: 0;
		position: relative;
	}

	/* Error banner */
	.error-banner {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 6px 12px;
		background: var(--status-error-container, #3d1515);
		color: var(--status-error, #f87171);
		font-size: 12px;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.error-icon {
		font-weight: 700;
		flex-shrink: 0;
	}

	.error-dismiss {
		margin-left: auto;
		background: transparent;
		border: none;
		color: inherit;
		font-size: 16px;
		cursor: pointer;
		padding: 0 4px;
		opacity: 0.7;
		line-height: 1;
	}

	.error-dismiss:hover {
		opacity: 1;
	}
</style>
