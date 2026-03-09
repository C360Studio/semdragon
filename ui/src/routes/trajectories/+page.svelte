<script lang="ts">
	/**
	 * Trajectory Explorer - Search and browse trajectories
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import { worldStore } from '$stores/worldStore.svelte';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	type TrajectorySource = { type: 'quest' | 'battle'; title: string; status: string; href: string };

	// Pre-build map of trajectory ID → source for O(1) lookup per row
	const trajectorySourceMap = $derived.by(() => {
		const map = new Map<string, TrajectorySource>();
		for (const q of worldStore.questList) {
			if (q.loop_id) map.set(q.loop_id, { type: 'quest', title: q.title, status: q.status, href: `/quests/${q.id}` });
		}
		for (const b of worldStore.battleList) {
			if (b.loop_id) map.set(b.loop_id, { type: 'battle', title: `Battle #${String(b.id).slice(-6)}`, status: b.status, href: `/battles/${b.id}` });
		}
		return map;
	});

	const trajectoryIds = $derived([...trajectorySourceMap.keys()].slice(0, 30));
</script>

<svelte:head>
	<title>Trajectories - Semdragons</title>
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
		<div class="trajectories-page" data-testid="trajectories-page">
			<header class="page-header">
				<h1 data-testid="trajectories-heading">Trajectory Explorer</h1>
				<p class="page-description">
					Browse the full LLM execution history for quests and boss battle reviews.
				</p>
			</header>

			<div class="trajectory-list" data-testid="trajectory-list">
				<h2>Recent Trajectories</h2>
				{#each trajectoryIds as trajectoryId}
					{@const source = trajectorySourceMap.get(trajectoryId)}
					<a href="/trajectories/{trajectoryId}" class="trajectory-item" data-testid="trajectory-item">
						<span class="trajectory-id" data-testid="trajectory-item-id">{trajectoryId.slice(0, 12)}&hellip;</span>
						{#if source}
							<span class="trajectory-type-badge" data-type={source.type}>{source.type}</span>
							<span class="trajectory-quest" data-testid="trajectory-item-quest">{source.title}</span>
							<span class="trajectory-status" data-testid="trajectory-item-status" data-status={source.status}>{source.status}</span>
						{/if}
					</a>
				{:else}
					<div class="empty-state" data-testid="trajectory-empty-state">
						<p>No trajectories found.</p>
						<p>Trajectories are created when quests are posted to the board.</p>
					</div>
				{/each}
			</div>
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel">
			<header class="panel-header">
				<h2>Trajectory Details</h2>
			</header>
			<div class="details-content">
				<p class="empty-state">Select a trajectory to view details</p>
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	.trajectories-page {
		height: 100%;
		overflow-y: auto;
		padding: var(--spacing-lg);
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

	.trajectory-type-badge {
		font-size: 0.625rem;
		padding: 1px 6px;
		border-radius: var(--radius-sm);
		text-transform: uppercase;
		font-weight: 600;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.trajectory-type-badge[data-type='battle'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
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
	.trajectory-status[data-status='active'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}
	.trajectory-status[data-status='victory'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}
	.trajectory-status[data-status='defeat'] {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.empty-state {
		text-align: center;
		padding: var(--spacing-xl);
		color: var(--ui-text-tertiary);
	}

	.empty-state p {
		margin: var(--spacing-sm) 0;
	}

	/* Details Panel */
	.details-panel {
		height: 100%;
		display: flex;
		flex-direction: column;
	}

	.panel-header {
		padding: var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.panel-header h2 {
		font-size: 0.875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-secondary);
		margin: 0;
	}

	.details-content {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
	}
</style>
