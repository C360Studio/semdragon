<script lang="ts">
	/**
	 * Quest Detail Page - Full quest view with trajectory
	 */

	import { page } from '$app/state';
	import { worldStore } from '$stores/worldStore.svelte';
	import { pageContext } from '$lib/stores/pageContext.svelte';
	import { QuestDifficultyNames, TrustTierNames, questId, extractInstance, type BossBattle } from '$types';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import ActivityFeed from '$components/ActivityFeed.svelte';
	import { listQuestArtifacts } from '$services/api';

	let artifactCount = $state<number | null>(null);

	// Fetch artifact count for completed/failed quests
	$effect(() => {
		const q = quest;
		if (q && (q.status === 'completed' || q.status === 'failed' || q.status === 'in_review')) {
			const instanceId = extractInstance(q.id);
			listQuestArtifacts(instanceId).then((result) => {
				artifactCount = result.count;
			}).catch(() => {
				artifactCount = null;
			});
		}
	});

	const id = $derived(questId(page.params.id ?? ''));
	const quest = $derived(worldStore.quests.get(id));

	// Sub-quests of this quest (for party quest parents)
	const subQuests = $derived(
		worldStore.questList.filter((q) => q.parent_quest && String(q.parent_quest) === String(id))
	);

	// Find the most recent battle for this quest
	const questBattle: BossBattle | undefined = $derived.by(() => {
		const qid = String(id);
		return worldStore.battleList
			.filter((b) => {
				const bid = String(b.quest_id);
				return bid === qid || bid.includes(qid) || qid.includes(bid);
			})
			.sort((a, b) => new Date(b.started_at ?? 0).getTime() - new Date(a.started_at ?? 0).getTime())[0];
	});

	$effect(() => {
		if (quest) {
			pageContext.set([{ type: 'quest', id: quest.id, label: quest.title }]);
		}
		return () => pageContext.clear();
	});

	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);
</script>

<svelte:head>
	<title>{quest?.title ?? 'Quest'} - Semdragons</title>
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
		<div class="quest-detail-page">
			<header class="page-header">
				<a href="/quests" class="back-link">Back to Quest Board</a>
			</header>

			{#if quest}
				<div class="quest-header">
					<h1>{quest.title}</h1>
					<span class="status-badge" data-status={quest.status}>{quest.status}</span>
				</div>

				<p class="quest-description">{quest.description}</p>

				<div class="quest-details">
					<section class="detail-card">
						<h2>Requirements</h2>
						<dl>
							<dt>Difficulty</dt>
							<dd>
								<span class="difficulty-badge" data-difficulty={quest.difficulty}>
									{QuestDifficultyNames[quest.difficulty]}
								</span>
							</dd>
							<dt>Min Tier</dt>
							<dd>{TrustTierNames[quest.min_tier]}</dd>
							<dt>Required Skills</dt>
							<dd>{quest.required_skills.join(', ') || 'None'}</dd>
							{#if quest.party_required}
								<dt>Party Size</dt>
								<dd>Min {quest.min_party_size} members</dd>
							{/if}
						</dl>
					</section>

					<section class="detail-card">
						<h2>Rewards</h2>
						<dl>
							<dt>Base XP</dt>
							<dd class="xp-value">{quest.base_xp}</dd>
							{#if quest.bonus_xp}
								<dt>Bonus XP</dt>
								<dd class="xp-value bonus">{quest.bonus_xp}</dd>
							{/if}
							{#if quest.guild_xp}
								<dt>Guild XP</dt>
								<dd>{quest.guild_xp}</dd>
							{/if}
						</dl>
					</section>

					<section class="detail-card">
						<h2>Lifecycle</h2>
						<dl>
							<dt>Posted</dt>
							<dd>{new Date(quest.posted_at).toLocaleString()}</dd>
							{#if quest.claimed_at}
								<dt>Claimed</dt>
								<dd>{new Date(quest.claimed_at).toLocaleString()}</dd>
							{/if}
							{#if quest.started_at}
								<dt>Started</dt>
								<dd>{new Date(quest.started_at).toLocaleString()}</dd>
							{/if}
							{#if quest.completed_at}
								<dt>Completed</dt>
								<dd>{new Date(quest.completed_at).toLocaleString()}</dd>
							{/if}
							<dt>Attempts</dt>
							<dd>{quest.attempts} / {quest.max_attempts}</dd>
						</dl>
					</section>

					{#if quest.claimed_by}
						<section class="detail-card">
							<h2>Assignment</h2>
							<dl>
								<dt>Claimed By</dt>
								<dd><a href="/agents/{quest.claimed_by}">{worldStore.agentName(quest.claimed_by)}</a></dd>
								{#if quest.party_id}
									<dt>Party</dt>
									<dd>{quest.party_id}</dd>
								{/if}
							</dl>
						</section>
					{/if}

				{#if quest.parent_quest}
					<section class="detail-card">
						<h2>Parent Quest</h2>
						<a href="/quests/{quest.parent_quest}" class="parent-quest-link">
							{worldStore.questTitle(quest.parent_quest)}
						</a>
					</section>
				{/if}

				{#if subQuests.length > 0}
					<section class="detail-card full-width">
						<h2>Sub-Quests ({subQuests.length})</h2>
						<div class="sub-quest-grid">
							{#each subQuests as sub}
								<a href="/quests/{sub.id}" class="sub-quest-item">
									<span class="sub-status-pip" data-status={sub.status}></span>
									<span class="sub-quest-title">{sub.title}</span>
									<span class="sub-quest-status">{sub.status}</span>
									{#if sub.claimed_by}
										<span class="sub-quest-agent">{worldStore.agentName(sub.claimed_by)}</span>
									{/if}
								</a>
							{/each}
						</div>
					</section>
				{/if}

					{#if quest.loop_id}
						<section class="detail-card full-width">
							<h2>Trajectory</h2>
							<a href="/trajectories/{quest.loop_id}" class="trajectory-link">
								View full trajectory timeline
							</a>
						</section>
					{/if}

					{#if questBattle}
						{@const battle = questBattle}
						<section class="detail-card full-width">
							<h2>Boss Battle</h2>
							<div class="battle-summary">
								<span class="verdict-badge" data-passed={battle.verdict ? battle.verdict.passed : 'active'}>
									{#if battle.verdict}
										{battle.verdict.passed ? 'Victory' : 'Defeat'}
									{:else}
										In Progress
									{/if}
								</span>
								{#if battle.verdict}
									<dl>
										<dt>Quality</dt>
										<dd>{battle.verdict.quality_score.toFixed(1)}</dd>
										{#if battle.verdict.xp_awarded > 0}
											<dt>XP Awarded</dt>
											<dd class="xp-value">+{battle.verdict.xp_awarded}</dd>
										{/if}
										{#if battle.verdict.xp_penalty > 0}
											<dt>XP Penalty</dt>
											<dd class="xp-penalty">-{battle.verdict.xp_penalty}</dd>
										{/if}
										{#if battle.verdict.feedback}
											<dt>Feedback</dt>
											<dd class="battle-feedback">{battle.verdict.feedback}</dd>
										{/if}
									</dl>
								{/if}
								<a href="/battles?selected={battle.id}" class="trajectory-link">View battle details</a>
							</div>
						</section>
					{/if}

					{#if artifactCount !== null && artifactCount > 0}
						<section class="detail-card full-width">
							<h2>Artifacts</h2>
							<p class="artifact-summary">{artifactCount} files produced</p>
							<a href="/workspace?quest={extractInstance(quest.id)}" class="trajectory-link">
								Browse in workspace
							</a>
						</section>
					{/if}
				</div>

				<ActivityFeed questId={quest.id} />
			{:else}
				<div class="not-found">
					<h2>Quest not found</h2>
					<p>The quest with ID "{id}" could not be found.</p>
					<a href="/quests">Back to Quest Board</a>
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
				<p class="empty-state">Quest context</p>
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	.quest-detail-page {
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

	.quest-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
		margin-bottom: var(--spacing-md);
	}

	.quest-header h1 {
		margin: 0;
		flex: 1;
	}

	.quest-description {
		color: var(--ui-text-secondary);
		margin-bottom: var(--spacing-xl);
		font-size: 1rem;
		line-height: 1.6;
	}

	.quest-details {
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

	.detail-card dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
		margin: 0;
	}

	.detail-card dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.detail-card dd {
		margin: 0;
	}

	.status-badge {
		padding: var(--spacing-xs) var(--spacing-sm);
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		font-weight: 500;
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

	.difficulty-badge {
		padding: 2px 8px;
		border-radius: var(--radius-sm);
		font-size: 0.75rem;
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

	.xp-value {
		font-weight: 600;
		color: var(--ui-interactive-primary);
	}

	.xp-value.bonus {
		color: var(--status-success);
	}

	.trajectory-link {
		display: inline-block;
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		border-radius: var(--radius-md);
		text-decoration: none;
	}

	.trajectory-link:hover {
		background: var(--ui-interactive-primary-hover);
		text-decoration: none;
	}

	.artifact-summary {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		margin: 0 0 var(--spacing-sm);
	}

	/* Parent/sub-quest relationships */
	.parent-quest-link {
		display: inline-block;
		color: var(--ui-interactive-primary);
		font-size: 0.875rem;
	}

	.sub-quest-grid {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.sub-quest-item {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		text-decoration: none;
		color: var(--ui-text-primary);
		font-size: 0.8125rem;
		transition: background 150ms ease;
	}

	.sub-quest-item:hover {
		background: var(--ui-surface-tertiary);
		text-decoration: none;
	}

	.sub-status-pip {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.sub-status-pip[data-status='posted'] { background: var(--quest-posted); }
	.sub-status-pip[data-status='claimed'] { background: var(--quest-claimed); }
	.sub-status-pip[data-status='in_progress'] { background: var(--quest-in-progress); }
	.sub-status-pip[data-status='in_review'] { background: var(--quest-in-review); }
	.sub-status-pip[data-status='completed'] { background: var(--quest-completed); }
	.sub-status-pip[data-status='failed'] { background: var(--quest-failed); }

	.sub-quest-title {
		flex: 1;
	}

	.sub-quest-status {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.sub-quest-agent {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
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

	/* Battle summary */
	.battle-summary {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.battle-summary dl {
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

	.xp-penalty {
		color: var(--quest-failed);
		font-weight: 600;
	}

	.battle-feedback {
		grid-column: 1 / -1;
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		line-height: 1.5;
		margin-top: var(--spacing-xs);
	}
</style>
