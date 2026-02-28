<script lang="ts">
	/**
	 * DM Command Center - Main Dashboard
	 *
	 * The DM's Scrying Pool - overview of the entire game world.
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import { worldStore } from '$stores/worldStore.svelte';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);
</script>

<svelte:head>
	<title>DM Command Center - Semdragons</title>
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
		<div class="explorer-panel">
			<header class="panel-header">
				<h2>Explorer</h2>
			</header>
			<nav class="explorer-nav">
				<a href="/quests" class="nav-item">
					<span class="nav-icon">Q</span>
					<span class="nav-label">Quest Board</span>
					<span class="nav-count">{worldStore.questList.length}</span>
				</a>
				<a href="/agents" class="nav-item">
					<span class="nav-icon">A</span>
					<span class="nav-label">Agent Roster</span>
					<span class="nav-count">{worldStore.agentList.length}</span>
				</a>
				<a href="/battles" class="nav-item">
					<span class="nav-icon">B</span>
					<span class="nav-label">Boss Battles</span>
					<span class="nav-count">{worldStore.activeBattles.length}</span>
				</a>
				<a href="/guilds" class="nav-item">
					<span class="nav-icon">G</span>
					<span class="nav-label">Guilds</span>
					<span class="nav-count">{worldStore.guildList.length}</span>
				</a>
				<a href="/trajectories" class="nav-item">
					<span class="nav-icon">T</span>
					<span class="nav-label">Trajectories</span>
				</a>
			</nav>

			<div class="event-feed">
				<header class="section-header">
					<h3>Recent Activity</h3>
				</header>
				<ul class="event-list">
					{#each worldStore.recentEvents.slice(0, 10) as event}
						<li class="event-item">
							<span class="event-type">{event.type}</span>
							<span class="event-time">{new Date(event.timestamp).toLocaleTimeString()}</span>
						</li>
					{:else}
						<li class="event-empty">No recent events</li>
					{/each}
				</ul>
			</div>
		</div>
	{/snippet}

	{#snippet centerPanel()}
		<div class="dashboard">
			<header class="dashboard-header">
				<h1>The DM's Scrying Pool</h1>
				<div class="connection-status" class:connected={worldStore.connected}>
					{worldStore.connected ? 'Connected' : 'Disconnected'}
				</div>
			</header>

			{#if worldStore.loading}
				<div class="loading">Loading world state...</div>
			{:else if worldStore.error}
				<div class="error">{worldStore.error}</div>
			{:else}
				<div class="stats-grid">
					<div class="stat-card">
						<span class="stat-value">{worldStore.stats.active_agents}</span>
						<span class="stat-label">Active Agents</span>
					</div>
					<div class="stat-card">
						<span class="stat-value">{worldStore.stats.idle_agents}</span>
						<span class="stat-label">Idle Agents</span>
					</div>
					<div class="stat-card">
						<span class="stat-value">{worldStore.stats.open_quests}</span>
						<span class="stat-label">Open Quests</span>
					</div>
					<div class="stat-card">
						<span class="stat-value">{worldStore.stats.active_quests}</span>
						<span class="stat-label">Active Quests</span>
					</div>
					<div class="stat-card">
						<span class="stat-value">{(worldStore.stats.completion_rate * 100).toFixed(1)}%</span>
						<span class="stat-label">Completion Rate</span>
					</div>
					<div class="stat-card">
						<span class="stat-value">{(worldStore.stats.avg_quality * 100).toFixed(1)}%</span>
						<span class="stat-label">Avg Quality</span>
					</div>
				</div>

				<section class="dashboard-section">
					<h2>Active Quests</h2>
					<div class="quest-summary">
						{#each worldStore.activeQuests.slice(0, 5) as quest}
							<a href="/quests/{quest.id}" class="quest-row">
								<span class="quest-title">{quest.title}</span>
								<span class="quest-status" data-status={quest.status}>{quest.status}</span>
							</a>
						{:else}
							<p class="empty-state">No active quests</p>
						{/each}
					</div>
				</section>

				<section class="dashboard-section">
					<h2>Active Battles</h2>
					<div class="battle-summary">
						{#each worldStore.activeBattles.slice(0, 5) as battle}
							<a href="/battles/{battle.id}" class="battle-row">
								<span class="battle-id">Battle #{battle.id}</span>
								<span class="battle-status" data-status={battle.status}>{battle.status}</span>
							</a>
						{:else}
							<p class="empty-state">No active battles</p>
						{/each}
					</div>
				</section>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel">
			<header class="panel-header">
				<h2>Details</h2>
			</header>
			<div class="details-content">
				{#if worldStore.selectedQuest}
					<section class="detail-section">
						<h3>Selected Quest</h3>
						<dl class="detail-list">
							<dt>Title</dt>
							<dd>{worldStore.selectedQuest.title}</dd>
							<dt>Status</dt>
							<dd>{worldStore.selectedQuest.status}</dd>
							<dt>Difficulty</dt>
							<dd>{worldStore.selectedQuest.difficulty}</dd>
							<dt>XP</dt>
							<dd>{worldStore.selectedQuest.base_xp}</dd>
						</dl>
					</section>
				{:else if worldStore.selectedAgent}
					<section class="detail-section">
						<h3>Selected Agent</h3>
						<dl class="detail-list">
							<dt>Name</dt>
							<dd>{worldStore.selectedAgent.name}</dd>
							<dt>Level</dt>
							<dd>{worldStore.selectedAgent.level}</dd>
							<dt>Status</dt>
							<dd>{worldStore.selectedAgent.status}</dd>
							<dt>XP</dt>
							<dd>{worldStore.selectedAgent.xp} / {worldStore.selectedAgent.xp_to_level}</dd>
						</dl>
					</section>
				{:else}
					<p class="empty-state">Select an entity to view details</p>
				{/if}
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	/* Explorer Panel */
	.explorer-panel {
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

	.explorer-nav {
		padding: var(--spacing-sm);
	}

	.nav-item {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		border-radius: var(--radius-md);
		color: var(--ui-text-primary);
		text-decoration: none;
		transition: background-color 150ms ease;
	}

	.nav-item:hover {
		background: var(--ui-surface-tertiary);
		text-decoration: none;
	}

	.nav-icon {
		width: 24px;
		height: 24px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
		font-weight: 600;
		font-size: 0.75rem;
	}

	.nav-label {
		flex: 1;
	}

	.nav-count {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-tertiary);
		padding: 2px 6px;
		border-radius: var(--radius-full);
	}

	/* Event Feed */
	.event-feed {
		flex: 1;
		overflow: hidden;
		display: flex;
		flex-direction: column;
		border-top: 1px solid var(--ui-border-subtle);
		margin-top: var(--spacing-md);
	}

	.section-header {
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-tertiary);
	}

	.section-header h3 {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: 0;
	}

	.event-list {
		list-style: none;
		padding: 0;
		margin: 0;
		overflow-y: auto;
		flex: 1;
	}

	.event-item {
		display: flex;
		justify-content: space-between;
		padding: var(--spacing-sm) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
		font-size: 0.75rem;
	}

	.event-type {
		color: var(--ui-text-secondary);
	}

	.event-time {
		color: var(--ui-text-tertiary);
	}

	.event-empty {
		padding: var(--spacing-md);
		color: var(--ui-text-tertiary);
		text-align: center;
	}

	/* Dashboard */
	.dashboard {
		height: 100%;
		overflow-y: auto;
		padding: var(--spacing-lg);
	}

	.dashboard-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-lg);
	}

	.dashboard-header h1 {
		margin: 0;
		font-size: 1.5rem;
	}

	.connection-status {
		padding: var(--spacing-xs) var(--spacing-sm);
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.connection-status.connected {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	/* Stats Grid */
	.stats-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		gap: var(--spacing-md);
		margin-bottom: var(--spacing-xl);
	}

	.stat-card {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-lg);
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
	}

	.stat-value {
		font-size: 2rem;
		font-weight: 700;
		color: var(--ui-interactive-primary);
	}

	.stat-label {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-top: var(--spacing-xs);
	}

	/* Dashboard Sections */
	.dashboard-section {
		margin-bottom: var(--spacing-xl);
	}

	.dashboard-section h2 {
		font-size: 1rem;
		margin-bottom: var(--spacing-md);
		color: var(--ui-text-secondary);
	}

	.quest-summary,
	.battle-summary {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
	}

	.quest-row,
	.battle-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		color: var(--ui-text-primary);
		text-decoration: none;
		transition: border-color 150ms ease;
	}

	.quest-row:hover,
	.battle-row:hover {
		border-color: var(--ui-border-interactive);
		text-decoration: none;
	}

	.quest-status,
	.battle-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		background: var(--ui-surface-tertiary);
	}

	.quest-status[data-status='in_progress'],
	.battle-status[data-status='active'] {
		background: var(--quest-in-progress-container);
		color: var(--quest-in-progress);
	}

	.quest-status[data-status='completed'],
	.battle-status[data-status='victory'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}

	/* Details Panel */
	.details-panel {
		height: 100%;
		display: flex;
		flex-direction: column;
	}

	.details-content {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
	}

	.detail-section h3 {
		font-size: 0.875rem;
		margin-bottom: var(--spacing-md);
	}

	.detail-list {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
	}

	.detail-list dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.detail-list dd {
		margin: 0;
	}

	/* Utility */
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
</style>
