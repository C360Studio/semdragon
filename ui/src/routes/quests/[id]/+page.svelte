<script lang="ts">
	/**
	 * Quest Detail Page - Full quest view with trajectory
	 */

	import { page } from '$app/stores';
	import { worldStore } from '$stores/worldStore.svelte';
	import { QuestDifficultyNames, TrustTierNames, type QuestID, questId } from '$types';

	const id = $derived(questId($page.params.id ?? ''));
	const quest = $derived(worldStore.quests.get(id));
</script>

<svelte:head>
	<title>{quest?.title ?? 'Quest'} - Semdragons</title>
</svelte:head>

<div class="quest-detail-page">
	<header class="page-header">
		<a href="/quests" class="back-link">Back to Quest Board</a>
	</header>

	{#if quest}
		<div class="quest-content">
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
							<dd><a href="/agents/{quest.claimed_by}">{quest.claimed_by}</a></dd>
							{#if quest.party_id}
								<dt>Party</dt>
								<dd>{quest.party_id}</dd>
							{/if}
						</dl>
					</section>
				{/if}

				{#if quest.trajectory_id}
					<section class="detail-card full-width">
						<h2>Trajectory</h2>
						<a href="/trajectories/{quest.trajectory_id}" class="trajectory-link">
							View full trajectory timeline
						</a>
					</section>
				{/if}
			</div>
		</div>
	{:else}
		<div class="not-found">
			<h2>Quest not found</h2>
			<p>The quest with ID "{id}" could not be found.</p>
			<a href="/quests">Back to Quest Board</a>
		</div>
	{/if}
</div>

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

	.quest-content {
		max-width: 900px;
		margin: 0 auto;
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
