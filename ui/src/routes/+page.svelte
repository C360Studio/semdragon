<script lang="ts">
	/**
	 * DM Command Center - Main Dashboard
	 *
	 * DM Dashboard - overview of the entire game world.
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import { worldStore } from '$stores/worldStore.svelte';
	import { formatTokenCount, formatCostUSD } from '$lib/utils/format';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

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
		<ExplorerNav />
	{/snippet}

	{#snippet rightPanel()}{/snippet}

	{#snippet centerPanel()}
		<div class="dashboard">
			<header class="dashboard-header">
				<h1>DM Dashboard</h1>
			</header>

			{#if worldStore.loading}
				<div class="loading">Loading world state...</div>
			{:else if !worldStore.connected}
				<div class="reconnecting-banner" role="status">Reconnecting...</div>
			{/if}

			{#if worldStore.error}
				<div class="error">{worldStore.error}</div>
			{:else if !worldStore.loading}
				<!-- Stats Grid -->
				<div class="stats-grid" data-testid="dashboard-stats">
					<div class="stat-card" data-testid="stat-active-agents">
						<span class="stat-value">{worldStore.stats.active_agents}</span>
						<span class="stat-label">Active Agents</span>
					</div>
					<div class="stat-card" data-testid="stat-idle-agents">
						<span class="stat-value">{worldStore.stats.idle_agents}</span>
						<span class="stat-label">Idle Agents</span>
					</div>
					<div class="stat-card" data-testid="stat-open-quests">
						<span class="stat-value">{worldStore.stats.open_quests}</span>
						<span class="stat-label">Open Quests</span>
					</div>
					<div class="stat-card" data-testid="stat-active-quests">
						<span class="stat-value">{worldStore.stats.active_quests}</span>
						<span class="stat-label">Active Quests</span>
					</div>
					<div class="stat-card" data-testid="stat-completion-rate">
						<span class="stat-value">{(worldStore.stats.completion_rate * 100).toFixed(1)}%</span>
						<span class="stat-label">Completion Rate</span>
					</div>
					<div class="stat-card" data-testid="stat-avg-quality">
						<span class="stat-value">{(worldStore.stats.avg_quality * 100).toFixed(1)}%</span>
						<span class="stat-label">Avg Quality</span>
					</div>
					<div class="stat-card" data-testid="stat-total-xp-earned">
						<span class="stat-value">{formatNumber(worldStore.totalXpEarned)}</span>
						<span class="stat-label">Total XP Earned</span>
					</div>
					<div class="stat-card" data-testid="stat-active-parties">
						<span class="stat-value">{worldStore.partyList.length}</span>
						<span class="stat-label">Active Parties</span>
					</div>
					<div class="stat-card" data-testid="stat-guilds">
						<span class="stat-value">{worldStore.guildList.length}</span>
						<span class="stat-label">Guilds</span>
					</div>
					<div class="stat-card" data-testid="stat-battle-win-rate">
						<span class="stat-value">{worldStore.battleStats.winRate.toFixed(0)}%</span>
						<span class="stat-label">Battle Win Rate</span>
					</div>
					<div
						class="stat-card"
						class:stat-warning={worldStore.stats.token_breaker === 'warning'}
						class:stat-error={worldStore.stats.token_breaker === 'tripped'}
						data-testid="stat-tokens-hour"
					>
						<span class="stat-value">{formatTokenCount(worldStore.stats.tokens_used_hourly ?? 0)}</span>
						<span class="stat-label">Tokens / Hour</span>
					</div>
					<div
						class="stat-card"
						class:stat-warning={worldStore.stats.token_breaker === 'warning'}
						class:stat-error={worldStore.stats.token_breaker === 'tripped'}
						data-testid="stat-token-budget"
					>
						<span class="stat-value">{((worldStore.stats.token_budget_pct ?? 0) * 100).toFixed(0)}%</span>
						<span class="stat-label">Token Budget</span>
					</div>
					<div
						class="stat-card"
						class:stat-warning={worldStore.stats.token_breaker === 'warning'}
						class:stat-error={worldStore.stats.token_breaker === 'tripped'}
						data-testid="stat-hourly-cost"
					>
						<span class="stat-value">{formatCostUSD(worldStore.stats.cost_used_hourly_usd ?? 0)}</span>
						<span class="stat-label">Hourly Cost</span>
					</div>
				</div>

				<!-- Agent Distribution -->
				<section class="dashboard-section" aria-labelledby="tier-heading">
					<h2 id="tier-heading">Agent Distribution by Tier</h2>
					<div class="tier-bars">
						{#each worldStore.tierDistribution as tier}
							<div class="tier-row" data-testid="tier-{tier.name.toLowerCase()}">
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
				<section class="dashboard-section" aria-labelledby="quests-heading">
					<h2 id="quests-heading">Active Quests</h2>
					<div class="quest-summary">
						{#each worldStore.activeQuests.slice(0, 5) as quest}
							<a href="/quests/{quest.id}" class="quest-row">
								<span class="quest-title">{quest.title}</span>
								<span class="quest-status" data-status={quest.status}>{quest.status}</span>
							</a>
						{:else}
							<p class="empty-state" role="status">No active quests</p>
						{/each}
					</div>
				</section>

				<!-- Active Battles -->
				<section class="dashboard-section" aria-labelledby="battles-heading">
					<h2 id="battles-heading">Active Battles</h2>
					<div class="battle-summary">
						{#each worldStore.activeBattles.slice(0, 5) as battle}
							<a href="/battles/{battle.id}" class="battle-row">
								<span class="battle-id">Battle #{battle.id.slice(-6)}</span>
								<span class="battle-status" data-status={battle.status}>{battle.status}</span>
							</a>
						{:else}
							<p class="empty-state" role="status">No active battles</p>
						{/each}
					</div>
				</section>

				<!-- Guild Activity -->
				<section class="dashboard-section" aria-labelledby="guilds-heading">
					<h2 id="guilds-heading">Guild Activity</h2>
					<div class="guild-summary">
						{#each worldStore.guildList.slice(0, 5) as guild}
							<a href="/guilds/{guild.id}" class="guild-row">
								<span class="guild-name">{guild.name}</span>
								<span class="guild-members">{guild.members.length} members</span>
								<span class="guild-reputation">{(guild.reputation * 100).toFixed(0)}%</span>
							</a>
						{:else}
							<p class="empty-state" role="status">No guilds formed</p>
						{/each}
					</div>
				</section>

				<!-- Active Parties -->
				<section class="dashboard-section" aria-labelledby="parties-heading">
					<h2 id="parties-heading">Active Parties</h2>
					<div class="party-summary">
						{#each worldStore.partyList.filter((p) => p.status === 'active') as party}
							<a href="/parties/{party.id}" class="party-row">
								<span class="party-name">{party.name || `Party #${party.id.slice(-6)}`}</span>
								<span class="party-quest">Quest: {party.quest_id ? party.quest_id.slice(-6) : 'TBD'}</span>
								<span class="party-size">{party.members.length} agents</span>
							</a>
						{:else}
							<p class="empty-state" role="status">No active parties</p>
						{/each}
					</div>
				</section>
			{/if}
		</div>
	{/snippet}

</ThreePanelLayout>

<style>
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

	.stat-card.stat-warning .stat-value {
		color: var(--status-warning);
	}

	.stat-card.stat-error .stat-value {
		color: var(--status-error);
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
	.guild-row:hover,
	.party-row:hover {
		border-color: var(--ui-border-interactive);
		text-decoration: none;
	}

	.quest-row:focus-visible,
	.battle-row:focus-visible,
	.guild-row:focus-visible,
	.party-row:focus-visible {
		outline: 2px solid var(--ui-interactive-primary);
		outline-offset: 2px;
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

	.reconnecting-banner {
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--status-warning-container);
		color: var(--status-warning);
		border-radius: var(--radius-md);
		text-align: center;
		font-size: 0.8125rem;
		font-weight: 500;
		margin-bottom: var(--spacing-md);
		animation: pulse-opacity 2s ease-in-out infinite;
	}

	@keyframes pulse-opacity {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.5; }
	}
</style>
