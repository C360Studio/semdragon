<script lang="ts">
	/**
	 * GraphDetail - Entity detail panel for the graph visualization
	 *
	 * Shows a complete breakdown of the selected game entity:
	 * - Color-coded type badge with instance label
	 * - 6-part entity ID decomposed into labeled fields
	 * - Properties table with confidence opacity
	 * - Outgoing and incoming relationships as clickable navigation links
	 * - Last-updated timestamp derived from the most recent property
	 */

	import type { GraphEntity } from '$lib/api/graph-types';
	import { getEntityLabel, getEntityTypeLabel, parseEntityId } from '$lib/api/graph-types';
	import { getEntityColor, getPredicateColor, getConfidenceOpacity } from '$lib/utils/entity-colors';
	import CopyButton from '$components/CopyButton.svelte';

	interface GraphDetailProps {
		entity: GraphEntity | null;
		onEntitySelect: (id: string) => void;
	}

	let { entity, onEntitySelect }: GraphDetailProps = $props();

	// Derived display values
	const label = $derived(entity ? getEntityLabel(entity) : '');
	const typeLabel = $derived(entity ? getEntityTypeLabel(entity) : '');
	const entityColor = $derived(entity ? getEntityColor(entity.idParts) : '#888');

	function formatTimestamp(ts: number): string {
		return new Date(ts).toLocaleString();
	}

	function formatConfidence(confidence: number): string {
		return `${(confidence * 100).toFixed(0)}%`;
	}

	/** Show only the last segment of a dotted predicate name. */
	function shortPredicate(predicate: string): string {
		const parts = predicate.split('.');
		return parts[parts.length - 1] || predicate;
	}

	/** Navigate to a related entity when the user clicks a relationship row. */
	function handleRelatedEntityClick(entityId: string) {
		onEntitySelect(entityId);
	}
</script>

{#if entity}
	<div class="detail-panel" data-testid="graph-detail-panel">
		<!-- Header -->
		<div class="panel-header">
			<div class="entity-badge" style="background-color: {entityColor}" aria-hidden="true">
				{entity.idParts.type.charAt(0).toUpperCase()}
			</div>
			<div class="entity-title">
				<h3 class="entity-label" title={entity.id}>{label}</h3>
				<span class="entity-type">{typeLabel}</span>
			</div>
			<CopyButton text={entity.id} variant="inline" label="Copy entity ID" />
		</div>

		<!-- ID Breakdown -->
		<section class="section" aria-label="Entity ID breakdown">
			<h4 class="section-title">Entity ID</h4>
			<div class="id-breakdown">
				<div class="id-part">
					<span class="id-label">org</span>
					<span class="id-value">{entity.idParts.org}</span>
				</div>
				<div class="id-part">
					<span class="id-label">platform</span>
					<span class="id-value">{entity.idParts.platform}</span>
				</div>
				<div class="id-part">
					<span class="id-label">domain</span>
					<span class="id-value">{entity.idParts.domain}</span>
				</div>
				<div class="id-part">
					<span class="id-label">system</span>
					<span class="id-value">{entity.idParts.system}</span>
				</div>
				<div class="id-part">
					<span class="id-label">type</span>
					<span class="id-value">{entity.idParts.type}</span>
				</div>
				<div class="id-part">
					<span class="id-label">instance</span>
					<span class="id-value">{entity.idParts.instance}</span>
				</div>
			</div>
		</section>

		<!-- Properties -->
		{#if entity.properties.length > 0}
			<section class="section" aria-label="Properties">
				<h4 class="section-title">Properties ({entity.properties.length})</h4>
				<div class="properties-list">
					{#each entity.properties as prop, idx (prop.predicate + idx)}
						<div class="property-row">
							<span
								class="property-predicate"
								style="color: {getPredicateColor(prop.predicate)}"
								title={prop.predicate}
							>
								{shortPredicate(prop.predicate)}
							</span>
							<span class="property-value" title={String(prop.object)}>
								{String(prop.object)}
							</span>
							<span
								class="property-confidence"
								style="opacity: {getConfidenceOpacity(prop.confidence)}"
								title="Confidence: {formatConfidence(prop.confidence)}"
							>
								{formatConfidence(prop.confidence)}
							</span>
						</div>
					{/each}
				</div>
			</section>
		{/if}

		<!-- Outgoing Relationships -->
		{#if entity.outgoing.length > 0}
			<section class="section" aria-label="Outgoing relationships">
				<h4 class="section-title">Outgoing ({entity.outgoing.length})</h4>
				<div class="relationships-list">
					{#each entity.outgoing as rel, idx (rel.id + ':' + idx)}
						<button
							class="relationship-row"
							onclick={() => handleRelatedEntityClick(rel.targetId)}
							title="Navigate to {rel.targetId}"
						>
							<span class="rel-predicate" style="color: {getPredicateColor(rel.predicate)}">
								{shortPredicate(rel.predicate)}
							</span>
							<span class="rel-arrow" aria-hidden="true">→</span>
							<span class="rel-target">{parseEntityId(rel.targetId).instance}</span>
							<span
								class="rel-confidence"
								style="opacity: {getConfidenceOpacity(rel.confidence)}"
								aria-label="Confidence {formatConfidence(rel.confidence)}"
							>
								{formatConfidence(rel.confidence)}
							</span>
						</button>
					{/each}
				</div>
			</section>
		{/if}

		<!-- Incoming Relationships -->
		{#if entity.incoming.length > 0}
			<section class="section" aria-label="Incoming relationships">
				<h4 class="section-title">Incoming ({entity.incoming.length})</h4>
				<div class="relationships-list">
					{#each entity.incoming as rel, idx (rel.id + ':' + idx)}
						<button
							class="relationship-row"
							onclick={() => handleRelatedEntityClick(rel.sourceId)}
							title="Navigate to {rel.sourceId}"
						>
							<span class="rel-source">{parseEntityId(rel.sourceId).instance}</span>
							<span class="rel-arrow" aria-hidden="true">←</span>
							<span class="rel-predicate" style="color: {getPredicateColor(rel.predicate)}">
								{shortPredicate(rel.predicate)}
							</span>
							<span
								class="rel-confidence"
								style="opacity: {getConfidenceOpacity(rel.confidence)}"
								aria-label="Confidence {formatConfidence(rel.confidence)}"
							>
								{formatConfidence(rel.confidence)}
							</span>
						</button>
					{/each}
				</div>
			</section>
		{/if}

		<!-- Last updated timestamp -->
		{#if entity.properties.length > 0}
			{@const latestProp = entity.properties.reduce((latest, prop) =>
				prop.timestamp > latest.timestamp ? prop : latest
			)}
			<section class="section section-footer">
				<span class="timestamp">Last updated: {formatTimestamp(latestProp.timestamp)}</span>
			</section>
		{/if}
	</div>
{/if}

<style>
	.detail-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow-y: auto;
		background: var(--ui-surface-secondary);
	}

	/* Header */
	.panel-header {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 12px;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-primary);
		flex-shrink: 0;
	}

	.entity-badge {
		width: 36px;
		height: 36px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		color: white;
		font-weight: 600;
		font-size: 16px;
		flex-shrink: 0;
	}

	.entity-title {
		flex: 1;
		min-width: 0;
	}

	.entity-label {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--ui-text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.entity-type {
		font-size: 11px;
		color: var(--ui-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	/* Sections */
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

	.section-footer {
		border-bottom: none;
	}

	/* ID Breakdown */
	.id-breakdown {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 6px;
	}

	.id-part {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.id-label {
		font-size: 9px;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
	}

	.id-value {
		font-size: 12px;
		color: var(--ui-text-primary);
		font-family: var(--font-mono, monospace);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	/* Properties */
	.properties-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.property-row {
		display: grid;
		grid-template-columns: 1fr 1fr auto;
		gap: 8px;
		align-items: center;
		padding: 4px 6px;
		background: var(--ui-surface-primary);
		border-radius: 4px;
		font-size: 11px;
	}

	.property-predicate {
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.property-value {
		color: var(--ui-text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		font-family: var(--font-mono, monospace);
	}

	.property-confidence {
		font-size: 10px;
		color: var(--ui-text-secondary);
		min-width: 32px;
		text-align: right;
	}

	/* Relationships */
	.relationships-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.relationship-row {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 6px 8px;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		font-size: 11px;
		cursor: pointer;
		transition: border-color 150ms ease, background-color 150ms ease;
		text-align: left;
		width: 100%;
		color: var(--ui-text-primary);
	}

	.relationship-row:hover {
		border-color: var(--ui-border-strong);
		background: var(--ui-surface-tertiary);
	}

	.rel-predicate {
		font-weight: 500;
		white-space: nowrap;
	}

	.rel-arrow {
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.rel-source,
	.rel-target {
		color: var(--ui-text-primary);
		font-family: var(--font-mono, monospace);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		flex: 1;
		min-width: 0;
	}

	.rel-confidence {
		font-size: 10px;
		color: var(--ui-text-secondary);
		flex-shrink: 0;
	}

	/* Footer */
	.timestamp {
		font-size: 10px;
		color: var(--ui-text-tertiary);
	}
</style>
