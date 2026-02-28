<script lang="ts">
	/**
	 * Trajectory Timeline View - Full event timeline for a trajectory
	 */

	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { api, type TrajectoryEvent } from '$services/api';
	import { worldStore } from '$stores/worldStore.svelte';

	const trajectoryId = $derived($page.params.id ?? '');
	const quest = $derived(worldStore.questList.find((q) => q.trajectory_id === trajectoryId));

	let events = $state<TrajectoryEvent[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(async () => {
		if (!trajectoryId) return;
		try {
			events = await api.getTrajectory(trajectoryId);
		} catch (err) {
			console.error('Failed to load trajectory:', err);
			error = 'Failed to load trajectory events';
		} finally {
			loading = false;
		}
	});

	function formatTime(timestamp: number): string {
		return new Date(timestamp).toLocaleString();
	}

	function formatDuration(start: number, end: number): string {
		const ms = end - start;
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
		return `${(ms / 60000).toFixed(1)}m`;
	}
</script>

<svelte:head>
	<title>Trajectory {trajectoryId.slice(0, 8)} - Semdragons</title>
</svelte:head>

<div class="trajectory-page">
	<header class="page-header">
		<a href="/trajectories" class="back-link">Back to Trajectories</a>
	</header>

	<div class="trajectory-content">
		<div class="trajectory-header">
			<h1>Trajectory Timeline</h1>
			<span class="trajectory-id">{trajectoryId}</span>
		</div>

		{#if quest}
			<div class="quest-context">
				<a href="/quests/{quest.id}" class="quest-link">
					<span class="quest-title">{quest.title}</span>
					<span class="quest-status" data-status={quest.status}>{quest.status}</span>
				</a>
			</div>
		{/if}

		{#if loading}
			<div class="loading">Loading trajectory events...</div>
		{:else if error}
			<div class="error">{error}</div>
		{:else if events.length === 0}
			<div class="empty-state">
				<p>No events found for this trajectory.</p>
				<p>Events will appear here as the quest progresses.</p>
			</div>
		{:else}
			<div class="timeline">
				{#each events as event, i}
					{@const prevEvent = events[i - 1]}
					<div class="timeline-event">
						<div class="event-marker"></div>
						<div class="event-content">
							<div class="event-header">
								<span class="event-type">{event.type}</span>
								<span class="event-time">{formatTime(event.timestamp)}</span>
								{#if prevEvent}
									<span class="event-delta">
										+{formatDuration(prevEvent.timestamp, event.timestamp)}
									</span>
								{/if}
							</div>
							{#if event.data}
								<pre class="event-data">{JSON.stringify(event.data, null, 2)}</pre>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	.trajectory-page {
		height: 100%;
		overflow-y: auto;
		padding: var(--spacing-lg);
		background: var(--ui-surface-primary);
	}

	.page-header {
		margin-bottom: var(--spacing-lg);
	}

	.back-link {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
	}

	.trajectory-content {
		max-width: 800px;
		margin: 0 auto;
	}

	.trajectory-header {
		margin-bottom: var(--spacing-md);
	}

	.trajectory-header h1 {
		margin: 0 0 var(--spacing-xs);
	}

	.trajectory-id {
		font-family: monospace;
		font-size: 0.875rem;
		color: var(--ui-text-tertiary);
	}

	.quest-context {
		margin-bottom: var(--spacing-lg);
	}

	.quest-link {
		display: inline-flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		color: var(--ui-text-primary);
		text-decoration: none;
	}

	.quest-link:hover {
		border-color: var(--ui-border-interactive);
		text-decoration: none;
	}

	.quest-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.quest-status[data-status='completed'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}

	/* Timeline */
	.timeline {
		position: relative;
		padding-left: var(--spacing-xl);
	}

	.timeline::before {
		content: '';
		position: absolute;
		left: 8px;
		top: 0;
		bottom: 0;
		width: 2px;
		background: var(--ui-border-subtle);
	}

	.timeline-event {
		position: relative;
		margin-bottom: var(--spacing-md);
	}

	.event-marker {
		position: absolute;
		left: calc(-1 * var(--spacing-xl) + 4px);
		top: 4px;
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: var(--ui-interactive-primary);
		border: 2px solid var(--ui-surface-primary);
	}

	.event-content {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		padding: var(--spacing-md);
	}

	.event-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
		margin-bottom: var(--spacing-sm);
	}

	.event-type {
		font-weight: 600;
		font-size: 0.875rem;
	}

	.event-time {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.event-delta {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-tertiary);
		padding: 2px 6px;
		border-radius: var(--radius-sm);
	}

	.event-data {
		font-family: monospace;
		font-size: 0.75rem;
		background: var(--ui-surface-primary);
		padding: var(--spacing-sm);
		border-radius: var(--radius-sm);
		overflow-x: auto;
		margin: 0;
		color: var(--ui-text-secondary);
	}

	.loading,
	.error,
	.empty-state {
		text-align: center;
		padding: var(--spacing-xl);
		color: var(--ui-text-tertiary);
	}

	.error {
		color: var(--status-error);
	}

	.empty-state p {
		margin: var(--spacing-sm) 0;
	}
</style>
