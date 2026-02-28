<script lang="ts">
	/**
	 * DM Command Center - Main Dashboard
	 *
	 * The DM's Scrying Pool - overview of the entire game world.
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import { worldStore } from '$stores/worldStore.svelte';
	import type { GameEventType } from '$types';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// Event filter state
	type EventCategory = 'all' | 'quest' | 'agent' | 'battle' | 'guild';
	let eventFilter = $state<EventCategory>('all');

	// Filter events by category
	const filteredEvents = $derived(() => {
		const events = worldStore.recentEvents.slice(0, 15);
		if (eventFilter === 'all') return events;

		const categoryPrefixes: Record<EventCategory, string[]> = {
			all: [],
			quest: ['quest.'],
			agent: ['agent.'],
			battle: ['battle.'],
			guild: ['guild.', 'party.']
		};

		const prefixes = categoryPrefixes[eventFilter];
		return events.filter((e) => prefixes.some((p) => e.type.startsWith(p)));
	});

	// Get icon for event category
	function eventIcon(type: GameEventType): string {
		if (type.startsWith('quest.')) return 'Q';
		if (type.startsWith('agent.')) return 'A';
		if (type.startsWith('battle.')) return 'B';
		if (type.startsWith('guild.') || type.startsWith('party.')) return 'G';
		if (type.startsWith('store.')) return 'S';
		if (type.startsWith('dm.')) return 'D';
		return '*';
	}

	// Format relative time
	function formatTime(timestamp: number): string {
		const now = Date.now();
		const diff = now - timestamp;
		if (diff < 60000) return 'just now';
		if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
		if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
		return new Date(timestamp).toLocaleDateString();
	}

	// Format large numbers
	function formatNumber(n: number): string {
		if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
		if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
		return n.toString();
	}
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
				<a href="/store" class="nav-item">
					<span class="nav-icon">S</span>
					<span class="nav-label">Store</span>
				</a>
			</nav>

			<div class="event-feed">
				<header class="section-header">
					<h3>Recent Activity</h3>
					<select
						bind:value={eventFilter}
						class="event-filter"
						aria-label="Filter events by category"
					>
						<option value="all">All</option>
						<option value="quest">Quests</option>
						<option value="agent">Agents</option>
						<option value="battle">Battles</option>
						<option value="guild">Guilds</option>
					</select>
				</header>
				<ul class="event-list">
					{#each filteredEvents() as event}
						<li class="event-item" data-category={eventIcon(event.type)}>
							<span class="event-icon">{eventIcon(event.type)}</span>
							<span class="event-type">{event.type.split('.').slice(-1)[0]}</span>
							<span class="event-time">{formatTime(event.timestamp)}</span>
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
				<div class="header-actions">
					<div class="connection-status" class:connected={worldStore.connected}>
						{worldStore.connected ? 'Connected' : 'Disconnected'}
					</div>
				</div>
			</header>

			{#if worldStore.loading}
				<div class="loading">Loading world state...</div>
			{:else if worldStore.error}
				<div class="error">{worldStore.error}</div>
			{:else}
				<!-- Stats Grid -->
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
					<div class="stat-card">
						<span class="stat-value">{formatNumber(worldStore.totalXpEarned)}</span>
						<span class="stat-label">Total XP Earned</span>
					</div>
					<div class="stat-card">
						<span class="stat-value">{worldStore.partyList.length}</span>
						<span class="stat-label">Active Parties</span>
					</div>
					<div class="stat-card">
						<span class="stat-value">{worldStore.guildList.length}</span>
						<span class="stat-label">Guilds</span>
					</div>
					<div class="stat-card">
						<span class="stat-value">{worldStore.battleStats.winRate.toFixed(0)}%</span>
						<span class="stat-label">Battle Win Rate</span>
					</div>
				</div>

				<!-- Agent Distribution -->
				<section class="dashboard-section">
					<h2>Agent Distribution by Tier</h2>
					<div class="tier-bars">
						{#each worldStore.tierDistribution as tier}
							<div class="tier-row">
								<span class="tier-name">{tier.name}</span>
								<div class="tier-bar-container">
									<div
										class="tier-bar"
										style="width: {Math.max(tier.percentage, 2)}%"
										data-tier={tier.tier}
									></div>
								</div>
								<span class="tier-count">{tier.count}</span>
							</div>
						{/each}
					</div>
				</section>

				<!-- Active Quests -->
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

				<!-- Active Battles -->
				<section class="dashboard-section">
					<h2>Active Battles</h2>
					<div class="battle-summary">
						{#each worldStore.activeBattles.slice(0, 5) as battle}
							<a href="/battles/{battle.id}" class="battle-row">
								<span class="battle-id">Battle #{battle.id.slice(-6)}</span>
								<span class="battle-status" data-status={battle.status}>{battle.status}</span>
							</a>
						{:else}
							<p class="empty-state">No active battles</p>
						{/each}
					</div>
				</section>

				<!-- Guild Activity -->
				<section class="dashboard-section">
					<h2>Guild Activity</h2>
					<div class="guild-summary">
						{#each worldStore.guildList.slice(0, 5) as guild}
							<a href="/guilds/{guild.id}" class="guild-row">
								<span class="guild-name">{guild.name}</span>
								<span class="guild-members">{guild.members.length} members</span>
								<span class="guild-reputation">{(guild.reputation * 100).toFixed(0)}%</span>
							</a>
						{:else}
							<p class="empty-state">No guilds formed</p>
						{/each}
					</div>
				</section>

				<!-- Active Parties -->
				<section class="dashboard-section">
					<h2>Active Parties</h2>
					<div class="party-summary">
						{#each worldStore.partyList.filter((p) => p.status === 'active') as party}
							<div class="party-row">
								<span class="party-name">{party.name || `Party #${party.id.slice(-6)}`}</span>
								<span class="party-quest">Quest: {party.quest_id.slice(-6)}</span>
								<span class="party-size">{party.members.length} agents</span>
							</div>
						{:else}
							<p class="empty-state">No active parties</p>
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
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.section-header h3 {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: 0;
	}

	.event-filter {
		font-size: 0.75rem;
		padding: 2px 6px;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		color: var(--ui-text-secondary);
		cursor: pointer;
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
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
		font-size: 0.75rem;
	}

	.event-icon {
		width: 18px;
		height: 18px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-sm);
		font-weight: 600;
		font-size: 0.625rem;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
	}

	.event-item[data-category='Q'] .event-icon {
		background: var(--quest-posted-container, #e3f2fd);
		color: var(--quest-posted, #1976d2);
	}

	.event-item[data-category='A'] .event-icon {
		background: var(--status-success-container, #e8f5e9);
		color: var(--status-success, #388e3c);
	}

	.event-item[data-category='B'] .event-icon {
		background: var(--status-warning-container, #fff3e0);
		color: var(--status-warning, #f57c00);
	}

	.event-item[data-category='G'] .event-icon {
		background: var(--tier-master-container, #f3e5f5);
		color: var(--tier-master, #7b1fa2);
	}

	.event-type {
		flex: 1;
		color: var(--ui-text-secondary);
		text-transform: capitalize;
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
		flex-wrap: wrap;
		gap: var(--spacing-md);
	}

	.dashboard-header h1 {
		margin: 0;
		font-size: 1.5rem;
	}

	.header-actions {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
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
		grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
		gap: var(--spacing-md);
		margin-bottom: var(--spacing-xl);
	}

	.stat-card {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-md);
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
	}

	.stat-value {
		font-size: 1.5rem;
		font-weight: 700;
		color: var(--ui-interactive-primary);
	}

	.stat-label {
		font-size: 0.625rem;
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

	/* Tier Distribution Bars */
	.tier-bars {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.tier-row {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
	}

	.tier-name {
		width: 100px;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
	}

	.tier-bar-container {
		flex: 1;
		height: 20px;
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		overflow: hidden;
	}

	.tier-bar {
		height: 100%;
		border-radius: var(--radius-md);
		transition: width 300ms ease;
	}

	.tier-bar[data-tier='0'] {
		background: var(--tier-apprentice, #78909c);
	}

	.tier-bar[data-tier='1'] {
		background: var(--tier-journeyman, #4caf50);
	}

	.tier-bar[data-tier='2'] {
		background: var(--tier-expert, #2196f3);
	}

	.tier-bar[data-tier='3'] {
		background: var(--tier-master, #9c27b0);
	}

	.tier-bar[data-tier='4'] {
		background: var(--tier-grandmaster, #ff9800);
	}

	.tier-count {
		width: 30px;
		text-align: right;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	/* Quest, Battle, Guild, Party Summaries */
	.quest-summary,
	.battle-summary,
	.guild-summary,
	.party-summary {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
	}

	.quest-row,
	.battle-row,
	.guild-row,
	.party-row {
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
	.battle-row:hover,
	.guild-row:hover {
		border-color: var(--ui-border-interactive);
		text-decoration: none;
	}

	.quest-title,
	.guild-name,
	.party-name {
		flex: 1;
		font-weight: 500;
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

	.quest-status[data-status='claimed'] {
		background: var(--quest-claimed-container, #e3f2fd);
		color: var(--quest-claimed, #1565c0);
	}

	.quest-status[data-status='in_review'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.guild-members,
	.party-quest,
	.party-size {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		padding: 2px 8px;
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-full);
	}

	.guild-reputation {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--status-success);
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
