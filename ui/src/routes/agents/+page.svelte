<script lang="ts">
	/**
	 * Agent Roster - Grid view of all agents
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import { worldStore } from '$stores/worldStore.svelte';
	import { TrustTierNames, type Agent } from '$types';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// Filter state
	let statusFilter = $state<string>('all');

	const filteredAgents = $derived.by(() => {
		if (statusFilter === 'all') return worldStore.agentList;
		return worldStore.agentList.filter((a) => a.status === statusFilter);
	});

	function selectAgent(agent: Agent) {
		worldStore.selectAgent(agent.id);
	}

	function xpPercentage(agent: Agent): number {
		if (agent.xp_to_level === 0) return 100;
		return Math.min((agent.xp / agent.xp_to_level) * 100, 100);
	}
</script>

<svelte:head>
	<title>Agent Roster - Semdragons</title>
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
		<div class="filters-panel">
			<header class="panel-header">
				<h2>Filters</h2>
			</header>
			<div class="filters-content">
				<label for="agent-status-filter" class="filter-label">Status</label>
				<select id="agent-status-filter" bind:value={statusFilter} class="filter-select">
					<option value="all">All</option>
					<option value="idle">Idle</option>
					<option value="on_quest">On Quest</option>
					<option value="in_battle">In Battle</option>
					<option value="cooldown">Cooldown</option>
					<option value="retired">Retired</option>
				</select>
			</div>
		</div>
	{/snippet}

	{#snippet centerPanel()}
		<div class="agent-roster">
			<header class="roster-header">
				<h1>Agent Roster</h1>
				<span class="agent-count">{filteredAgents.length} agents</span>
			</header>

			{#if worldStore.loading}
				<div class="loading-state">
					<div class="loading-grid">
						<div class="skeleton-card"></div>
						<div class="skeleton-card"></div>
						<div class="skeleton-card"></div>
						<div class="skeleton-card"></div>
					</div>
				</div>
			{:else}
			<div class="agent-grid">
				{#each filteredAgents as agent}
					<button
						class="agent-card"
						class:selected={worldStore.selectedAgentId === agent.id}
						aria-label="Select agent: {agent.name}, Level {agent.level} {TrustTierNames[agent.tier]}"
						aria-pressed={worldStore.selectedAgentId === agent.id}
						onclick={() => selectAgent(agent)}
					>
						<div class="agent-header">
							<span class="agent-name">{agent.name}</span>
							<span class="agent-level">Lv.{agent.level}</span>
						</div>

						<div class="tier-badge" data-tier={agent.tier}>
							{TrustTierNames[agent.tier]}
						</div>

						<div class="xp-bar">
							<div class="xp-fill" style="width: {xpPercentage(agent)}%"></div>
						</div>
						<div class="xp-label">
							{agent.xp} / {agent.xp_to_level} XP
						</div>

						<div class="agent-status" data-status={agent.status}>
							{agent.status.replace('_', ' ')}
						</div>

						<div class="agent-skills">
							{#each agent.skills.slice(0, 3) as skill}
								<span class="skill-tag">{skill.replace('_', ' ')}</span>
							{/each}
							{#if agent.skills.length > 3}
								<span class="skill-more">+{agent.skills.length - 3}</span>
							{/if}
						</div>
					</button>
				{:else}
					<div class="empty-state">No agents found</div>
				{/each}
			</div>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel">
			<header class="panel-header">
				<h2>Agent Details</h2>
			</header>
			<div class="details-content">
				{#if worldStore.selectedAgent}
					{@const agent = worldStore.selectedAgent}
					<section class="detail-section">
						<div class="agent-profile">
							<h3>{agent.name}</h3>
							<span class="tier-badge large" data-tier={agent.tier}>
								{TrustTierNames[agent.tier]}
							</span>
						</div>

						<div class="level-display">
							<span class="level-number">Level {agent.level}</span>
							<div class="xp-bar large">
								<div class="xp-fill" style="width: {xpPercentage(agent)}%"></div>
							</div>
							<span class="xp-text">{agent.xp} / {agent.xp_to_level} XP</span>
						</div>

						<dl class="detail-list">
							<dt>Status</dt>
							<dd>
								<span class="status-badge" data-status={agent.status}>
									{agent.status.replace('_', ' ')}
								</span>
							</dd>
							<dt>Deaths</dt>
							<dd>{agent.death_count}</dd>
							{#if agent.current_quest}
								<dt>Current Quest</dt>
								<dd><a href="/quests/{agent.current_quest}">{agent.current_quest}</a></dd>
							{/if}
							{#if agent.cooldown_until}
								<dt>Cooldown Until</dt>
								<dd>{new Date(agent.cooldown_until).toLocaleString()}</dd>
							{/if}
						</dl>

						<h4>Skills</h4>
						<div class="skills-list">
							{#each agent.skills as skill}
								<span class="skill-tag">{skill.replace('_', ' ')}</span>
							{/each}
						</div>

						<h4>Stats</h4>
						<dl class="stats-list">
							<dt>Quests Completed</dt>
							<dd>{agent.stats.quests_completed}</dd>
							<dt>Quests Failed</dt>
							<dd>{agent.stats.quests_failed}</dd>
							<dt>Bosses Defeated</dt>
							<dd>{agent.stats.bosses_defeated}</dd>
							<dt>Total XP Earned</dt>
							<dd>{agent.stats.total_xp_earned}</dd>
							<dt>Avg Quality</dt>
							<dd>{(agent.stats.avg_quality_score * 100).toFixed(1)}%</dd>
						</dl>

						<a href="/agents/{agent.id}" class="view-full-link">View full profile</a>
					</section>
				{:else}
					<p class="empty-state">Select an agent to view details</p>
				{/if}
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	/* Filters Panel */
	.filters-panel {
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

	.filters-content {
		padding: var(--spacing-md);
	}

	.filter-label {
		display: block;
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		margin-bottom: var(--spacing-xs);
	}

	.filter-select {
		width: 100%;
	}

	/* Agent Roster */
	.agent-roster {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.roster-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.roster-header h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.agent-count {
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}

	/* Agent Grid */
	.agent-grid {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: var(--spacing-md);
		align-content: start;
	}

	.agent-card {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-md);
		text-align: left;
		cursor: pointer;
		transition:
			border-color 150ms ease,
			box-shadow 150ms ease;
	}

	.agent-card:hover {
		border-color: var(--ui-border-interactive);
	}

	.agent-card.selected {
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 1px var(--ui-interactive-primary);
	}

	.agent-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-sm);
	}

	.agent-name {
		font-weight: 600;
		font-size: 0.875rem;
	}

	.agent-level {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-tertiary);
		padding: 2px 6px;
		border-radius: var(--radius-sm);
	}

	.tier-badge {
		display: inline-block;
		font-size: 0.625rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		text-transform: uppercase;
		font-weight: 600;
		margin-bottom: var(--spacing-sm);
	}

	.tier-badge[data-tier='0'] {
		background: var(--tier-apprentice-container);
		color: var(--tier-apprentice);
	}
	.tier-badge[data-tier='1'] {
		background: var(--tier-journeyman-container);
		color: var(--tier-journeyman);
	}
	.tier-badge[data-tier='2'] {
		background: var(--tier-expert-container);
		color: var(--tier-expert);
	}
	.tier-badge[data-tier='3'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}
	.tier-badge[data-tier='4'] {
		background: var(--tier-grandmaster-container);
		color: var(--tier-grandmaster);
	}

	.tier-badge.large {
		font-size: 0.75rem;
		padding: 4px 12px;
	}

	/* XP Bar */
	.xp-bar {
		height: 4px;
		background: var(--xp-bar-background);
		border-radius: 2px;
		overflow: hidden;
		margin-bottom: 2px;
	}

	.xp-bar.large {
		height: 8px;
		border-radius: 4px;
	}

	.xp-fill {
		height: 100%;
		background: var(--xp-bar-fill);
		transition: width 300ms ease;
	}

	.xp-label,
	.xp-text {
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
		margin-bottom: var(--spacing-sm);
	}

	.agent-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		display: inline-block;
		margin-bottom: var(--spacing-sm);
		text-transform: capitalize;
	}

	.agent-status[data-status='idle'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}
	.agent-status[data-status='on_quest'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}
	.agent-status[data-status='in_battle'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}
	.agent-status[data-status='cooldown'] {
		background: var(--status-error-container);
		color: var(--status-error);
	}
	.agent-status[data-status='retired'] {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
	}

	.agent-skills,
	.skills-list {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}

	.skill-tag {
		font-size: 0.625rem;
		padding: 2px 6px;
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
		color: var(--ui-text-secondary);
		text-transform: capitalize;
	}

	.skill-more {
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
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

	.agent-profile {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-md);
	}

	.agent-profile h3 {
		margin: 0;
		font-size: 1.125rem;
	}

	.level-display {
		text-align: center;
		padding: var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-lg);
		margin-bottom: var(--spacing-md);
	}

	.level-number {
		font-size: 1.5rem;
		font-weight: 700;
		color: var(--ui-interactive-primary);
	}

	.detail-list,
	.stats-list {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
		margin-bottom: var(--spacing-md);
	}

	.detail-list dt,
	.stats-list dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.detail-list dd,
	.stats-list dd {
		margin: 0;
	}

	.status-badge {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		text-transform: capitalize;
	}

	h4 {
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: var(--spacing-md) 0 var(--spacing-sm);
	}

	.view-full-link {
		display: block;
		text-align: center;
		padding: var(--spacing-sm);
		margin-top: var(--spacing-md);
	}

	.empty-state {
		text-align: center;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-xl);
	}

	/* Loading State */
	.loading-state {
		flex: 1;
		padding: var(--spacing-md);
	}

	.loading-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: var(--spacing-md);
	}

	.skeleton-card {
		height: 180px;
		background: linear-gradient(
			90deg,
			var(--ui-surface-secondary) 0%,
			var(--ui-surface-tertiary) 50%,
			var(--ui-surface-secondary) 100%
		);
		background-size: 200% 100%;
		border-radius: var(--radius-lg);
		animation: shimmer 1.5s infinite;
	}

	@keyframes shimmer {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}
</style>
