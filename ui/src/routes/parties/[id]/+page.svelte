<script lang="ts">
	/**
	 * Party Detail Page - Full party profile with members and quest assignment
	 */

	import { page } from '$app/stores';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import { worldStore } from '$stores/worldStore.svelte';
	import { pageContext } from '$lib/stores/pageContext.svelte';
	import { TrustTierNames, type AgentID, type TrustTier, partyId } from '$types';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	const id = $derived(partyId($page.params.id ?? ''));
	const party = $derived(worldStore.parties.get(id));

	$effect(() => {
		if (party) {
			pageContext.set([{
				type: 'party',
				id: party.id,
				label: party.name || `Party #${party.id.slice(-6)}`
			}]);
		}
		return () => pageContext.clear();
	});

	const quest = $derived(party?.quest_id ? worldStore.quests.get(party.quest_id) : null);
	const leadAgent = $derived(party?.lead ? worldStore.agents.get(party.lead) : null);

	function resolveAgent(agentId: AgentID) {
		return worldStore.agents.get(agentId);
	}
</script>

<svelte:head>
	<title>{party?.name ?? 'Party'} - Semdragons</title>
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
		<div class="party-detail">
			<header class="page-header">
				<a href="/parties" class="back-link">Back to Parties</a>
			</header>

			{#if party}
				<div class="party-content">
					<!-- Party Header -->
					<div class="party-header">
						<div class="party-identity">
							<h1>{party.name || `Party #${party.id.slice(-6)}`}</h1>
							<span class="status-badge" data-status={party.status}>{party.status}</span>
							<span class="strategy-badge">{party.strategy}</span>
						</div>
					</div>

					<div class="party-details">
						<!-- Quest Assignment -->
						{#if quest}
							<section class="detail-card">
								<h2>Quest Assignment</h2>
								<a href="/quests/{quest.id}" class="quest-link">
									<span class="quest-title">{quest.title}</span>
									<span class="quest-status" data-status={quest.status}>{quest.status}</span>
								</a>
							</section>
						{:else if party.quest_id}
							<section class="detail-card">
								<h2>Quest Assignment</h2>
								<p class="empty-text">{party.quest_id}</p>
							</section>
						{/if}

						<!-- Lead Agent -->
						{#if leadAgent}
							<section class="detail-card">
								<h2>Lead Agent</h2>
								<a href="/agents/{leadAgent.id}" class="agent-link">
									<span class="agent-name">{leadAgent.name}</span>
									<span class="agent-level">Lv.{leadAgent.level}</span>
									<span class="tier-badge" data-tier={leadAgent.tier}>
										{TrustTierNames[leadAgent.tier as TrustTier]}
									</span>
								</a>
							</section>
						{/if}

						<!-- Lifecycle -->
						<section class="detail-card">
							<h2>Lifecycle</h2>
							<dl class="lifecycle-list">
								<dt>Formed</dt>
								<dd>{new Date(party.formed_at).toLocaleString()}</dd>
								{#if party.disbanded_at}
									<dt>Disbanded</dt>
									<dd>{new Date(party.disbanded_at).toLocaleString()}</dd>
								{/if}
							</dl>
						</section>

						<!-- Members Grid -->
						<section class="detail-card full-width">
							<h2>Members ({party.members.length})</h2>
							<div class="members-grid">
								{#each party.members as member}
									{@const agent = resolveAgent(member.agent_id)}
									<a href="/agents/{member.agent_id}" class="member-card">
										<div class="member-header">
											<span class="member-name">
												{agent?.name ?? member.agent_id.slice(-8)}
											</span>
											<span class="role-badge">{member.role}</span>
										</div>
										{#if agent}
											<div class="member-level">
												Lv.{agent.level}
												{TrustTierNames[agent.tier as TrustTier]}
											</div>
										{/if}
										<div class="member-skills">
											{#each member.skills.slice(0, 3) as skill}
												<span class="skill-tag">{skill.replaceAll('_', ' ')}</span>
											{/each}
										</div>
										<div class="member-joined">
											Joined {new Date(member.joined_at).toLocaleDateString()}
										</div>
									</a>
								{:else}
									<p class="empty-state">No members</p>
								{/each}
							</div>
						</section>
					</div>
				</div>
			{:else}
				<div class="not-found" data-testid="party-not-found">
					<h2>Party not found</h2>
					<p>The party with ID "{id}" could not be found.</p>
					<a href="/parties">Back to Parties</a>
				</div>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="side-panel">
			<header class="panel-header">
				<h2>Details</h2>
			</header>
			<div class="side-content">
				<p class="empty-state">Additional details</p>
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	.party-detail {
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

	/* Party Header */
	.party-header {
		margin-bottom: var(--spacing-lg);
	}

	.party-identity {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
		flex-wrap: wrap;
	}

	.party-identity h1 {
		margin: 0;
	}

	.status-badge {
		padding: var(--spacing-xs) var(--spacing-sm);
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		font-weight: 500;
		text-transform: capitalize;
	}

	.status-badge[data-status='active'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.status-badge[data-status='forming'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.status-badge[data-status='disbanded'] {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
	}

	.strategy-badge {
		display: inline-block;
		font-size: 0.625rem;
		padding: 2px 8px;
		border-radius: var(--radius-sm);
		text-transform: uppercase;
		font-weight: 600;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
	}

	/* Detail Cards Grid */
	.party-details {
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

	.detail-card.full-width {
		grid-column: 1 / -1;
	}

	.detail-card h2 {
		font-size: 0.875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: 0 0 var(--spacing-md);
	}

	/* Quest Link */
	.quest-link {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		text-decoration: none;
		color: var(--ui-text-primary);
		transition: background-color 150ms ease;
	}

	.quest-link:hover {
		background: var(--ui-border-subtle);
		text-decoration: none;
	}

	.quest-title {
		flex: 1;
		font-weight: 500;
	}

	.quest-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		background: var(--ui-surface-primary);
		text-transform: capitalize;
	}

	.quest-status[data-status='posted'] {
		background: var(--quest-posted-container);
		color: var(--quest-posted);
	}

	.quest-status[data-status='claimed'] {
		background: var(--quest-claimed-container);
		color: var(--quest-claimed);
	}

	.quest-status[data-status='in_progress'] {
		background: var(--quest-in-progress-container);
		color: var(--quest-in-progress);
	}

	.quest-status[data-status='in_review'] {
		background: var(--quest-in-review-container);
		color: var(--quest-in-review);
	}

	.quest-status[data-status='completed'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}

	.quest-status[data-status='failed'] {
		background: var(--quest-failed-container);
		color: var(--quest-failed);
	}

	/* Agent Link */
	.agent-link {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		text-decoration: none;
		color: var(--ui-text-primary);
		transition: background-color 150ms ease;
	}

	.agent-link:hover {
		background: var(--ui-border-subtle);
		text-decoration: none;
	}

	.agent-name {
		flex: 1;
		font-weight: 500;
	}

	.agent-level {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-primary);
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

	/* Lifecycle List */
	.lifecycle-list {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
		margin: 0;
	}

	.lifecycle-list dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.lifecycle-list dd {
		margin: 0;
		font-size: 0.875rem;
		text-align: right;
	}

	/* Members Grid */
	.members-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
		gap: var(--spacing-md);
	}

	.member-card {
		display: block;
		background: var(--ui-surface-tertiary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		padding: var(--spacing-md);
		text-decoration: none;
		color: var(--ui-text-primary);
		transition: border-color 150ms ease;
	}

	.member-card:hover {
		border-color: var(--ui-border-interactive);
		text-decoration: none;
	}

	.member-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: var(--spacing-xs);
		margin-bottom: var(--spacing-xs);
	}

	.member-name {
		font-weight: 600;
		font-size: 0.875rem;
	}

	.role-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		color: var(--ui-text-secondary);
		text-transform: capitalize;
		white-space: nowrap;
	}

	.member-level {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		margin-bottom: var(--spacing-xs);
	}

	.member-skills {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		margin-bottom: var(--spacing-xs);
	}

	.skill-tag {
		font-size: 0.625rem;
		padding: 2px 6px;
		background: var(--ui-surface-primary);
		border-radius: var(--radius-sm);
		color: var(--ui-text-secondary);
		text-transform: capitalize;
	}

	.member-joined {
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
	}

	/* Empty / Not Found */
	.empty-text {
		color: var(--ui-text-tertiary);
		font-style: italic;
		margin: 0;
	}

	.empty-state {
		grid-column: 1 / -1;
		text-align: center;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-lg);
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

	/* Side Panel */
	.side-panel {
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

	.side-content {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
	}
</style>
