<script lang="ts">
	/**
	 * Quest Board - Kanban view of quests by status
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import { worldStore } from '$stores/worldStore.svelte';
	import { QuestDifficultyNames, type Quest, type QuestStatus, type BossBattle, type BattleVerdict } from '$types';
	import { formatTokenCount } from '$lib/utils/format';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// Track which party quests have their sub-quest list expanded
	let expandedParties = $state(new Set<string>());
	function togglePartyExpand(questId: string, event: Event) {
		event.stopPropagation();
		const next = new Set(expandedParties);
		if (next.has(questId)) next.delete(questId);
		else next.add(questId);
		expandedParties = next;
	}

	// Build parent→children index for party quest grouping
	const subQuestsByParent = $derived.by(() => {
		const index = new Map<string, Quest[]>();
		for (const quest of worldStore.questList) {
			if (quest.parent_quest) {
				const parentId = String(quest.parent_quest);
				const children = index.get(parentId) ?? [];
				children.push(quest);
				index.set(parentId, children);
			}
		}
		return index;
	});

	// Set of quest IDs that are sub-quests (so we can skip them in top-level rendering)
	const subQuestIds = $derived(
		new Set(worldStore.questList.filter((q) => q.parent_quest).map((q) => q.id))
	);

	// Group quests by status (top-level only — sub-quests rendered under their parent)
	const questsByStatus = $derived.by(() => {
		const groups: Record<QuestStatus, Quest[]> = {
			posted: [],
			claimed: [], // transient — not shown as column but needed for type completeness
			in_progress: [],
			in_review: [],
			completed: [],
			failed: [],
			escalated: [],
			cancelled: []
		};

		for (const quest of worldStore.questList) {
			if (!subQuestIds.has(quest.id)) {
				groups[quest.status].push(quest);
			}
		}

		return groups;
	});

	const allColumns: { status: QuestStatus; label: string }[] = [
		{ status: 'posted', label: 'Posted' },
		{ status: 'in_progress', label: 'In Progress' },
		{ status: 'in_review', label: 'In Review' },
		{ status: 'completed', label: 'Completed' },
		{ status: 'failed', label: 'Failed' },
		{ status: 'escalated', label: 'Escalated' },
		{ status: 'cancelled', label: 'Cancelled' }
	];

	const defaultStatuses: Set<QuestStatus> = new Set([
		'posted', 'in_progress', 'in_review', 'completed'
	]);

	let activeStatuses = $state<Set<QuestStatus>>(new Set(defaultStatuses));

	function toggleStatus(status: QuestStatus) {
		const next = new Set(activeStatuses);
		if (next.has(status)) {
			if (next.size > 1) next.delete(status);
		} else {
			next.add(status);
		}
		activeStatuses = next;
	}

	const kanbanColumns = $derived(
		allColumns.filter((col) => activeStatuses.has(col.status))
	);

	const filteredQuestCount = $derived(
		kanbanColumns.reduce((sum, col) => sum + questsByStatus[col.status].length, 0)
	);

	// Resolve verdict for the selected quest — from battle entity or quest's own verdict
	const selectedQuestVerdict: { verdict: BattleVerdict; battleId?: string } | undefined = $derived.by(() => {
		const quest = worldStore.selectedQuest;
		if (!quest) return undefined;

		// First check for a battle entity
		const qid = String(quest.id);
		const battle = worldStore.battleList
			.filter((b) => {
				const bid = String(b.quest_id);
				return bid === qid || bid.includes(qid) || qid.includes(bid);
			})
			.sort((a, b) => new Date(b.started_at ?? 0).getTime() - new Date(a.started_at ?? 0).getTime())[0];

		if (battle?.verdict) return { verdict: battle.verdict, battleId: String(battle.id) };

		// Fall back to quest's own verdict (set by auto-pass)
		if (quest.verdict) return { verdict: quest.verdict };

		return undefined;
	});

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
		<ExplorerNav />
	{/snippet}

	{#snippet centerPanel()}
		<div class="quest-board">
			<header class="board-header">
				<div class="board-title-row">
					<h1>Quest Board</h1>
					<span class="quest-count">{filteredQuestCount} / {worldStore.topLevelQuestList.length} quests</span>
				</div>
				<div class="status-filters" data-testid="quest-status-filters">
					{#each allColumns as col}
						{@const count = questsByStatus[col.status].length}
						<button
							type="button"
							class="filter-chip"
							class:active={activeStatuses.has(col.status)}
							data-status={col.status}
							onclick={() => toggleStatus(col.status)}
							data-testid="filter-{col.status}"
						>
							{col.label}
							{#if count > 0}
								<span class="filter-count">{count}</span>
							{/if}
						</button>
					{/each}
				</div>
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
					<div class="kanban-column" data-status={column.status} data-testid="quest-column-{column.status}">
						<header class="column-header">
							<h3>{column.label}</h3>
							<span class="column-count">{questsByStatus[column.status].length}</span>
						</header>
						<div class="column-content">
							{#each questsByStatus[column.status] as quest}
								{@const children = subQuestsByParent.get(String(quest.id)) ?? []}
								{#if children.length > 0}
									{@const failed = children.filter(c => c.status === 'failed').length}
									{@const escalated = children.filter(c => c.status === 'escalated').length}
									{@const completed = children.filter(c => c.status === 'completed').length}
									{@const inProgress = children.filter(c => c.status === 'in_progress' || c.status === 'in_review').length}
									<div class="quest-group">
										<button
											class="quest-card parent-card"
											class:selected={worldStore.selectedQuestId === quest.id}
											aria-label="{quest.title}, party quest with {children.length} sub-quests"
											aria-pressed={worldStore.selectedQuestId === quest.id}
											onclick={() => selectQuest(quest)}
											data-testid="quest-card"
										>
											<div class="quest-title-row">
												<h4 class="quest-title" data-testid="quest-title">{quest.title}</h4>
												<span
													class="party-badge"
													role="button"
													tabindex="0"
													onclick={(e) => togglePartyExpand(String(quest.id), e)}
													onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); togglePartyExpand(String(quest.id), e); } }}
													aria-expanded={expandedParties.has(String(quest.id))}
													aria-label="Toggle {children.length} sub-quests"
												>
													<span class="expand-chevron" class:expanded={expandedParties.has(String(quest.id))}>&#9656;</span>
													{children.length} sub-quests
												</span>
											</div>
											<div class="quest-meta">
												<span
													class="difficulty-badge"
													data-difficulty={quest.difficulty}
												>
													{QuestDifficultyNames[quest.difficulty]}
												</span>
												<span class="xp-badge">{quest.base_xp} XP</span>
												{#if (quest.tokens_prompt ?? 0) + (quest.tokens_completion ?? 0) > 0}
													<span class="token-badge">{formatTokenCount((quest.tokens_prompt ?? 0) + (quest.tokens_completion ?? 0))}</span>
												{/if}
											</div>
											{#if quest.claimed_by}
												<div class="quest-assignee">
													Lead: {worldStore.agentName(quest.claimed_by)}
												</div>
											{/if}
											<div class="sub-quest-summary">
												{#if completed > 0}
													<span class="summary-pip" data-status="completed">{completed}</span>
												{/if}
												{#if inProgress > 0}
													<span class="summary-pip" data-status="in_progress">{inProgress}</span>
												{/if}
												{#if failed > 0}
													<span class="summary-pip" data-status="failed">{failed}</span>
												{/if}
												{#if escalated > 0}
													<span class="summary-pip" data-status="escalated">{escalated}</span>
												{/if}
											</div>
										</button>
										{#if expandedParties.has(String(quest.id))}
											<div class="sub-quest-list">
												{#each children as child}
													<button
														class="quest-card sub-quest-card"
														class:selected={worldStore.selectedQuestId === child.id}
														aria-label="{child.title}, sub-quest, {QuestDifficultyNames[child.difficulty]} difficulty"
														aria-pressed={worldStore.selectedQuestId === child.id}
														onclick={() => selectQuest(child)}
														data-testid="quest-card"
													>
														<h4 class="quest-title" data-testid="quest-title">{child.title}</h4>
														<div class="quest-meta">
															<span class="status-pip" data-status={child.status}></span>
															<span class="difficulty-badge" data-difficulty={child.difficulty}>
																{QuestDifficultyNames[child.difficulty]}
															</span>
															<span class="xp-badge">{child.base_xp} XP</span>
															{#if (child.tokens_prompt ?? 0) + (child.tokens_completion ?? 0) > 0}
																<span class="token-badge">{formatTokenCount((child.tokens_prompt ?? 0) + (child.tokens_completion ?? 0))}</span>
															{/if}
														</div>
														{#if child.claimed_by}
															<div class="quest-assignee">
																{worldStore.agentName(child.claimed_by)}
															</div>
														{/if}
													</button>
												{/each}
											</div>
										{/if}
									</div>
								{:else}
									<button
										class="quest-card"
										class:selected={worldStore.selectedQuestId === quest.id}
										aria-label="{quest.title}, {QuestDifficultyNames[quest.difficulty]} difficulty, {quest.base_xp} XP"
										aria-pressed={worldStore.selectedQuestId === quest.id}
										onclick={() => selectQuest(quest)}
										data-testid="quest-card"
									>
										<h4 class="quest-title" data-testid="quest-title">{quest.title}</h4>
										<div class="quest-meta">
											<span
												class="difficulty-badge"
												data-difficulty={quest.difficulty}
											>
												{QuestDifficultyNames[quest.difficulty]}
											</span>
											<span class="xp-badge">{quest.base_xp} XP</span>
											{#if (quest.tokens_prompt ?? 0) + (quest.tokens_completion ?? 0) > 0}
												<span class="token-badge">{formatTokenCount((quest.tokens_prompt ?? 0) + (quest.tokens_completion ?? 0))}</span>
											{/if}
										</div>
										{#if quest.claimed_by}
											<div class="quest-assignee">
												Claimed by: {worldStore.agentName(quest.claimed_by)}
											</div>
										{/if}
										{#if quest.failure_reason && (quest.status === 'escalated' || quest.status === 'failed')}
											<p class="quest-escalation-hint">{quest.failure_reason}</p>
										{/if}
									</button>
								{/if}
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
		<div class="details-panel" data-testid="details-panel">
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
							{#if quest.parent_quest}
								<dt>Parent Quest</dt>
								<dd><a href="/quests/{quest.parent_quest}">{worldStore.questTitle(quest.parent_quest)}</a></dd>
							{/if}
							{#if (subQuestsByParent.get(String(quest.id)) ?? []).length > 0}
								<dt>Sub-Quests</dt>
								<dd>{(subQuestsByParent.get(String(quest.id)) ?? []).length} tasks</dd>
							{/if}
							<dt>Attempts</dt>
							<dd>{quest.attempts} / {quest.max_attempts}</dd>
							{#if quest.claimed_by}
								<dt>Claimed By</dt>
								<dd><a href="/agents/{quest.claimed_by}">{worldStore.agentName(quest.claimed_by)}</a></dd>
							{/if}
							{#if quest.loop_id}
								<dt>Trajectory</dt>
								<dd><a href="/trajectories/{quest.loop_id}">View</a></dd>
							{/if}
						</dl>

						{#if quest.escalated || quest.failure_reason || quest.status === 'failed'}
							<div class="escalation-block">
								<h4>{quest.status === 'failed' ? 'Failure Reason' : 'Escalation'}</h4>
								{#if quest.failure_type}
									<span class="status-badge" data-status="failed">{quest.failure_type}</span>
								{/if}
								{#if quest.failure_reason}
									<p class="escalation-reason-text">{quest.failure_reason}</p>
								{:else if quest.status === 'failed'}
									<p class="escalation-reason-text" style="opacity: 0.6">No failure details available. Check the trajectory for tool call history.</p>
								{/if}
								{#if Array.isArray(quest.dm_clarifications) && quest.dm_clarifications.length > 0}
									<span class="clarification-count">{quest.dm_clarifications.length} clarification{quest.dm_clarifications.length > 1 ? 's' : ''}</span>
								{/if}
								<a href="/quests/{quest.id}" class="escalation-details-link">View full context</a>
							</div>
						{/if}

						{#if selectedQuestVerdict}
							{@const v = selectedQuestVerdict}
							<div class="battle-card">
								<h4>Boss Battle</h4>
								<span class="verdict-badge" data-passed={v.verdict.passed}>
									{v.verdict.passed ? 'Victory' : 'Defeat'}
								</span>
								<span class="quality-score">{v.verdict.quality_score.toFixed(1)} quality</span>
								{#if v.verdict.xp_awarded > 0}
									<span class="xp-awarded">+{v.verdict.xp_awarded} XP</span>
								{/if}
								{#if v.battleId}
									<a href="/battles?selected={v.battleId}" class="battle-link">View battle</a>
								{/if}
							</div>
						{/if}

						<a href="/quests/{quest.id}" class="view-full-link">View full quest</a>
					</section>
				{:else}
					<p class="empty-state">Select a quest to view details</p>
				{/if}
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	/* Quest Board */
	.quest-board {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.board-header {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.board-title-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.board-title-row h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.quest-count {
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}

	/* Status filter chips */
	.status-filters {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
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

	/* Kanban Board */
	.kanban-board {
		flex: 1;
		display: flex;
		gap: var(--spacing-md);
		padding: var(--spacing-md);
		overflow-x: auto;
		overflow-y: hidden;
		min-height: 0;
		height: 0;
	}

	.kanban-column {
		flex: 0 0 280px;
		display: flex;
		flex-direction: column;
		background: var(--ui-surface-secondary);
		border-radius: var(--radius-lg);
		min-height: 0;
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
		min-height: 0;
	}

	.column-content > * {
		flex-shrink: 0;
	}

	/* Quest Card */
	.quest-card {
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		padding: var(--spacing-md);
		text-align: left;
		cursor: pointer;
		color: var(--ui-text-primary);
		font: inherit;
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

	.xp-badge,
	.token-badge {
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

	/* Party quest grouping */
	.quest-group {
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		overflow: hidden;
	}

	.quest-title-row {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		margin-bottom: var(--spacing-sm);
	}

	.quest-title-row .quest-title {
		flex: 1;
		margin: 0;
	}

	.party-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border: none;
		border-radius: var(--radius-sm);
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		white-space: nowrap;
		cursor: pointer;
		font: inherit;
		display: inline-flex;
		align-items: center;
		gap: 2px;
	}

	.party-badge:hover {
		filter: brightness(1.2);
	}

	.expand-chevron {
		display: inline-block;
		transition: transform 150ms ease;
		font-size: 0.5rem;
	}

	.expand-chevron.expanded {
		transform: rotate(90deg);
	}

	.sub-quest-summary {
		display: flex;
		gap: 6px;
		margin-top: 4px;
		flex-wrap: wrap;
	}

	.summary-pip {
		font-size: 0.6rem;
		padding: 1px 5px;
		border-radius: var(--radius-sm);
		color: var(--ui-text-primary);
		opacity: 0.85;
	}

	.summary-pip[data-status='completed'] { background: color-mix(in srgb, var(--quest-completed) 25%, transparent); }
	.summary-pip[data-status='in_progress'] { background: color-mix(in srgb, var(--quest-in-progress) 25%, transparent); }
	.summary-pip[data-status='failed'] { background: color-mix(in srgb, var(--quest-failed) 30%, transparent); color: var(--quest-failed); font-weight: 600; }
	.summary-pip[data-status='escalated'] { background: color-mix(in srgb, var(--quest-escalated) 30%, transparent); color: var(--quest-escalated); font-weight: 600; }

	.parent-card {
		border: none;
		border-radius: 0;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.sub-quest-list {
		display: flex;
		flex-direction: column;
		background: var(--ui-surface-tertiary);
	}

	.sub-quest-card {
		border: none;
		border-radius: 0;
		border-bottom: 1px solid var(--ui-border-subtle);
		padding: var(--spacing-sm) var(--spacing-md);
		padding-left: calc(var(--spacing-md) + 8px);
		font-size: 0.8125rem;
		background: var(--ui-surface-tertiary);
	}

	.sub-quest-card:last-child {
		border-bottom: none;
	}

	.sub-quest-card .quest-title {
		font-size: 0.8125rem;
	}

	.status-pip {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.status-pip[data-status='posted'] { background: var(--quest-posted); }
	.status-pip[data-status='claimed'] { background: var(--quest-claimed); }
	.status-pip[data-status='in_progress'] { background: var(--quest-in-progress); }
	.status-pip[data-status='in_review'] { background: var(--quest-in-review); }
	.status-pip[data-status='completed'] { background: var(--quest-completed); }
	.status-pip[data-status='failed'] { background: var(--quest-failed); }
	.status-pip[data-status='escalated'] { background: var(--quest-escalated); }
	.status-pip[data-status='pending_triage'] { background: var(--quest-pending-triage); }

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

	.status-badge[data-status='escalated'] {
		background: var(--quest-escalated-container);
		color: var(--quest-escalated);
	}

	.status-badge[data-status='pending_triage'] {
		background: var(--quest-pending-triage-container);
		color: var(--quest-pending-triage);
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

	/* Battle card in quest detail panel */
	.battle-card {
		margin-top: var(--spacing-md);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--spacing-xs) var(--spacing-sm);
	}

	.battle-card h4 {
		margin: 0;
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		width: 100%;
	}

	.verdict-badge {
		padding: 2px 8px;
		border-radius: var(--radius-sm);
		font-size: 0.75rem;
		font-weight: 600;
	}

	.verdict-badge[data-passed='true'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}

	.verdict-badge[data-passed='false'] {
		background: var(--quest-failed-container);
		color: var(--quest-failed);
	}

	.verdict-badge[data-passed='active'] {
		background: var(--quest-in-progress-container);
		color: var(--quest-in-progress);
	}

	.quality-score {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
	}

	.xp-awarded {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--status-success);
	}

	.battle-link {
		margin-left: auto;
		font-size: 0.75rem;
		color: var(--ui-interactive-primary);
	}

	/* Escalation hint on quest cards */
	.quest-escalation-hint {
		margin: var(--spacing-xs) 0 0;
		font-size: 0.6875rem;
		color: var(--quest-escalated, #e67e22);
		line-height: 1.3;
		overflow: hidden;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}

	/* Escalation block in right panel */
	.escalation-block {
		margin-top: var(--spacing-md);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--quest-escalated, #e67e22);
		border-radius: var(--radius-md);
	}

	.escalation-block h4 {
		margin: 0 0 var(--spacing-xs);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--quest-escalated, #e67e22);
	}

	.escalation-reason-text {
		margin: 0 0 var(--spacing-xs);
		font-size: 0.8125rem;
		line-height: 1.4;
		color: var(--ui-text-secondary);
		white-space: pre-wrap;
	}

	.clarification-count {
		display: block;
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		margin-bottom: var(--spacing-xs);
	}

	.escalation-details-link {
		font-size: 0.75rem;
		color: var(--ui-interactive-primary);
	}
</style>
