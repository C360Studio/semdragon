<script lang="ts">
	/**
	 * Party List - Grid view of all parties with status filter
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import { worldStore } from '$stores/worldStore.svelte';
	import type { AgentID } from '$types';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// Filter state
	let statusFilter = $state<string>('all');

	const filteredParties = $derived(
		statusFilter === 'all'
			? worldStore.partyList
			: worldStore.partyList.filter((p) => p.status === statusFilter)
	);

	function resolveAgentName(agentId: AgentID): string {
		return worldStore.agents.get(agentId)?.name ?? agentId.slice(-8);
	}
</script>

<svelte:head>
	<title>Parties - Semdragons</title>
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
		<div class="parties-content">
			<header class="parties-header">
				<h1>Parties</h1>
				<div class="header-controls">
					<select
						bind:value={statusFilter}
						class="inline-filter"
						aria-label="Filter by status"
						data-testid="party-status-filter"
					>
						<option value="all">All Status</option>
						<option value="forming">Forming</option>
						<option value="active">Active</option>
						<option value="disbanded">Disbanded</option>
					</select>
					<span class="party-count" data-testid="party-count">{filteredParties.length} parties</span>
				</div>
			</header>

			<div class="party-grid">
				{#each filteredParties as party}
					<a
						href="/parties/{party.id}"
						class="party-card"
						data-status={party.status}
						data-testid="party-card"
					>
						<div class="party-header">
							<h2>{party.name || `Party #${party.id.slice(-6)}`}</h2>
							<span class="party-status">{party.status}</span>
						</div>
						<div class="party-badges">
							<span class="strategy-badge">{party.strategy}</span>
						</div>
						<div class="party-info">
							<div class="info-item">
								<span class="info-label">Quest</span>
								<span class="info-value">{party.quest_id ? party.quest_id.slice(-8) : 'None'}</span>
							</div>
							<div class="info-item">
								<span class="info-label">Members</span>
								<span class="info-value">{party.members.length}</span>
							</div>
							<div class="info-item">
								<span class="info-label">Lead</span>
								<span class="info-value">{resolveAgentName(party.lead)}</span>
							</div>
						</div>
					</a>
				{:else}
					<div class="empty-state">No parties found</div>
				{/each}
			</div>
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel" data-testid="details-panel">
			<header class="panel-header">
				<h2>Party Details</h2>
			</header>
			<div class="details-content">
				<p class="empty-state">Select a party to view details</p>
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	/* Parties Content */
	.parties-content {
		height: 100%;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
	}

	.parties-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
		flex-shrink: 0;
	}

	.parties-header h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.header-controls {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
	}

	.inline-filter {
		font-size: 0.875rem;
		padding: var(--spacing-xs) var(--spacing-sm);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
	}

	.party-count {
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}

	/* Party Grid */
	.party-grid {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: var(--spacing-md);
		align-content: start;
	}

	.party-card {
		display: block;
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-lg);
		color: var(--ui-text-primary);
		text-decoration: none;
		transition: border-color 150ms ease;
	}

	.party-card:hover {
		border-color: var(--ui-border-interactive);
		text-decoration: none;
	}

	.party-card:focus-visible {
		outline: 2px solid var(--ui-interactive-primary);
		outline-offset: 2px;
	}

	.party-card[data-status='active'] {
		border-left: 4px solid var(--status-success);
	}

	.party-card[data-status='forming'] {
		border-left: 4px solid var(--status-warning);
	}

	.party-card[data-status='disbanded'] {
		border-left: 4px solid var(--ui-border-subtle);
		opacity: 0.7;
	}

	/* Party Card Header */
	.party-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-sm);
	}

	.party-header h2 {
		margin: 0;
		font-size: 1.125rem;
	}

	.party-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		text-transform: capitalize;
		background: var(--ui-surface-tertiary);
	}

	/* Strategy Badge */
	.party-badges {
		margin-bottom: var(--spacing-sm);
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

	/* Party Info Grid */
	.party-info {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: var(--spacing-sm);
		padding: var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		margin-top: var(--spacing-sm);
	}

	.info-item {
		text-align: center;
	}

	.info-label {
		display: block;
		font-size: 0.625rem;
		text-transform: uppercase;
		color: var(--ui-text-tertiary);
	}

	.info-value {
		display: block;
		font-size: 0.875rem;
		font-weight: 500;
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

	.empty-state {
		grid-column: 1 / -1;
		text-align: center;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-xl);
	}
</style>
