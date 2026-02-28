<script lang="ts">
	/**
	 * Quest Board - Kanban view of quests by status
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import { worldStore } from '$stores/worldStore.svelte';
	import { QuestDifficultyNames, type Quest, type QuestStatus } from '$types';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// Group quests by status
	const questsByStatus = $derived.by(() => {
		const groups: Record<QuestStatus, Quest[]> = {
			posted: [],
			claimed: [],
			in_progress: [],
			in_review: [],
			completed: [],
			failed: [],
			escalated: [],
			cancelled: []
		};

		for (const quest of worldStore.questList) {
			groups[quest.status].push(quest);
		}

		return groups;
	});

	const kanbanColumns: { status: QuestStatus; label: string }[] = [
		{ status: 'posted', label: 'Posted' },
		{ status: 'claimed', label: 'Claimed' },
		{ status: 'in_progress', label: 'In Progress' },
		{ status: 'in_review', label: 'In Review' },
		{ status: 'completed', label: 'Completed' }
	];

	function selectQuest(quest: Quest) {
		worldStore.selectQuest(quest.id);
	}
</script>

<svelte:head>
	<title>Quest Board - Semdragons</title>
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
				<p class="placeholder">Quest filters coming soon</p>
			</div>
		</div>
	{/snippet}

	{#snippet centerPanel()}
		<div class="quest-board">
			<header class="board-header">
				<h1>Quest Board</h1>
				<span class="quest-count">{worldStore.questList.length} quests</span>
			</header>

			{#if worldStore.loading}
				<div class="loading-state">
					<div class="loading-board">
						{#each Array(5) as _}
							<div class="skeleton-column">
								<div class="skeleton-header"></div>
								<div class="skeleton-cards">
									<div class="skeleton-card"></div>
									<div class="skeleton-card"></div>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{:else}
			<div class="kanban-board">
				{#each kanbanColumns as column}
					<div class="kanban-column" data-status={column.status}>
						<header class="column-header">
							<h3>{column.label}</h3>
							<span class="column-count">{questsByStatus[column.status].length}</span>
						</header>
						<div class="column-content">
							{#each questsByStatus[column.status] as quest}
								<button
									class="quest-card"
									class:selected={worldStore.selectedQuestId === quest.id}
									aria-label="{quest.title}, {QuestDifficultyNames[quest.difficulty]} difficulty, {quest.base_xp} XP"
									aria-pressed={worldStore.selectedQuestId === quest.id}
									onclick={() => selectQuest(quest)}
								>
									<h4 class="quest-title">{quest.title}</h4>
									<div class="quest-meta">
										<span
											class="difficulty-badge"
											data-difficulty={quest.difficulty}
										>
											{QuestDifficultyNames[quest.difficulty]}
										</span>
										<span class="xp-badge">{quest.base_xp} XP</span>
									</div>
									{#if quest.claimed_by}
										<div class="quest-assignee">
											Claimed by: {quest.claimed_by}
										</div>
									{/if}
								</button>
							{:else}
								<div class="empty-column">No quests</div>
							{/each}
						</div>
					</div>
				{/each}
			</div>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel">
			<header class="panel-header">
				<h2>Quest Details</h2>
			</header>
			<div class="details-content">
				{#if worldStore.selectedQuest}
					{@const quest = worldStore.selectedQuest}
					<section class="detail-section">
						<h3>{quest.title}</h3>
						<p class="quest-description">{quest.description}</p>

						<dl class="detail-list">
							<dt>Status</dt>
							<dd>
								<span class="status-badge" data-status={quest.status}>{quest.status}</span>
							</dd>
							<dt>Difficulty</dt>
							<dd>{QuestDifficultyNames[quest.difficulty]}</dd>
							<dt>Base XP</dt>
							<dd>{quest.base_xp}</dd>
							{#if quest.bonus_xp}
								<dt>Bonus XP</dt>
								<dd>{quest.bonus_xp}</dd>
							{/if}
							<dt>Required Skills</dt>
							<dd>{quest.required_skills.join(', ') || 'None'}</dd>
							<dt>Min Tier</dt>
							<dd>{quest.min_tier}</dd>
							{#if quest.party_required}
								<dt>Party Required</dt>
								<dd>Yes (min {quest.min_party_size})</dd>
							{/if}
							<dt>Attempts</dt>
							<dd>{quest.attempts} / {quest.max_attempts}</dd>
							{#if quest.claimed_by}
								<dt>Claimed By</dt>
								<dd><a href="/agents/{quest.claimed_by}">{quest.claimed_by}</a></dd>
							{/if}
							{#if quest.trajectory_id}
								<dt>Trajectory</dt>
								<dd><a href="/trajectories/{quest.trajectory_id}">View</a></dd>
							{/if}
						</dl>
					</section>
				{:else}
					<p class="empty-state">Select a quest to view details</p>
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
		flex: 1;
		padding: var(--spacing-md);
	}

	.placeholder {
		color: var(--ui-text-tertiary);
		font-style: italic;
	}

	/* Quest Board */
	.quest-board {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.board-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.board-header h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.quest-count {
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}

	/* Kanban Board */
	.kanban-board {
		flex: 1;
		display: flex;
		gap: var(--spacing-md);
		padding: var(--spacing-md);
		overflow-x: auto;
	}

	.kanban-column {
		flex: 0 0 280px;
		display: flex;
		flex-direction: column;
		background: var(--ui-surface-secondary);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}

	.column-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-tertiary);
	}

	.column-header h3 {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
	}

	.column-count {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-primary);
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.column-content {
		flex: 1;
		padding: var(--spacing-sm);
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	/* Quest Card */
	.quest-card {
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		padding: var(--spacing-md);
		text-align: left;
		cursor: pointer;
		transition:
			border-color 150ms ease,
			box-shadow 150ms ease;
	}

	.quest-card:hover {
		border-color: var(--ui-border-interactive);
	}

	.quest-card.selected {
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 1px var(--ui-interactive-primary);
	}

	.quest-title {
		margin: 0 0 var(--spacing-sm);
		font-size: 0.875rem;
		font-weight: 500;
	}

	.quest-meta {
		display: flex;
		gap: var(--spacing-sm);
		margin-bottom: var(--spacing-xs);
	}

	.difficulty-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		text-transform: uppercase;
		font-weight: 600;
	}

	.difficulty-badge[data-difficulty='0'] {
		background: var(--difficulty-trivial-container);
		color: var(--difficulty-trivial);
	}
	.difficulty-badge[data-difficulty='1'] {
		background: var(--difficulty-easy-container);
		color: var(--difficulty-easy);
	}
	.difficulty-badge[data-difficulty='2'] {
		background: var(--difficulty-moderate-container);
		color: var(--difficulty-moderate);
	}
	.difficulty-badge[data-difficulty='3'] {
		background: var(--difficulty-hard-container);
		color: var(--difficulty-hard);
	}
	.difficulty-badge[data-difficulty='4'] {
		background: var(--difficulty-epic-container);
		color: var(--difficulty-epic);
	}
	.difficulty-badge[data-difficulty='5'] {
		background: var(--difficulty-legendary-container);
		color: var(--difficulty-legendary);
	}

	.xp-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
	}

	.quest-assignee {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		margin-top: var(--spacing-xs);
	}

	.empty-column {
		text-align: center;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-lg);
		font-size: 0.875rem;
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
		font-size: 1rem;
		margin-bottom: var(--spacing-sm);
	}

	.quest-description {
		color: var(--ui-text-secondary);
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
		font-size: 0.875rem;
	}

	.status-badge {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.status-badge[data-status='posted'] {
		background: var(--quest-posted-container);
		color: var(--quest-posted);
	}
	.status-badge[data-status='claimed'] {
		background: var(--quest-claimed-container);
		color: var(--quest-claimed);
	}
	.status-badge[data-status='in_progress'] {
		background: var(--quest-in-progress-container);
		color: var(--quest-in-progress);
	}
	.status-badge[data-status='in_review'] {
		background: var(--quest-in-review-container);
		color: var(--quest-in-review);
	}
	.status-badge[data-status='completed'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}
	.status-badge[data-status='failed'] {
		background: var(--quest-failed-container);
		color: var(--quest-failed);
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

	.loading-board {
		display: flex;
		gap: var(--spacing-md);
		height: 100%;
	}

	.skeleton-column {
		flex: 0 0 280px;
		background: var(--ui-surface-secondary);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}

	.skeleton-header {
		height: 40px;
		background: var(--ui-surface-tertiary);
	}

	.skeleton-cards {
		padding: var(--spacing-sm);
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.skeleton-card {
		height: 80px;
		background: linear-gradient(
			90deg,
			var(--ui-surface-primary) 0%,
			var(--ui-surface-tertiary) 50%,
			var(--ui-surface-primary) 100%
		);
		background-size: 200% 100%;
		border-radius: var(--radius-md);
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
