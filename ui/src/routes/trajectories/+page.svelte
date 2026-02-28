<script lang="ts">
	/**
	 * Trajectory Explorer - Search and browse trajectories
	 */

	import { worldStore } from '$stores/worldStore.svelte';

	// Get unique trajectory IDs from quests
	const trajectoryIds = $derived(
		[...new Set(worldStore.questList.map((q) => q.trajectory_id).filter(Boolean))].slice(0, 20)
	);
</script>

<svelte:head>
	<title>Trajectories - Semdragons</title>
</svelte:head>

<div class="trajectories-page">
	<header class="page-header">
		<h1>Trajectory Explorer</h1>
		<p class="page-description">
			Browse the full event timeline for quests. Each trajectory captures the complete history of a
			quest from creation to completion.
		</p>
	</header>

	<div class="trajectory-list">
		<h2>Recent Trajectories</h2>
		{#each trajectoryIds as trajectoryId}
			{@const quest = worldStore.questList.find((q) => q.trajectory_id === trajectoryId)}
			<a href="/trajectories/{trajectoryId}" class="trajectory-item">
				<span class="trajectory-id">{trajectoryId.slice(0, 12)}...</span>
				{#if quest}
					<span class="trajectory-quest">{quest.title}</span>
					<span class="trajectory-status" data-status={quest.status}>{quest.status}</span>
				{/if}
			</a>
		{:else}
			<div class="empty-state">
				<p>No trajectories found.</p>
				<p>Trajectories are created when quests are posted to the board.</p>
			</div>
		{/each}
	</div>
</div>

<style>
	.trajectories-page {
		height: 100%;
		overflow-y: auto;
		padding: var(--spacing-lg);
		background: var(--ui-surface-primary);
		max-width: 800px;
		margin: 0 auto;
	}

	.page-header {
		margin-bottom: var(--spacing-xl);
	}

	.page-header h1 {
		margin: 0 0 var(--spacing-sm);
	}

	.page-description {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		margin: 0;
	}

	.trajectory-list h2 {
		font-size: 0.875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin-bottom: var(--spacing-md);
	}

	.trajectory-item {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
		padding: var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		margin-bottom: var(--spacing-sm);
		color: var(--ui-text-primary);
		text-decoration: none;
		transition: border-color 150ms ease;
	}

	.trajectory-item:hover {
		border-color: var(--ui-border-interactive);
		text-decoration: none;
	}

	.trajectory-id {
		font-family: monospace;
		font-size: 0.875rem;
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.trajectory-quest {
		flex: 1;
	}

	.trajectory-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.trajectory-status[data-status='posted'] {
		background: var(--quest-posted-container);
		color: var(--quest-posted);
	}
	.trajectory-status[data-status='claimed'] {
		background: var(--quest-claimed-container);
		color: var(--quest-claimed);
	}
	.trajectory-status[data-status='in_progress'] {
		background: var(--quest-in-progress-container);
		color: var(--quest-in-progress);
	}
	.trajectory-status[data-status='in_review'] {
		background: var(--quest-in-review-container);
		color: var(--quest-in-review);
	}
	.trajectory-status[data-status='completed'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}
	.trajectory-status[data-status='failed'] {
		background: var(--quest-failed-container);
		color: var(--quest-failed);
	}

	.empty-state {
		text-align: center;
		padding: var(--spacing-xl);
		color: var(--ui-text-tertiary);
	}

	.empty-state p {
		margin: var(--spacing-sm) 0;
	}
</style>
