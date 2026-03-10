<script lang="ts">
	/**
	 * ConnectionCard - Displays NATS connection status and latency.
	 */
	import type { NATSInfo, OverallHealth } from '$types';

	let {
		nats,
		overall
	}: {
		nats: NATSInfo;
		overall?: OverallHealth;
	} = $props();
</script>

<div class="connection-grid">
	<div class="connection-item" aria-label="NATS: {nats.connected ? 'Connected' : 'Disconnected'}">
		<span class="status-dot" data-status={nats.connected ? 'ok' : 'error'} aria-hidden="true"></span>
		<div class="connection-detail">
			<span class="connection-label">NATS</span>
			<span class="connection-value">{nats.connected ? 'Connected' : 'Disconnected'}</span>
		</div>
		<span class="connection-meta">{nats.url}</span>
	</div>
	{#if nats.latency_ms !== undefined}
		<div class="connection-item" aria-label="Latency: {nats.latency_ms.toFixed(1)}ms">
			<span class="status-dot" data-status={nats.latency_ms < 100 ? 'ok' : 'warning'} aria-hidden="true"></span>
			<div class="connection-detail">
				<span class="connection-label">Latency</span>
				<span class="connection-value">{nats.latency_ms.toFixed(1)}ms</span>
			</div>
		</div>
	{/if}
	{#if overall}
		<div class="connection-item" aria-label="Overall health: {overall}">
			<span class="status-dot" data-status={overall === 'healthy' ? 'ok' : overall === 'degraded' ? 'warning' : 'error'} aria-hidden="true"></span>
			<div class="connection-detail">
				<span class="connection-label">Overall</span>
				<span class="connection-value capitalize">{overall}</span>
			</div>
		</div>
	{/if}
</div>

<style>
	.connection-grid {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.connection-item {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-primary);
		border-radius: var(--radius-md);
		border: 1px solid var(--ui-border-subtle);
	}

	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: var(--radius-full);
		flex-shrink: 0;
	}

	.status-dot[data-status='ok'] {
		background: var(--status-success);
	}

	.status-dot[data-status='warning'] {
		background: var(--status-warning);
	}

	.status-dot[data-status='error'] {
		background: var(--status-error);
	}

	.connection-detail {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-width: 0;
	}

	.connection-label {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.connection-value {
		font-size: 0.875rem;
		font-weight: 500;
	}

	.capitalize {
		text-transform: capitalize;
	}

	.connection-meta {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		font-family: monospace;
	}
</style>
