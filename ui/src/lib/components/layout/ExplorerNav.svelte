<script lang="ts">
	/**
	 * ExplorerNav - Left panel explorer navigation
	 *
	 * Renders the site navigation links with entity counts and optionally
	 * the Recent Activity event feed. Designed for use inside the left panel
	 * of ThreePanelLayout.
	 */

	import { page } from '$app/state';
	import { worldStore } from '$stores/worldStore.svelte';
	import type { GameEventType } from '$types';

	type EventCategory = 'all' | 'quest' | 'agent' | 'battle' | 'guild';

	let { showActivity = true }: { showActivity?: boolean } = $props();

	// Active route highlighting — exact match for `/`, prefix match for others
	const currentPath = $derived(page.url.pathname);

	function isActive(href: string): boolean {
		if (href === '/') return currentPath === '/';
		return currentPath.startsWith(href);
	}

	// Event filter state — only used when showActivity is true
	let eventFilter = $state<EventCategory>('all');

	const filteredEvents = $derived.by(() => {
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

	// Get icon letter for an event type
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
</script>

<div class="explorer-panel">
	<header class="panel-header">
		<h2>Explorer</h2>
	</header>
	<nav class="explorer-nav" aria-label="Main navigation">
		<a href="/" class="nav-item" class:active={isActive('/')} data-testid="nav-dashboard">
			<span class="nav-icon">D</span>
			<span class="nav-label">Dashboard</span>
		</a>
		<a href="/quests" class="nav-item" class:active={isActive('/quests')} data-testid="nav-quests">
			<span class="nav-icon">Q</span>
			<span class="nav-label">Quest Board</span>
			<span class="nav-count" data-testid="nav-count-quests">{worldStore.questList.length}</span>
		</a>
		<a href="/agents" class="nav-item" class:active={isActive('/agents')} data-testid="nav-agents">
			<span class="nav-icon">A</span>
			<span class="nav-label">Agent Roster</span>
			<span class="nav-count" data-testid="nav-count-agents">{worldStore.agentList.length}</span>
		</a>
		<a href="/battles" class="nav-item" class:active={isActive('/battles')} data-testid="nav-battles">
			<span class="nav-icon">B</span>
			<span class="nav-label">Boss Battles</span>
			<span class="nav-count" data-testid="nav-count-battles">{worldStore.battleList.length}</span>
		</a>
		<a href="/parties" class="nav-item" class:active={isActive('/parties')} data-testid="nav-parties">
			<span class="nav-icon">P</span>
			<span class="nav-label">Parties</span>
			<span class="nav-count" data-testid="nav-count-parties">{worldStore.partyList.length}</span>
		</a>
		<a href="/guilds" class="nav-item" class:active={isActive('/guilds')} data-testid="nav-guilds">
			<span class="nav-icon">G</span>
			<span class="nav-label">Guilds</span>
			<span class="nav-count" data-testid="nav-count-guilds">{worldStore.guildList.length}</span>
		</a>
		<a href="/trajectories" class="nav-item" class:active={isActive('/trajectories')} data-testid="nav-trajectories">
			<span class="nav-icon">T</span>
			<span class="nav-label">Trajectories</span>
		</a>
		<a href="/graph" class="nav-item" class:active={isActive('/graph')} data-testid="nav-graph">
			<span class="nav-icon">K</span>
			<span class="nav-label">Knowledge Graph</span>
		</a>
		<a href="/store" class="nav-item" class:active={isActive('/store')} data-testid="nav-store">
			<span class="nav-icon">S</span>
			<span class="nav-label">Store</span>
		</a>
		<a href="/workspace" class="nav-item" class:active={isActive('/workspace')} data-testid="nav-workspace">
			<span class="nav-icon">W</span>
			<span class="nav-label">Workspace</span>
		</a>
		<a href="/settings" class="nav-item" class:active={isActive('/settings')} data-testid="nav-settings">
			<span class="nav-icon">*</span>
			<span class="nav-label">Settings</span>
		</a>
	</nav>

	{#if showActivity}
		<div class="event-feed" data-testid="event-feed">
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
				{#each filteredEvents as event}
					<li class="event-item" data-testid="event-item" data-category={eventIcon(event.type)}>
						<span class="event-icon">{eventIcon(event.type)}</span>
						<span class="event-type">{event.type.split('.').slice(-1)[0]}</span>
						<span class="event-time">{formatTime(event.timestamp)}</span>
					</li>
				{:else}
					<li class="event-empty">No recent events</li>
				{/each}
			</ul>
		</div>
	{/if}
</div>

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
		flex-shrink: 1;
		overflow-y: auto;
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

	.nav-item.active {
		background: var(--ui-surface-tertiary);
		font-weight: 600;
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
		min-height: 120px;
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
		background: var(--quest-posted-container);
		color: var(--quest-posted);
	}

	.event-item[data-category='A'] .event-icon {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.event-item[data-category='B'] .event-icon {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.event-item[data-category='G'] .event-icon {
		background: var(--tier-master-container);
		color: var(--tier-master);
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
</style>
