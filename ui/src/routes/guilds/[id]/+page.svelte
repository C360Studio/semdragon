<script lang="ts">
	/**
	 * Guild Detail Page - Full guild profile with members and stats
	 */

	import { page } from '$app/state';
	import { worldStore } from '$stores/worldStore.svelte';
	import { pageContext } from '$lib/stores/pageContext.svelte';
	import { guildId } from '$types';
	import type { AgentID } from '$types';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';

	const id = $derived(guildId(page.params.id ?? ''));
	const guild = $derived(worldStore.guilds.get(id));

	const guildmaster = $derived(guild?.members.find((m) => m.rank === 'guildmaster'));
	const officers = $derived(guild?.members.filter((m) => m.rank === 'officer') ?? []);
	const otherMembers = $derived(
		guild?.members.filter((m) => m.rank !== 'guildmaster' && m.rank !== 'officer') ?? []
	);

	function agentName(agentId: AgentID): string {
		const agent = worldStore.agents.get(agentId);
		return agent?.name ?? String(agentId);
	}

	$effect(() => {
		if (guild) {
			pageContext.set([{ type: 'guild', id: guild.id, label: guild.name }]);
		}
		return () => pageContext.clear();
	});

	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);
</script>

<svelte:head>
	<title>{guild?.name ?? 'Guild'} - Semdragons</title>
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
		<div class="guild-detail-page" data-testid="guild-detail-page">
			<header class="page-header">
				<a href="/guilds" class="back-link">Back to Guild Registry</a>
			</header>

			{#if guild}
				<div class="guild-header">
					<div class="guild-identity">
						<h1 data-testid="guild-name">{guild.name}</h1>
						<span class="guild-status" data-status={guild.status}>{guild.status}</span>
					</div>
					{#if guild.motto}
						<p class="guild-motto">"{guild.motto}"</p>
					{/if}
				</div>

				<p class="guild-description">{guild.description}</p>

				<div class="stats-bar">
					<div class="stat">
						<span class="stat-value">{guild.members.length}</span>
						<span class="stat-label">Members</span>
					</div>
					<div class="stat">
						<span class="stat-value">{(guild.reputation * 100).toFixed(0)}%</span>
						<span class="stat-label">Reputation</span>
					</div>
					<div class="stat">
						<span class="stat-value">{guild.quests_handled}</span>
						<span class="stat-label">Quests</span>
					</div>
					<div class="stat">
						<span class="stat-value">{(guild.success_rate * 100).toFixed(0)}%</span>
						<span class="stat-label">Success</span>
					</div>
					<div class="stat">
						<span class="stat-value">{guild.quests_failed}</span>
						<span class="stat-label">Failed</span>
					</div>
				</div>

				<div class="guild-details">
					{#if guildmaster}
						<section class="detail-card guildmaster-card">
							<h2>Guildmaster</h2>
							<a href="/agents/{guildmaster.agent_id}" class="member-highlight"
								aria-label="{agentName(guildmaster.agent_id)}, guildmaster">
								<span class="member-name">{agentName(guildmaster.agent_id)}</span>
								<span class="member-contribution">Contribution: {guildmaster.contribution}</span>
								<span class="member-joined">Since {new Date(guildmaster.joined_at).toLocaleDateString()}</span>
							</a>
						</section>
					{/if}

					<section class="detail-card">
						<h2>Members ({guild.members.length} / {guild.max_members})</h2>
						<div class="members-list">
							{#if officers.length > 0}
								<div class="rank-group">
									<h3>Officers</h3>
									{#each officers as member}
										<a href="/agents/{member.agent_id}" class="member-row"
											aria-label="{agentName(member.agent_id)}, {member.rank}, contribution {member.contribution}">
											<span class="member-name">{agentName(member.agent_id)}</span>
											<span class="member-rank">{member.rank}</span>
											<span class="member-contribution">{member.contribution}</span>
										</a>
									{/each}
								</div>
							{/if}
							{#if otherMembers.length > 0}
								<div class="rank-group">
									<h3>Roster</h3>
									{#each otherMembers as member}
										<a href="/agents/{member.agent_id}" class="member-row"
											aria-label="{agentName(member.agent_id)}, {member.rank}, contribution {member.contribution}">
											<span class="member-name">{agentName(member.agent_id)}</span>
											<span class="member-rank">{member.rank}</span>
											<span class="member-contribution">{member.contribution}</span>
										</a>
									{/each}
								</div>
							{/if}
						</div>
					</section>

					<section class="detail-card">
						<h2>Info</h2>
						<dl class="info-list">
							<dt>Culture</dt>
							<dd>{guild.culture}</dd>
							<dt>Min Level</dt>
							<dd>{guild.min_level}</dd>
							<dt>Founded</dt>
							<dd>{new Date(guild.founded).toLocaleDateString()}</dd>
							<dt>Founded By</dt>
							<dd>
								<a href="/agents/{guild.founded_by}">{agentName(guild.founded_by)}</a>
							</dd>
						</dl>
					</section>

					{#if (guild.quest_types ?? []).length > 0}
						<section class="detail-card">
							<h2>Quest Types</h2>
							<div class="tag-list">
								{#each guild.quest_types ?? [] as qtype}
									<span class="tag">{qtype.replaceAll('_', ' ')}</span>
								{/each}
							</div>
						</section>
					{/if}

					{#if guild.shared_tools.length > 0}
						<section class="detail-card">
							<h2>Shared Tools</h2>
							<div class="tag-list">
								{#each guild.shared_tools as tool}
									<span class="tag">{tool}</span>
								{/each}
							</div>
						</section>
					{/if}
				</div>
			{:else}
				<div class="not-found" data-testid="guild-not-found">
					<h2>Guild not found</h2>
					<p>The guild with ID "{page.params.id}" could not be found.</p>
					<a href="/guilds">Back to Guild Registry</a>
				</div>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel">
			<header class="panel-header">
				<h2>Related</h2>
			</header>
			<div class="details-content">
				<p class="empty-state">Guild context</p>
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	.guild-detail-page {
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

	.guild-header {
		margin-bottom: var(--spacing-md);
	}

	.guild-identity {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
	}

	.guild-identity h1 {
		margin: 0;
	}

	.guild-status {
		padding: var(--spacing-xs) var(--spacing-md);
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
	}

	.guild-status[data-status='active'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.guild-status[data-status='inactive'] {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
	}

	.guild-motto {
		color: var(--ui-text-secondary);
		font-style: italic;
		margin: var(--spacing-sm) 0 0;
	}

	.guild-description {
		color: var(--ui-text-secondary);
		margin-bottom: var(--spacing-lg);
	}

	.stats-bar {
		display: flex;
		gap: var(--spacing-lg);
		padding: var(--spacing-md) var(--spacing-lg);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		margin-bottom: var(--spacing-xl);
	}

	.stat {
		text-align: center;
		flex: 1;
	}

	.stat-value {
		display: block;
		font-size: 1.5rem;
		font-weight: 700;
		color: var(--ui-interactive-primary);
		line-height: 1;
	}

	.stat-label {
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
	}

	.guild-details {
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

	.guildmaster-card {
		border-left: 4px solid var(--tier-master);
	}

	.member-highlight {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
		padding: var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		color: var(--ui-text-primary);
		text-decoration: none;
	}

	.member-highlight:hover {
		text-decoration: none;
		border-color: var(--ui-border-interactive);
	}

	.member-highlight:focus-visible,
	.member-row:focus-visible {
		outline: 2px solid var(--ui-interactive-primary);
		outline-offset: 2px;
	}

	.member-highlight .member-name {
		font-size: 1.125rem;
		font-weight: 600;
	}

	.member-highlight .member-contribution,
	.member-highlight .member-joined {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.members-list {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.rank-group h3 {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		margin: 0 0 var(--spacing-sm);
	}

	.member-row {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		color: var(--ui-text-primary);
		text-decoration: none;
	}

	.member-row:hover {
		text-decoration: none;
		background: var(--ui-surface-primary);
	}

	.member-row .member-name {
		flex: 1;
		font-weight: 500;
	}

	.member-row .member-rank {
		font-size: 0.75rem;
		text-transform: capitalize;
		color: var(--ui-text-secondary);
	}

	.member-row .member-contribution {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.info-list {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
		margin: 0;
	}

	.info-list dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.info-list dd {
		margin: 0;
		text-align: right;
	}

	.info-list dd a {
		color: var(--ui-interactive-primary);
	}

	.tag-list {
		display: flex;
		flex-wrap: wrap;
		gap: var(--spacing-xs);
	}

	.tag {
		padding: var(--spacing-xs) var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
		font-size: 0.875rem;
		text-transform: capitalize;
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

	/* Right panel */
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
		padding: var(--spacing-md);
	}

	.empty-state {
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}
</style>
