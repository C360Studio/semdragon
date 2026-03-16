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
	import GraphFilters from '$lib/components/graph/GraphFilters.svelte';

	import { graphStore } from '$lib/stores/graphStore.svelte';
	import { graphApi } from '$lib/services/graphApi';
	import { transformPathSearchResult, transformGlobalSearchResult } from '$lib/services/graphTransform';
	import type { GraphStoreAdapter } from '$lib/stores/graphStore.svelte';

	// ---------------------------------------------------------------------------
	// Panel state — matches the pattern used in trajectories/quests pages
	// ---------------------------------------------------------------------------
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// ---------------------------------------------------------------------------
	// GraphStoreAdapter adapter
	//
	// graphStore expects listEntities / getEntityNeighbors / searchEntities.
	// graphApi exposes getEntitiesByPrefix / pathSearch / globalSearch.
	// This adapter bridges the two without modifying either file.
	// ---------------------------------------------------------------------------
	const graphApiAdapter: GraphStoreAdapter = {
		async listEntities({ prefix = '', limit = 200 }) {
			const backendEntities = await graphApi.getEntitiesByPrefix(prefix, limit);
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
	// Load initial graph on first mount only.
	// If the user navigates away and back, the existing store data is preserved
	// rather than silently resetting their exploration. The refresh button
	// provides an explicit way to reload.
	// ---------------------------------------------------------------------------
	$effect(() => {
		if (graphStore.entities.size > 0) return;
		graphStore.loadInitialGraph(graphApiAdapter);
	});

	// ---------------------------------------------------------------------------
	// Derived passthrough values from the store
	// ---------------------------------------------------------------------------
	const filteredEntities = $derived(graphStore.filteredEntities);
	const filteredRelationships = $derived(graphStore.filteredRelationships);
	const selectedEntity = $derived(graphStore.selectedEntity);

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
				<GraphFilters
					visibleTypes={graphStore.visibleTypes}
					presentTypes={graphStore.presentEntityTypes}
					search={graphStore.filters.search}
					onToggleType={handleToggleType}
					onSearchChange={handleSearchChange}
					onShowAll={() => graphStore.showAllTypes()}
					onHideAll={() => graphStore.hideAllTypes()}
				/>
				<div class="toolbar-spacer"></div>
				<GraphMetrics
					entities={filteredEntities}
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
		<GraphDetail
			entity={selectedEntity ?? null}
			onEntitySelect={handleRelatedEntitySelect}
		/>
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
