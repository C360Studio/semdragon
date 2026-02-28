<script lang="ts">
	/**
	 * Battle Detail Page - Full battle view with criteria scoring
	 */

	import { page } from '$app/stores';
	import { worldStore } from '$stores/worldStore.svelte';
	import { ReviewLevelNames, type BattleID, battleId } from '$types';

	const id = $derived(battleId($page.params.id ?? ''));
	const battle = $derived(worldStore.battles.get(id));
</script>

<svelte:head>
	<title>Battle {battle?.id.slice(0, 8) ?? ''} - Semdragons</title>
</svelte:head>

<div class="battle-detail-page">
	<header class="page-header">
		<a href="/battles" class="back-link">Back to Boss Battle Arena</a>
	</header>

	{#if battle}
		<div class="battle-content">
			<div class="battle-header">
				<h1>Battle #{battle.id.slice(0, 8)}</h1>
				<span class="battle-status" data-status={battle.status}>
					{battle.status.toUpperCase()}
				</span>
			</div>

			<div class="battle-summary">
				<div class="summary-item">
					<span class="summary-label">Quest</span>
					<a href="/quests/{battle.quest_id}" class="summary-value">{battle.quest_id}</a>
				</div>
				<div class="summary-item">
					<span class="summary-label">Agent</span>
					<a href="/agents/{battle.agent_id}" class="summary-value">{battle.agent_id}</a>
				</div>
				<div class="summary-item">
					<span class="summary-label">Review Level</span>
					<span class="summary-value">{ReviewLevelNames[battle.level]}</span>
				</div>
			</div>

			<section class="criteria-section">
				<h2>Review Criteria</h2>
				<div class="criteria-grid">
					{#each battle.criteria as criterion}
						{@const result = battle.results.find((r) => r.criterion_name === criterion.name)}
						<div class="criterion-card" class:passed={result?.passed}>
							<div class="criterion-header">
								<span class="criterion-name">{criterion.name}</span>
								{#if result}
									<span class="criterion-badge" class:pass={result.passed}>
										{result.passed ? 'PASS' : 'FAIL'}
									</span>
								{/if}
							</div>

							<p class="criterion-description">{criterion.description}</p>

							<div class="criterion-bar">
								<div
									class="bar-fill"
									class:passing={result && result.score >= criterion.threshold}
									style="width: {(result?.score ?? 0) * 100}%"
								></div>
								<div class="bar-threshold" style="left: {criterion.threshold * 100}%">
									<span class="threshold-label">{(criterion.threshold * 100).toFixed(0)}%</span>
								</div>
							</div>

							<div class="criterion-meta">
								<span>Score: {result ? (result.score * 100).toFixed(1) + '%' : 'Pending'}</span>
								<span>Weight: {criterion.weight}</span>
							</div>

							{#if result?.reasoning}
								<div class="criterion-reasoning">
									<strong>Reasoning:</strong>
									<p>{result.reasoning}</p>
								</div>
							{/if}
						</div>
					{/each}
				</div>
			</section>

			{#if battle.verdict}
				<section class="verdict-section">
					<h2>Final Verdict</h2>
					<div class="verdict-card" class:victory={battle.verdict.passed}>
						<div class="verdict-banner">
							{battle.verdict.passed ? 'VICTORY' : 'DEFEAT'}
						</div>

						<div class="verdict-stats">
							<div class="stat-item">
								<span class="stat-value">{(battle.verdict.quality_score * 100).toFixed(1)}%</span>
								<span class="stat-label">Quality Score</span>
							</div>
							<div class="stat-item xp">
								<span class="stat-value positive">+{battle.verdict.xp_awarded}</span>
								<span class="stat-label">XP Awarded</span>
							</div>
							{#if battle.verdict.xp_penalty > 0}
								<div class="stat-item xp">
									<span class="stat-value negative">-{battle.verdict.xp_penalty}</span>
									<span class="stat-label">XP Penalty</span>
								</div>
							{/if}
							{#if battle.verdict.level_change !== 0}
								<div class="stat-item">
									<span
										class="stat-value"
										class:positive={battle.verdict.level_change > 0}
										class:negative={battle.verdict.level_change < 0}
									>
										{battle.verdict.level_change > 0 ? '+' : ''}{battle.verdict.level_change}
									</span>
									<span class="stat-label">Level Change</span>
								</div>
							{/if}
						</div>

						{#if battle.verdict.feedback}
							<div class="verdict-feedback">
								<h3>Feedback</h3>
								<p>{battle.verdict.feedback}</p>
							</div>
						{/if}
					</div>
				</section>
			{/if}

			<section class="judges-section">
				<h2>Judges</h2>
				<div class="judges-list">
					{#each battle.judges as judge}
						<div class="judge-card">
							<span class="judge-type">{judge.type}</span>
							<span class="judge-id">{judge.id}</span>
						</div>
					{/each}
				</div>
			</section>

			<section class="timeline-section">
				<h2>Timeline</h2>
				<dl class="timeline-list">
					<dt>Started</dt>
					<dd>{new Date(battle.started_at).toLocaleString()}</dd>
					{#if battle.completed_at}
						<dt>Completed</dt>
						<dd>{new Date(battle.completed_at).toLocaleString()}</dd>
					{/if}
				</dl>
			</section>
		</div>
	{:else}
		<div class="not-found">
			<h2>Battle not found</h2>
			<p>The battle with ID "{id}" could not be found.</p>
			<a href="/battles">Back to Boss Battle Arena</a>
		</div>
	{/if}
</div>

<style>
	.battle-detail-page {
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

	.battle-content {
		max-width: 900px;
		margin: 0 auto;
	}

	.battle-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-lg);
	}

	.battle-header h1 {
		margin: 0;
	}

	.battle-status {
		padding: var(--spacing-sm) var(--spacing-md);
		border-radius: var(--radius-full);
		font-weight: 700;
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

	.battle-summary {
		display: flex;
		gap: var(--spacing-xl);
		margin-bottom: var(--spacing-xl);
		padding: var(--spacing-lg);
		background: var(--ui-surface-secondary);
		border-radius: var(--radius-lg);
	}

	.summary-item {
		display: flex;
		flex-direction: column;
	}

	.summary-label {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
	}

	.summary-value {
		font-weight: 500;
	}

	/* Criteria Section */
	.criteria-section,
	.verdict-section,
	.judges-section,
	.timeline-section {
		margin-bottom: var(--spacing-xl);
	}

	h2 {
		font-size: 1rem;
		margin-bottom: var(--spacing-md);
	}

	.criteria-grid {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.criterion-card {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		padding: var(--spacing-lg);
	}

	.criterion-card.passed {
		border-left: 4px solid var(--status-success);
	}

	.criterion-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--spacing-sm);
	}

	.criterion-name {
		font-weight: 600;
		font-size: 1rem;
	}

	.criterion-badge {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		font-weight: 600;
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.criterion-badge.pass {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.criterion-description {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		margin-bottom: var(--spacing-md);
	}

	.criterion-bar {
		position: relative;
		height: 24px;
		background: var(--ui-surface-primary);
		border-radius: 12px;
		overflow: visible;
		margin-bottom: var(--spacing-sm);
	}

	.bar-fill {
		height: 100%;
		background: var(--status-error);
		border-radius: 12px;
		transition: width 500ms ease;
	}

	.bar-fill.passing {
		background: var(--status-success);
	}

	.bar-threshold {
		position: absolute;
		top: -4px;
		bottom: -4px;
		width: 3px;
		background: var(--ui-text-secondary);
		border-radius: 2px;
	}

	.threshold-label {
		position: absolute;
		top: -20px;
		left: 50%;
		transform: translateX(-50%);
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
		white-space: nowrap;
	}

	.criterion-meta {
		display: flex;
		justify-content: space-between;
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.criterion-reasoning {
		margin-top: var(--spacing-md);
		padding-top: var(--spacing-md);
		border-top: 1px solid var(--ui-border-subtle);
	}

	.criterion-reasoning p {
		margin: var(--spacing-xs) 0 0;
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
	}

	/* Verdict */
	.verdict-card {
		background: var(--ui-surface-secondary);
		border: 2px solid var(--status-error);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}

	.verdict-card.victory {
		border-color: var(--status-success);
	}

	.verdict-banner {
		padding: var(--spacing-lg);
		text-align: center;
		font-size: 2rem;
		font-weight: 700;
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.verdict-card.victory .verdict-banner {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.verdict-stats {
		display: flex;
		justify-content: center;
		gap: var(--spacing-xl);
		padding: var(--spacing-lg);
	}

	.stat-item {
		text-align: center;
	}

	.stat-value {
		display: block;
		font-size: 1.5rem;
		font-weight: 700;
	}

	.stat-value.positive {
		color: var(--status-success);
	}

	.stat-value.negative {
		color: var(--status-error);
	}

	.stat-label {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.verdict-feedback {
		padding: var(--spacing-lg);
		border-top: 1px solid var(--ui-border-subtle);
	}

	.verdict-feedback h3 {
		font-size: 0.875rem;
		margin: 0 0 var(--spacing-sm);
	}

	.verdict-feedback p {
		margin: 0;
		color: var(--ui-text-secondary);
	}

	/* Judges */
	.judges-list {
		display: flex;
		flex-wrap: wrap;
		gap: var(--spacing-sm);
	}

	.judge-card {
		display: flex;
		flex-direction: column;
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
	}

	.judge-type {
		font-size: 0.625rem;
		text-transform: uppercase;
		color: var(--ui-text-tertiary);
	}

	.judge-id {
		font-weight: 500;
	}

	/* Timeline */
	.timeline-list {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
	}

	.timeline-list dt {
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}

	.timeline-list dd {
		margin: 0;
	}

	.not-found {
		text-align: center;
		padding: var(--spacing-xl);
	}
</style>
