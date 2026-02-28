<script lang="ts">
	/**
	 * Agent Detail Page - Full agent profile with history
	 */

	import { page } from '$app/stores';
	import { worldStore } from '$stores/worldStore.svelte';
	import { TrustTierNames, type AgentID, agentId } from '$types';

	const id = $derived(agentId($page.params.id ?? ''));
	const agent = $derived(worldStore.agents.get(id));

	function xpPercentage(): number {
		if (!agent || agent.xp_to_level === 0) return 100;
		return Math.min((agent.xp / agent.xp_to_level) * 100, 100);
	}
</script>

<svelte:head>
	<title>{agent?.name ?? 'Agent'} - Semdragons</title>
</svelte:head>

<div class="agent-detail-page">
	<header class="page-header">
		<a href="/agents" class="back-link">Back to Agent Roster</a>
	</header>

	{#if agent}
		<div class="agent-content">
			<div class="agent-header">
				<div class="agent-identity">
					<h1>{agent.name}</h1>
					<span class="tier-badge" data-tier={agent.tier}>
						{TrustTierNames[agent.tier]}
					</span>
				</div>
				<div class="agent-status" data-status={agent.status}>
					{agent.status.replace('_', ' ')}
				</div>
			</div>

			<div class="level-card">
				<div class="level-info">
					<span class="level-label">Level</span>
					<span class="level-value">{agent.level}</span>
				</div>
				<div class="xp-info">
					<div class="xp-bar">
						<div class="xp-fill" style="width: {xpPercentage()}%"></div>
					</div>
					<span class="xp-text">{agent.xp} / {agent.xp_to_level} XP to next level</span>
				</div>
			</div>

			<div class="agent-details">
				<section class="detail-card">
					<h2>Skills</h2>
					<div class="skills-grid">
						{#each agent.skills as skill}
							<span class="skill-tag">{skill.replace('_', ' ')}</span>
						{:else}
							<span class="empty-text">No skills</span>
						{/each}
					</div>
				</section>

				<section class="detail-card">
					<h2>Equipment</h2>
					<div class="equipment-list">
						{#each agent.equipment as tool}
							<div class="tool-item">
								<span class="tool-name">{tool.name}</span>
								<span class="tool-category">{tool.category}</span>
							</div>
						{:else}
							<span class="empty-text">No equipment</span>
						{/each}
					</div>
				</section>

				<section class="detail-card">
					<h2>Lifetime Stats</h2>
					<dl class="stats-grid">
						<dt>Quests Completed</dt>
						<dd class="stat-success">{agent.stats.quests_completed}</dd>
						<dt>Quests Failed</dt>
						<dd class="stat-error">{agent.stats.quests_failed}</dd>
						<dt>Bosses Defeated</dt>
						<dd class="stat-success">{agent.stats.bosses_defeated}</dd>
						<dt>Bosses Failed</dt>
						<dd class="stat-error">{agent.stats.bosses_failed}</dd>
						<dt>Total XP Earned</dt>
						<dd>{agent.stats.total_xp_earned}</dd>
						<dt>Avg Quality Score</dt>
						<dd>{(agent.stats.avg_quality_score * 100).toFixed(1)}%</dd>
						<dt>Avg Efficiency</dt>
						<dd>{(agent.stats.avg_efficiency * 100).toFixed(1)}%</dd>
						<dt>Parties Led</dt>
						<dd>{agent.stats.parties_led}</dd>
						<dt>Quests Decomposed</dt>
						<dd>{agent.stats.quests_decomposed}</dd>
					</dl>
				</section>

				<section class="detail-card">
					<h2>Configuration</h2>
					<dl class="config-list">
						<dt>Provider</dt>
						<dd>{agent.config.provider}</dd>
						<dt>Model</dt>
						<dd>{agent.config.model}</dd>
						<dt>Temperature</dt>
						<dd>{agent.config.temperature}</dd>
						<dt>Max Tokens</dt>
						<dd>{agent.config.max_tokens}</dd>
					</dl>
				</section>

				{#if agent.guilds.length > 0}
					<section class="detail-card">
						<h2>Guilds</h2>
						<div class="guilds-list">
							{#each agent.guilds as guildId}
								<a href="/guilds/{guildId}" class="guild-link">{guildId}</a>
							{/each}
						</div>
					</section>
				{/if}

				<section class="detail-card">
					<h2>Lifecycle</h2>
					<dl class="lifecycle-list">
						<dt>Created</dt>
						<dd>{new Date(agent.created_at).toLocaleString()}</dd>
						<dt>Last Updated</dt>
						<dd>{new Date(agent.updated_at).toLocaleString()}</dd>
						<dt>Deaths</dt>
						<dd class="stat-error">{agent.death_count}</dd>
						{#if agent.current_quest}
							<dt>Current Quest</dt>
							<dd><a href="/quests/{agent.current_quest}">{agent.current_quest}</a></dd>
						{/if}
						{#if agent.cooldown_until}
							<dt>Cooldown Until</dt>
							<dd>{new Date(agent.cooldown_until).toLocaleString()}</dd>
						{/if}
					</dl>
				</section>
			</div>
		</div>
	{:else}
		<div class="not-found">
			<h2>Agent not found</h2>
			<p>The agent with ID "{id}" could not be found.</p>
			<a href="/agents">Back to Agent Roster</a>
		</div>
	{/if}
</div>

<style>
	.agent-detail-page {
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

	.agent-content {
		max-width: 900px;
		margin: 0 auto;
	}

	.agent-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-lg);
	}

	.agent-identity {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
	}

	.agent-identity h1 {
		margin: 0;
	}

	.tier-badge {
		padding: var(--spacing-xs) var(--spacing-md);
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
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

	.agent-status {
		padding: var(--spacing-xs) var(--spacing-md);
		border-radius: var(--radius-full);
		font-size: 0.875rem;
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

	.level-card {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-lg);
		display: flex;
		align-items: center;
		gap: var(--spacing-xl);
		margin-bottom: var(--spacing-xl);
	}

	.level-info {
		text-align: center;
	}

	.level-label {
		display: block;
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
	}

	.level-value {
		font-size: 3rem;
		font-weight: 700;
		color: var(--ui-interactive-primary);
		line-height: 1;
	}

	.xp-info {
		flex: 1;
	}

	.xp-bar {
		height: 12px;
		background: var(--xp-bar-background);
		border-radius: 6px;
		overflow: hidden;
		margin-bottom: var(--spacing-xs);
	}

	.xp-fill {
		height: 100%;
		background: var(--xp-bar-fill);
		transition: width 300ms ease;
	}

	.xp-text {
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
	}

	.agent-details {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
		gap: var(--spacing-md);
	}

	.detail-card {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-lg);
	}

	.detail-card h2 {
		font-size: 0.875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: 0 0 var(--spacing-md);
	}

	.skills-grid {
		display: flex;
		flex-wrap: wrap;
		gap: var(--spacing-sm);
	}

	.skill-tag {
		padding: var(--spacing-xs) var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
		font-size: 0.875rem;
		text-transform: capitalize;
	}

	.equipment-list {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.tool-item {
		display: flex;
		justify-content: space-between;
		padding: var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
	}

	.tool-category {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.stats-grid,
	.config-list,
	.lifecycle-list {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
		margin: 0;
	}

	.stats-grid dt,
	.config-list dt,
	.lifecycle-list dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.stats-grid dd,
	.config-list dd,
	.lifecycle-list dd {
		margin: 0;
		text-align: right;
	}

	.stat-success {
		color: var(--status-success);
	}

	.stat-error {
		color: var(--status-error);
	}

	.guilds-list {
		display: flex;
		flex-wrap: wrap;
		gap: var(--spacing-sm);
	}

	.guild-link {
		padding: var(--spacing-xs) var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
	}

	.empty-text {
		color: var(--ui-text-tertiary);
		font-style: italic;
	}

	.not-found {
		text-align: center;
		padding: var(--spacing-xl);
	}

	.not-found h2 {
		margin-bottom: var(--spacing-md);
	}

	.not-found p {
		color: var(--ui-text-secondary);
		margin-bottom: var(--spacing-lg);
	}
</style>
