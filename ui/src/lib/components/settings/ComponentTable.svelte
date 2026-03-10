<script lang="ts">
	/**
	 * ComponentTable - Displays processor/component status with health indicators.
	 */
	import type { ComponentInfo } from '$types';

	let { components }: { components: ComponentInfo[] } = $props();

	function formatUptime(seconds?: number): string {
		if (seconds === undefined) return '--';
		if (seconds === 0) return '0s';
		if (seconds < 60) return `${seconds}s`;
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
		return `${Math.floor(seconds / 86400)}d`;
	}

	const healthyCt = $derived(components.filter((c) => c.healthy).length);
	const runningCt = $derived(components.filter((c) => c.running).length);
</script>

<div class="component-summary">
	<span class="summary-stat">{runningCt}/{components.length} running</span>
	<span class="summary-stat">{runningCt > 0 ? `${healthyCt}/${runningCt}` : '\u2014'} healthy</span>
</div>

<div class="component-table-wrapper">
	<table class="component-table" data-testid="component-table">
		<thead>
			<tr>
				<th>Name</th>
				<th>Type</th>
				<th>Status</th>
				<th>Uptime</th>
				<th>Errors</th>
			</tr>
		</thead>
		<tbody>
			{#each components as comp (comp.name)}
				<tr class:disabled-row={!comp.enabled}>
					<td class="comp-name">{comp.name}</td>
					<td class="comp-type">{comp.type}</td>
					<td>
						{#if !comp.enabled}
							<span class="comp-status" data-status="disabled">Disabled</span>
						{:else if !comp.running}
							<span class="comp-status" data-status="stopped">Stopped</span>
						{:else if comp.healthy}
							<span class="comp-status" data-status="healthy">Healthy</span>
						{:else}
							<span class="comp-status" data-status="unhealthy" title={comp.last_error ?? ''}>
								{comp.status ?? 'Unhealthy'}
							</span>
						{/if}
					</td>
					<td class="mono">{formatUptime(comp.uptime_seconds)}</td>
					<td>
						{#if (comp.error_count ?? 0) > 0}
							<span class="error-count">{comp.error_count}</span>
						{:else}
							<span class="no-errors">0</span>
						{/if}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

<style>
	.component-summary {
		display: flex;
		gap: var(--spacing-lg);
		margin-bottom: var(--spacing-md);
	}

	.summary-stat {
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
	}

	.component-table-wrapper {
		overflow-x: auto;
	}

	.component-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.8125rem;
	}

	.component-table th {
		text-align: left;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-sm) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.component-table td {
		padding: var(--spacing-sm) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.component-table tbody tr:last-child td {
		border-bottom: none;
	}

	.component-table tbody tr:hover {
		background: var(--ui-surface-tertiary);
	}

	.disabled-row {
		opacity: 0.5;
	}

	.comp-name {
		font-weight: 500;
	}

	.comp-type {
		color: var(--ui-text-secondary);
	}

	.comp-status {
		font-size: 0.75rem;
		font-weight: 500;
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.comp-status[data-status='healthy'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.comp-status[data-status='unhealthy'] {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.comp-status[data-status='stopped'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.comp-status[data-status='disabled'] {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
	}

	.mono {
		font-family: monospace;
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
	}

	.error-count {
		font-family: monospace;
		font-size: 0.75rem;
		color: var(--status-error);
		font-weight: 600;
	}

	.no-errors {
		font-family: monospace;
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}
</style>
