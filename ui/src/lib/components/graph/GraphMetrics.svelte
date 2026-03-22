<script lang="ts">
	/**
	 * GraphMetrics - Compact stats bar for the graph visualization
	 *
	 * Displays entity counts broken down by game entity type and total
	 * relationship count. Shown at the top of the center panel so users
	 * can see what the graph contains at a glance without inspecting nodes.
	 */

	import type { GraphRelationship } from '$lib/api/graph-types';

	interface GraphMetricsProps {
		entityCount: number;
		relationships: GraphRelationship[];
	}

	let { entityCount, relationships }: GraphMetricsProps = $props();

	/**
	 * Graph density = edges / (n * (n-1)) for a directed graph.
	 * Only meaningful above ~3 nodes; we suppress it below that threshold.
	 */
	const density = $derived.by(() => {
		if (entityCount < 3) return 0;
		return relationships.length / (entityCount * (entityCount - 1));
	});

	const densityLabel = $derived(
		density > 0 ? (density * 100).toFixed(1) + '%' : null
	);
</script>

<div class="graph-metrics" role="status" aria-label="Graph statistics" data-testid="graph-metrics">
	<span class="metric-item" data-testid="metrics-entities">
		<span class="metric-value">{entityCount}</span>
		<span class="metric-label">nodes</span>
	</span>

	<span class="metrics-sep" aria-hidden="true">|</span>

	<span class="metric-item" data-testid="metrics-relationships">
		<span class="metric-value">{relationships.length}</span>
		<span class="metric-label">edges</span>
	</span>

	{#if densityLabel}
		<span class="metrics-sep" aria-hidden="true">|</span>
		<span class="metric-item" data-testid="metrics-density">
			<span class="metric-value">{densityLabel}</span>
			<span class="metric-label">density</span>
		</span>
	{/if}
</div>

<style>
	.graph-metrics {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 6px 12px;
		font-size: 11px;
		white-space: nowrap;
		flex-shrink: 0;
	}

	.metrics-sep {
		color: var(--ui-border-subtle);
		margin: 0 2px;
	}

	.metric-item {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		color: var(--ui-text-secondary);
	}

	.metric-value {
		font-weight: 600;
		color: var(--ui-text-primary);
	}
</style>
