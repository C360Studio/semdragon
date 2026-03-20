<script lang="ts">
	/**
	 * Boss Battle Arena - View of all battles
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import { worldStore } from '$stores/worldStore.svelte';
	import { ReviewLevelNames, type BossBattle } from '$types';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// Filter state
	let statusFilter = $state<string>('all');

	const battleStatuses = [
		{ status: 'all', label: 'All' },
		{ status: 'active', label: 'Active' },
		{ status: 'victory', label: 'Victory' },
		{ status: 'defeat', label: 'Defeat' },
		{ status: 'retreat', label: 'Retreat' }
	];

	const filteredBattles = $derived.by(() => {
		if (statusFilter === 'all') return worldStore.battleList;
		return worldStore.battleList.filter((b) => b.status === statusFilter);
	});

	function getBattleCount(status: string): number {
		if (status === 'all') return worldStore.battleList.length;
		return worldStore.battleList.filter((b) => b.status === status).length;
	}

	function toggleFilter(status: string) {
		statusFilter = statusFilter === status ? 'all' : status;
	}

	function selectBattle(battle: BossBattle) {
		worldStore.selectBattle(battle.id);
	}
</script>

<svelte:head>
	<title>Boss Battle Arena - Semdragons</title>
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
		<div class="battle-arena">
			<header class="arena-header">
				<div class="arena-title-row">
					<h1>Boss Battle Arena</h1>
					<span class="battle-count">{filteredBattles.length} battles</span>
				</div>
				<div class="status-filters" data-testid="battle-status-filters">
					{#each battleStatuses as { status, label }}
						{@const count = getBattleCount(status)}
						<button
							type="button"
							class="filter-chip"
							class:active={statusFilter === status}
							data-status={status}
							onclick={() => toggleFilter(status)}
							data-testid="filter-{status}"
						>
							{label}
							{#if count > 0}
								<span class="filter-count">{count}</span>
							{/if}
						</button>
					{/each}
				</div>
			</header>

			{#if worldStore.loading}
				<div class="loading-state">
					<div class="loading-list">
						<div class="skeleton-card"></div>
						<div class="skeleton-card"></div>
						<div class="skeleton-card"></div>
					</div>
				</div>
			{:else}
			<div class="battle-list">
				{#each filteredBattles as battle}
					<button
						class="battle-card"
						class:selected={worldStore.selectedBattleId === battle.id}
						data-status={battle.status}
						aria-label="{worldStore.questTitle(battle.quest_id)}, {battle.status}, {ReviewLevelNames[battle.level]} review"
						aria-pressed={worldStore.selectedBattleId === battle.id}
						onclick={() => selectBattle(battle)}
						data-testid="battle-card"
					>
						<div class="battle-header">
							<span class="battle-title">{worldStore.questTitle(battle.quest_id)}</span>
							<span class="battle-status" data-status={battle.status}>
								{battle.status}
							</span>
						</div>

						<div class="battle-info">
							<div class="info-row">
								<span class="label">Quest:</span>
								<a href="/quests/{battle.quest_id}" class="value quest-link">{worldStore.questTitle(battle.quest_id)}</a>
							</div>
							<div class="info-row">
								<span class="label">Agent:</span>
								<span class="value">{worldStore.agentName(battle.agent_id)}</span>
							</div>
							<div class="info-row">
								<span class="label">Review Level:</span>
								<span class="review-level" data-level={battle.level}>
									{ReviewLevelNames[battle.level]}
								</span>
							</div>
						</div>

						{#if battle.verdict}
							<div class="verdict-preview">
								<span class="quality-score">
									Quality: {(battle.verdict.quality_score * 100).toFixed(0)}%
								</span>
								{#if battle.verdict.xp_awarded > 0}
									<span class="xp-awarded">+{battle.verdict.xp_awarded} XP</span>
								{/if}
								{#if battle.verdict.xp_penalty > 0}
									<span class="xp-penalty">-{battle.verdict.xp_penalty} XP</span>
								{/if}
							</div>
						{/if}

						<div class="battle-time">
							Started: {new Date(battle.started_at).toLocaleString()}
						</div>
					</button>
				{:else}
					<div class="empty-state">No battles found</div>
				{/each}
			</div>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel" data-testid="details-panel">
			<header class="panel-header">
				<h2>Battle Details</h2>
			</header>
			<div class="details-content">
				{#if worldStore.selectedBattle}
					{@const battle = worldStore.selectedBattle}
					<section class="detail-section">
						<h3>{worldStore.questTitle(battle.quest_id)}</h3>

						<div class="status-display" data-status={battle.status}>
							{battle.status.toUpperCase()}
						</div>

						<dl class="detail-list">
							<dt>Quest</dt>
							<dd><a href="/quests/{battle.quest_id}">{worldStore.questTitle(battle.quest_id)}</a></dd>
							<dt>Agent</dt>
							<dd><a href="/agents/{battle.agent_id}">{worldStore.agentName(battle.agent_id)}</a></dd>
							<dt>Review Level</dt>
							<dd>{ReviewLevelNames[battle.level]}</dd>
							<dt>Started</dt>
							<dd>{new Date(battle.started_at).toLocaleString()}</dd>
							{#if battle.completed_at}
								<dt>Completed</dt>
								<dd>{new Date(battle.completed_at).toLocaleString()}</dd>
							{/if}
						</dl>

						<h4>Criteria</h4>
						<div class="criteria-list">
							{#each battle.criteria as criterion}
								{@const result = (battle.results ?? []).find((r) => r.criterion_name === criterion.name)}
								<div class="criterion-item" class:passed={result?.passed}>
									<div class="criterion-header">
										<span class="criterion-name">{criterion.name}</span>
										<span class="criterion-weight">Weight: {criterion.weight}</span>
									</div>
									<div class="criterion-bar">
										<div
											class="criterion-fill"
											style="width: {(result?.score ?? 0) * 100}%"
										></div>
										<div
											class="criterion-threshold"
											style="left: {criterion.threshold * 100}%"
										></div>
									</div>
									{#if result}
										<div class="criterion-score">
											Score: {(result.score * 100).toFixed(0)}% (threshold:
											{(criterion.threshold * 100).toFixed(0)}%)
										</div>
									{/if}
								</div>
							{/each}
						</div>

						{#if battle.verdict}
							<h4>Verdict</h4>
							<div class="verdict-card" class:passed={battle.verdict.passed}>
								<div class="verdict-result">
									{battle.verdict.passed ? 'VICTORY' : 'DEFEAT'}
								</div>
								<dl class="verdict-stats">
									<dt>Quality Score</dt>
									<dd>{(battle.verdict.quality_score * 100).toFixed(1)}%</dd>
									{#if battle.verdict.xp_awarded > 0}
										<dt>XP Awarded</dt>
										<dd class="xp-positive">+{battle.verdict.xp_awarded}</dd>
									{/if}
									{#if battle.verdict.xp_penalty > 0}
										<dt>XP Penalty</dt>
										<dd class="xp-negative">-{battle.verdict.xp_penalty}</dd>
									{/if}
									{#if battle.verdict.level_change !== 0}
										<dt>Level Change</dt>
										<dd class:positive={battle.verdict.level_change > 0}>
											{battle.verdict.level_change > 0 ? '+' : ''}{battle.verdict.level_change}
										</dd>
									{/if}
								</dl>
								{#if battle.verdict.feedback}
									<div class="verdict-feedback">
										<strong>Feedback:</strong>
										<p>{battle.verdict.feedback}</p>
									</div>
								{/if}
							</div>
						{/if}

						<a href="/battles/{battle.id}" class="view-full-link">View full battle</a>
					</section>
				{:else}
					<p class="empty-state">Select a battle to view details</p>
				{/if}
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	/* Battle Arena */
	.battle-arena {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.arena-header {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.arena-title-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.arena-title-row h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.battle-count {
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

	/* Battle List */
	.battle-list {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.battle-card {
		background: var(--ui-surface-secondary);
		border: 2px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-md);
		text-align: left;
		cursor: pointer;
		transition:
			border-color 150ms ease,
			box-shadow 150ms ease;
	}

	.battle-card:hover {
		border-color: var(--ui-border-interactive);
	}

	.battle-card.selected {
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 1px var(--ui-interactive-primary);
	}

	.battle-card[data-status='active'] {
		border-left-color: var(--battle-active);
		border-left-width: 4px;
	}

	.battle-card[data-status='victory'] {
		border-left-color: var(--battle-victory);
		border-left-width: 4px;
	}

	.battle-card[data-status='defeat'] {
		border-left-color: var(--battle-defeat);
		border-left-width: 4px;
	}

	.battle-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-sm);
	}

	.battle-title {
		font-weight: 600;
		font-size: 0.875rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
		margin-right: var(--spacing-sm);
	}

	.quest-link {
		color: var(--ui-interactive-primary);
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.quest-link:hover {
		text-decoration: underline;
	}

	.battle-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		text-transform: uppercase;
	}

	.battle-status[data-status='active'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}

	.battle-status[data-status='victory'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.battle-status[data-status='defeat'] {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.battle-status[data-status='retreat'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.battle-info {
		margin-bottom: var(--spacing-sm);
	}

	.info-row {
		display: flex;
		justify-content: space-between;
		font-size: 0.875rem;
		margin-bottom: 2px;
	}

	.info-row .label {
		color: var(--ui-text-tertiary);
	}

	.review-level {
		font-size: 0.75rem;
		padding: 2px 6px;
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
	}

	.verdict-preview {
		display: flex;
		gap: var(--spacing-sm);
		margin-bottom: var(--spacing-sm);
	}

	.quality-score {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
	}

	.xp-awarded {
		font-size: 0.75rem;
		color: var(--status-success);
	}

	.xp-penalty {
		font-size: 0.75rem;
		color: var(--status-error);
	}

	.battle-time {
		font-size: 0.75rem;
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

	.detail-section h3 {
		margin: 0 0 var(--spacing-md);
	}

	.status-display {
		text-align: center;
		padding: var(--spacing-md);
		border-radius: var(--radius-md);
		font-weight: 700;
		font-size: 1.25rem;
		margin-bottom: var(--spacing-md);
	}

	.status-display[data-status='active'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}

	.status-display[data-status='victory'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.status-display[data-status='defeat'] {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.detail-list {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
		margin-bottom: var(--spacing-md);
	}

	.detail-list dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.detail-list dd {
		margin: 0;
	}

	h4 {
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: var(--spacing-md) 0 var(--spacing-sm);
	}

	/* Criteria */
	.criteria-list {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.criterion-item {
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		padding: var(--spacing-sm);
	}

	.criterion-item.passed {
		border-left: 3px solid var(--status-success);
	}

	.criterion-header {
		display: flex;
		justify-content: space-between;
		margin-bottom: var(--spacing-xs);
	}

	.criterion-name {
		font-weight: 500;
		font-size: 0.875rem;
	}

	.criterion-weight {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.criterion-bar {
		position: relative;
		height: 8px;
		background: var(--ui-surface-primary);
		border-radius: 4px;
		overflow: hidden;
		margin-bottom: var(--spacing-xs);
	}

	.criterion-fill {
		height: 100%;
		background: var(--ui-interactive-primary);
	}

	.criterion-threshold {
		position: absolute;
		top: 0;
		bottom: 0;
		width: 2px;
		background: var(--status-warning);
	}

	.criterion-score {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
	}

	/* Verdict */
	.verdict-card {
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
		padding: var(--spacing-md);
		margin-bottom: var(--spacing-md);
	}

	.verdict-card.passed {
		border-left: 4px solid var(--status-success);
	}

	.verdict-card:not(.passed) {
		border-left: 4px solid var(--status-error);
	}

	.verdict-result {
		font-size: 1.25rem;
		font-weight: 700;
		margin-bottom: var(--spacing-sm);
	}

	.verdict-stats {
		display: grid;
		grid-template-columns: auto auto;
		gap: var(--spacing-xs) var(--spacing-md);
		margin-bottom: var(--spacing-sm);
	}

	.verdict-stats dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.verdict-stats dd {
		margin: 0;
		text-align: right;
	}

	.xp-positive {
		color: var(--status-success);
	}

	.xp-negative {
		color: var(--status-error);
	}

	.positive {
		color: var(--status-success);
	}

	.verdict-feedback {
		border-top: 1px solid var(--ui-border-subtle);
		padding-top: var(--spacing-sm);
		font-size: 0.875rem;
	}

	.verdict-feedback p {
		margin: var(--spacing-xs) 0 0;
		color: var(--ui-text-secondary);
	}

	.view-full-link {
		display: block;
		text-align: center;
		padding: var(--spacing-sm);
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

	.loading-list {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.skeleton-card {
		height: 120px;
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
