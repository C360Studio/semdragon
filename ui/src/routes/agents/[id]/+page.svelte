<script lang="ts">
	/**
	 * Agent Detail Page - Full agent profile with history
	 */

	import { page } from '$app/state';
	import { worldStore } from '$stores/worldStore.svelte';
	import { pageContext } from '$lib/stores/pageContext.svelte';
	import { TrustTierNames, agentId, toolsForTier, ConsumableTypeNames } from '$types';
	import { resolveCapability, type ModelResolveResponse } from '$lib/services/api';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import ActivityFeed from '$components/ActivityFeed.svelte';
	import TabBar from '$components/TabBar.svelte';
	import AgentQuestHistory from '$components/AgentQuestHistory.svelte';
	import AgentPartyHistory from '$components/AgentPartyHistory.svelte';
	import AgentBattleHistory from '$components/AgentBattleHistory.svelte';
	import AgentCollaborators from '$components/AgentCollaborators.svelte';

	const id = $derived(agentId(page.params.id ?? ''));
	const agent = $derived(
		worldStore.agents.get(id) ??
			worldStore.agentList.find((a) => String(a.id).endsWith('.' + page.params.id)) ??
			undefined
	);

	$effect(() => {
		if (agent) {
			pageContext.set([{ type: 'agent', id: agent.id, label: agent.name }]);
		}
		return () => pageContext.clear();
	});

	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	const tools = $derived(agent ? toolsForTier(agent.tier) : []);

	let modelAssignment = $state<ModelResolveResponse | null>(null);

	$effect(() => {
		if (!agent) {
			modelAssignment = null;
			return;
		}
		let cancelled = false;
		const tierName = TrustTierNames[agent.tier].toLowerCase();
		resolveCapability(`agent-work.${tierName}`)
			.then((res) => {
				if (cancelled) return;
				if (res.endpoint_name) {
					modelAssignment = res;
				} else {
					return resolveCapability('agent-work').then((fallback) => {
						if (!cancelled) modelAssignment = fallback;
					});
				}
			})
			.catch(() => {
				if (cancelled) return;
				resolveCapability('agent-work')
					.then((res) => { if (!cancelled) modelAssignment = res; })
					.catch(() => { if (!cancelled) modelAssignment = null; });
			});
		return () => { cancelled = true; };
	});

	const xpPercentage = $derived(
		!agent || agent.xp_to_level === 0 ? 100 : Math.min((agent.xp / agent.xp_to_level) * 100, 100)
	);

	// Inventory: reactive from worldStore (populated by SSE or store page)
	const inventory = $derived(agent ? worldStore.getInventory(agent.id) ?? null : null);
	const ownedTools = $derived(inventory ? Object.values(inventory.owned_tools) : []);
	const consumableList = $derived(
		inventory ? Object.entries(inventory.consumables).filter(([, qty]) => qty > 0) : []
	);
	const bagIsEmpty = $derived(ownedTools.length === 0 && consumableList.length === 0);

	let activeHistoryTab = $state('quests');

	const historyTabs = $derived.by(() => {
		if (!agent) return [];
		const agentIdStr = String(agent.id);

		const questCount = worldStore.questList.filter((q) => {
			if (!q.claimed_by) return false;
			const cb = String(q.claimed_by);
			return cb === agentIdStr || cb.includes(agentIdStr) || agentIdStr.includes(cb);
		}).length;

		const partyCount = worldStore.partyList.filter((p) =>
			p.members.some((m) => {
				const mid = String(m.agent_id);
				return mid === agentIdStr || mid.includes(agentIdStr) || agentIdStr.includes(mid);
			})
		).length;

		const battleCount = worldStore.battleList.filter((b) => {
			const bid = String(b.agent_id);
			return bid === agentIdStr || bid.includes(agentIdStr) || agentIdStr.includes(bid);
		}).length;

		return [
			{ id: 'quests', label: 'Quests', count: questCount },
			{ id: 'parties', label: 'Parties', count: partyCount },
			{ id: 'battles', label: 'Boss Battles', count: battleCount },
			{ id: 'collaborators', label: 'Collaborators' }
		];
	});
</script>

<svelte:head>
	<title>{agent?.name ?? 'Agent'} - Semdragons</title>
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
		<div class="agent-detail-page" data-testid="agent-detail-page">
			<header class="page-header">
				<a href="/agents" class="back-link">Back to Agent Roster</a>
			</header>

			{#if agent}
				<div class="agent-header">
					<div class="agent-identity">
						<h1 data-testid="agent-name">{agent.name}</h1>
						<span class="tier-badge" data-tier={agent.tier}>
							{TrustTierNames[agent.tier]}
						</span>
					</div>
					<div class="agent-status" data-status={agent.status}>
						{agent.status.replaceAll('_', ' ')}
					</div>
				</div>

				<div class="level-card" data-testid="agent-level">
					<div class="level-info">
						<span class="level-label">Level</span>
						<span class="level-value">{agent.level}</span>
					</div>
					<div class="xp-info">
						<div class="xp-bar">
							<div class="xp-fill" style="width: {xpPercentage}%"></div>
						</div>
						<span class="xp-text">{agent.xp} / {agent.xp_to_level} XP to next level</span>
					</div>
				</div>

				<div class="agent-details">
					<section class="detail-card">
						<h2>Skills</h2>
						<div class="skills-grid">
							{#each Object.keys(agent.skill_proficiencies || {}) as skill}
								<span class="skill-tag">{skill.replaceAll('_', ' ')}</span>
							{:else}
								<span class="empty-text">No skills</span>
							{/each}
						</div>
					</section>

					<section class="detail-card">
						<h2>Tools</h2>
						<div class="tools-list">
							{#each tools as tool}
								<div class="tool-item">
									<div class="tool-header">
										<span class="tool-name">{tool.name.replaceAll('_', ' ')}</span>
										<span class="tool-category">{tool.category}</span>
									</div>
									<span class="tool-description">{tool.description}</span>
								</div>
							{:else}
								<span class="empty-text">No tools available</span>
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
						<h2>Model Assignment</h2>
						{#if modelAssignment}
							<dl class="config-list">
								<dt>Capability</dt>
								<dd class="mono">{modelAssignment.capability}</dd>
								<dt>Endpoint</dt>
								<dd class="mono">{modelAssignment.endpoint_name}</dd>
								{#if modelAssignment.provider}
									<dt>Provider</dt>
									<dd>{modelAssignment.provider}</dd>
								{/if}
								{#if modelAssignment.model}
									<dt>Model</dt>
									<dd class="mono">{modelAssignment.model}</dd>
								{/if}
								{#if modelAssignment.fallback_chain && modelAssignment.fallback_chain.length > 1}
									<dt>Fallbacks</dt>
									<dd class="mono">{modelAssignment.fallback_chain.slice(1).join(' → ')}</dd>
								{/if}
							</dl>
						{:else}
							<span class="empty-text">Loading model assignment...</span>
						{/if}
					</section>

					{#if agent.guild_id}
						{@const guild = worldStore.guilds.get(agent.guild_id)}
						<section class="detail-card">
							<h2>Guild</h2>
							<a href="/guilds/{agent.guild_id}" class="guild-link">
								{guild?.name ?? String(agent.guild_id).split('.').pop()}
							</a>
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

					<section class="detail-card bag-of-holding" data-testid="bag-of-holding">
						<h2>Bag of Holding</h2>
						{#if !inventory || bagIsEmpty}
							<div class="bag-empty">
								<span class="empty-text">No items yet</span>
								<a href="/store?agent={page.params.id}" class="store-link">Visit the Store</a>
							</div>
						{:else}
							{#if ownedTools.length > 0}
								<div class="bag-section">
									<span class="bag-section-label">Tools</span>
									<div class="bag-items">
										{#each ownedTools as tool (tool.item_id)}
											<div class="bag-item">
												<span class="bag-item-name">{tool.item_name}</span>
												{#if tool.purchase_type === 'rental' && tool.uses_remaining !== undefined}
													<span class="bag-badge bag-badge--rental">{tool.uses_remaining} uses</span>
												{:else}
													<span class="bag-badge bag-badge--owned">Permanent</span>
												{/if}
											</div>
										{/each}
									</div>
								</div>
							{/if}

							{#if consumableList.length > 0}
								<div class="bag-section">
									<span class="bag-section-label">Consumables</span>
									<div class="bag-items">
										{#each consumableList as [consumableId, qty]}
											<div class="bag-item">
												<span class="bag-item-name">
													{ConsumableTypeNames[consumableId as keyof typeof ConsumableTypeNames] ?? consumableId.replaceAll('_', ' ')}
												</span>
												<span class="bag-badge bag-badge--qty">x{qty}</span>
											</div>
										{/each}
									</div>
								</div>
							{/if}

							<div class="bag-footer">
								<span class="bag-total">Total spent: {inventory.total_spent.toLocaleString()} XP</span>
								<a href="/store?agent={page.params.id}" class="store-link">Store</a>
							</div>
						{/if}
					</section>
				</div>

				<section class="history-section">
					<h2 class="section-heading">History</h2>
					<TabBar tabs={historyTabs} bind:activeTab={activeHistoryTab} />
					<div role="tabpanel" id="tab-panel-{activeHistoryTab}">
						{#if activeHistoryTab === 'quests'}
							<AgentQuestHistory agentId={String(agent.id)} />
						{:else if activeHistoryTab === 'parties'}
							<AgentPartyHistory agentId={String(agent.id)} />
						{:else if activeHistoryTab === 'battles'}
							<AgentBattleHistory agentId={String(agent.id)} />
						{:else if activeHistoryTab === 'collaborators'}
							<AgentCollaborators agentId={String(agent.id)} />
						{/if}
					</div>
				</section>

				<ActivityFeed agentId={agent.id} />
			{:else}
				<div class="not-found" data-testid="agent-not-found">
					<h2>Agent not found</h2>
					<p>The agent with ID "{id}" could not be found.</p>
					<a href="/agents">Back to Agent Roster</a>
				</div>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel">
			<header class="panel-header">
				<h2>Collaborators</h2>
			</header>
			<div class="details-content">
				{#if agent}
					<AgentCollaborators agentId={String(agent.id)} compact={true} />
				{:else}
					<p class="empty-state">Agent context</p>
				{/if}
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

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

	.tools-list {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.tool-item {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
	}

	.tool-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.tool-name {
		font-weight: 500;
		text-transform: capitalize;
	}

	.tool-category {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.tool-description {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
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

	.guild-link {
		padding: var(--spacing-xs) var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
	}

	.empty-text {
		color: var(--ui-text-tertiary);
		font-style: italic;
	}

	.mono {
		font-family: monospace;
		font-size: 0.8125rem;
	}

	/* History section */
	.history-section {
		margin-top: var(--spacing-xl);
		margin-bottom: var(--spacing-xl);
	}

	.section-heading {
		font-size: 0.875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: 0 0 var(--spacing-sm);
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

	/* Bag of Holding */
	.bag-of-holding {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.bag-empty {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
	}

	.bag-section {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
	}

	.bag-section-label {
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
	}

	.bag-items {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.bag-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--spacing-xs) var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
	}

	.bag-item-name {
		font-size: 0.8125rem;
		text-transform: capitalize;
	}

	.bag-badge {
		font-size: 0.625rem;
		font-weight: 600;
		text-transform: uppercase;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
	}

	.bag-badge--owned {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.bag-badge--rental {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.bag-badge--qty {
		background: var(--ui-surface-primary);
		color: var(--ui-text-secondary);
		border: 1px solid var(--ui-border-subtle);
	}

	.bag-footer {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding-top: var(--spacing-sm);
		border-top: 1px solid var(--ui-border-subtle);
		margin-top: auto;
	}

	.bag-total {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.store-link {
		font-size: 0.75rem;
		color: var(--ui-interactive-primary);
	}
</style>
