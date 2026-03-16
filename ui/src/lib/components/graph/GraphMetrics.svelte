<script lang="ts">
	/**
	 * GraphMetrics - Compact stats bar for the graph visualization
	 *
	 * Displays entity counts broken down by game entity type and total
	 * relationship count. Shown at the top of the center panel so users
	 * can see what the graph contains at a glance without inspecting nodes.
	 */

	import type { GraphEntity, GraphRelationship } from '$lib/api/graph-types';
	import { ENTITY_TYPE_COLORS } from '$lib/utils/entity-colors';

	interface GraphMetricsProps {
		entities: GraphEntity[];
		relationships: GraphRelationship[];
	}

	let { entities, relationships }: GraphMetricsProps = $props();

	/** Count entities grouped by entity type (from 5th part of ID). */
	const typeCounts = $derived.by(() => {
		const counts = new Map<string, number>();
		for (const entity of entities) {
			const t = entity.idParts.type || 'unknown';
			counts.set(t, (counts.get(t) ?? 0) + 1);
		}
		return Array.from(counts.entries())
			.sort(([, a], [, b]) => b - a);
	});

	/**
	 * Graph density = edges / (n * (n-1)) for a directed graph.
	 * Only meaningful above ~3 nodes; we suppress it below that threshold.
	 */
	const density = $derived.by(() => {
		const n = entities.length;
		if (n < 3) return 0;
		return relationships.length / (n * (n - 1));
	});

	const densityLabel = $derived(
		density > 0 ? (density * 100).toFixed(1) + '%' : null
	);
</script>

<div class="graph-metrics" role="status" aria-label="Graph statistics" data-testid="graph-metrics">
	<span class="metrics-label">Graph:</span>

	{#each typeCounts as [type, count] (type)}
		<span
			class="type-chip"
			style="--chip-color: {ENTITY_TYPE_COLORS[type] ?? ENTITY_TYPE_COLORS.unknown}"
			title="{count} {type} entities"
			data-testid="metrics-type-{type}"
		>
			<span class="chip-dot" aria-hidden="true"></span>
			<span class="chip-type">{type}</span>
			<span class="chip-count">{count}</span>
		</span>
	{/each}

	<span class="metrics-sep" aria-hidden="true">|</span>

	<span class="metric-item" data-testid="metrics-relationships">
		<span class="metric-value">{relationships.length}</span>
		<span class="metric-label">edges</span>
	</span>

	{#if densityLabel}
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

	.metrics-label {
		font-weight: 600;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
		margin-right: 2px;
	}

	.type-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 2px 7px 2px 5px;
		border-radius: 10px;
		background: color-mix(in srgb, var(--chip-color, #6b7280) 15%, var(--ui-surface-primary));
		border: 1px solid color-mix(in srgb, var(--chip-color, #6b7280) 40%, transparent);
	}

	.type-chip-unknown {
		--chip-color: var(--ui-text-tertiary, #6b7280);
	}

	.chip-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--chip-color, #6b7280);
		flex-shrink: 0;
	}

	.chip-type {
		color: var(--ui-text-secondary);
		text-transform: capitalize;
	}

	.chip-count {
		font-weight: 600;
		color: var(--ui-text-primary);
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
