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

	// Filter state — clicking the active chip resets to 'all'
	let statusFilter = $state<string>('all');

	const statusChips = [
		{ status: 'all', label: 'All' },
		{ status: 'forming', label: 'Forming' },
		{ status: 'active', label: 'Active' },
		{ status: 'disbanded', label: 'Disbanded' }
	];

	const filteredParties = $derived(
		statusFilter === 'all'
			? worldStore.partyList
			: worldStore.partyList.filter((p) => p.status === statusFilter)
	);

	function countByStatus(status: string): number {
		if (status === 'all') return worldStore.partyList.length;
		return worldStore.partyList.filter((p) => p.status === status).length;
	}

	function selectFilter(status: string): void {
		statusFilter = statusFilter === status ? 'all' : status;
	}

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
				<span class="party-count" data-testid="party-count">{filteredParties.length} parties</span>
			</header>
			<div class="status-filters" data-testid="party-status-filters">
				{#each statusChips as chip}
					{@const count = countByStatus(chip.status)}
					<button
						type="button"
						class="filter-chip"
						class:active={statusFilter === chip.status}
						data-status={chip.status}
						onclick={() => selectFilter(chip.status)}
						data-testid="filter-{chip.status}"
					>
						{chip.label}
						{#if count > 0}
							<span class="filter-count">{count}</span>
						{/if}
					</button>
				{/each}
			</div>

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
						{#if party.status === 'disbanded' && party.disbanded_at}
							<div class="disbanded-date">
								Disbanded {new Date(party.disbanded_at).toLocaleDateString()}
							</div>
						{/if}
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
		border-bottom: none;
		flex-shrink: 0;
	}

	.parties-header h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.party-count {
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}

	/* Status filter chips */
	.status-filters {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		padding: 0 var(--spacing-lg) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
		flex-shrink: 0;
	}

	.filter-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 3px 10px;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-full);
		background: var(--ui-surface-primary);
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
		cursor: pointer;
		transition: all 150ms ease;
	}

	.filter-chip:hover {
		border-color: var(--ui-border-interactive);
		color: var(--ui-text-secondary);
	}

	.filter-chip.active {
		background: var(--ui-surface-tertiary);
		border-color: var(--ui-border-interactive);
		color: var(--ui-text-primary);
		font-weight: 500;
	}

	.filter-count {
		font-size: 0.625rem;
		padding: 0 5px;
		border-radius: var(--radius-full);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-tertiary);
		min-width: 16px;
		text-align: center;
	}

	.filter-chip.active .filter-count {
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
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

	/* Disbanded date */
	.disbanded-date {
		margin-top: var(--spacing-sm);
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		font-style: italic;
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
