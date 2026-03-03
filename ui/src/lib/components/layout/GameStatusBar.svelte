<script lang="ts">
	import { worldStore } from '$stores/worldStore.svelte';
	import { api } from '$services/api';

	let toggling = $state(false);
	let now = $state(Date.now());

	// Tick every 30s while paused so pausedAgo stays fresh.
	$effect(() => {
		if (!worldStore.boardPaused) return;
		const interval = setInterval(() => { now = Date.now(); }, 30_000);
		return () => clearInterval(interval);
	});

	const pausedAgo = $derived.by(() => {
		if (!worldStore.boardPausedAt) return '';
		const diff = now - new Date(worldStore.boardPausedAt).getTime();
		if (diff < 60_000) return 'just now';
		const mins = Math.floor(diff / 60_000);
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		return `${hours}h ${mins % 60}m ago`;
	});

	async function handleToggle() {
		if (toggling) return;
		toggling = true;
		try {
			const result = worldStore.boardPaused
				? await api.resumeBoard()
				: await api.pauseBoard('dm');
			worldStore.setBoardPaused(result.paused, result.paused_at, result.paused_by);
		} catch (e) {
			const msg = e instanceof Error ? e.message : 'Failed to toggle board state';
			worldStore.setError(msg);
			console.error('Failed to toggle board state:', e);
		} finally {
			toggling = false;
		}
	}
</script>

<div class="status-bar" class:paused={worldStore.boardPaused} role="status" aria-live="polite" aria-label="Game board status">
	<div class="status-left">
		<span class="status-dot" class:running={!worldStore.boardPaused} class:paused={worldStore.boardPaused} aria-hidden="true"></span>
		{#if worldStore.boardPaused}
			<span class="status-label">Paused</span>
			{#if pausedAgo}
				<span class="status-detail">{pausedAgo}</span>
			{/if}
		{:else}
			<span class="status-label">Running</span>
			<span class="status-detail">
				{worldStore.stats.active_agents + worldStore.stats.idle_agents} agents, {worldStore.stats.active_quests} quests active
			</span>
		{/if}
	</div>
	<div class="status-right">
		<span class="connection-indicator" class:connected={worldStore.connected} data-testid="connection-status">
			{worldStore.connected ? 'Connected' : 'Disconnected'}
		</span>
		<button
			class="toggle-btn"
			class:primary={worldStore.boardPaused}
			onclick={handleToggle}
			disabled={toggling}
			aria-label={worldStore.boardPaused ? 'Resume game board' : 'Pause game board'}
		>
			{worldStore.boardPaused ? 'Resume' : 'Pause'}
		</button>
	</div>
</div>

<style>
	.status-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		height: 32px;
		padding: 0 var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-bottom: 1px solid var(--ui-border-subtle);
		font-size: 0.75rem;
		flex-shrink: 0;
	}

	.status-bar.paused {
		background: var(--status-warning-container);
	}

	.status-left {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.status-right {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.status-dot.running {
		background: var(--status-success);
	}

	.status-dot.paused {
		background: var(--status-warning);
	}

	.status-label {
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.status-detail {
		color: var(--ui-text-secondary);
	}

	.connection-indicator {
		padding: 2px 8px;
		border-radius: var(--radius-full);
		font-size: 0.675rem;
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.connection-indicator.connected {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.toggle-btn {
		padding: 2px 10px;
		border-radius: var(--radius-sm);
		font-size: 0.7rem;
		cursor: pointer;
		border: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
	}

	.toggle-btn:hover:not(:disabled) {
		background: var(--ui-interactive-secondary-hover);
	}

	.toggle-btn.primary {
		background: var(--button-primary-background);
		color: var(--button-primary-text);
		border-color: var(--button-primary-background);
	}

	.toggle-btn.primary:hover:not(:disabled) {
		background: var(--button-primary-background-hover);
	}

	.toggle-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
