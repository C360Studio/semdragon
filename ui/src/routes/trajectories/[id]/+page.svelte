<script lang="ts">
	/**
	 * Trajectory Timeline View - Step-by-step timeline for a trajectory
	 */

	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { api } from '$services/api';
	import type { Trajectory, TrajectoryStep } from '$types';
	import { worldStore } from '$stores/worldStore.svelte';
	import { pageContext } from '$lib/stores/pageContext.svelte';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';

	const trajectoryId = $derived($page.params.id ?? '');
	const quest = $derived(worldStore.questList.find((q) => q.trajectory_id === trajectoryId));

	$effect(() => {
		if (quest) {
			pageContext.set([{ type: 'quest', id: quest.id, label: quest.title }]);
		}
		return () => pageContext.clear();
	});

	let trajectory = $state<Trajectory | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	const steps = $derived(trajectory?.steps ?? []);
	const totalTokensIn = $derived(trajectory?.total_tokens_in ?? 0);
	const totalTokensOut = $derived(trajectory?.total_tokens_out ?? 0);

	onMount(async () => {
		if (!trajectoryId) return;
		try {
			trajectory = await api.getTrajectory(trajectoryId);
		} catch (err) {
			console.error('Failed to load trajectory:', err);
			error = 'Failed to load trajectory';
		} finally {
			loading = false;
		}
	});

	function formatTime(timestamp: string): string {
		return new Date(timestamp).toLocaleString();
	}

	function formatMs(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
		return `${(ms / 60000).toFixed(1)}m`;
	}

	function stepLabel(step: TrajectoryStep): string {
		if (step.step_type === 'tool_call') return step.tool_name ?? 'tool_call';
		return 'model_call';
	}

	function stepDetail(step: TrajectoryStep): string | null {
		if (step.step_type === 'tool_call' && step.tool_arguments) {
			return JSON.stringify(step.tool_arguments, null, 2);
		}
		if (step.step_type === 'model_call' && step.response) {
			return step.response.length > 500 ? step.response.slice(0, 500) + '...' : step.response;
		}
		return null;
	}
</script>

<svelte:head>
	<title>Trajectory {trajectoryId.slice(0, 8)} - Semdragons</title>
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
		<div class="trajectory-page" data-testid="trajectory-detail-page">
			<header class="page-header">
				<a href="/trajectories" class="back-link" data-testid="trajectory-back-link">Back to Trajectories</a>
			</header>

			<div class="trajectory-header">
				<h1 data-testid="trajectory-heading">Trajectory Timeline</h1>
				<span class="trajectory-id" data-testid="trajectory-id">{trajectoryId}</span>
			</div>

			{#if quest}
				<div class="quest-context" data-testid="trajectory-quest-context">
					<a href="/quests/{quest.id}" class="quest-link">
						<span class="quest-title" data-testid="trajectory-quest-title">{quest.title}</span>
						<span class="quest-status" data-testid="trajectory-quest-status" data-status={quest.status}>{quest.status}</span>
					</a>
				</div>
			{/if}

			{#if loading}
				<div class="loading" data-testid="trajectory-loading">Loading trajectory...</div>
			{:else if error}
				<div class="error" data-testid="trajectory-error">{error}</div>
			{:else if !trajectory}
				<div class="empty-state" data-testid="trajectory-not-found">
					<p>Trajectory not found.</p>
				</div>
			{:else}
				{#if trajectory.outcome || totalTokensIn > 0}
					<div class="trajectory-summary" data-testid="trajectory-summary">
						{#if trajectory.outcome}
							<span class="summary-item" data-testid="trajectory-outcome">Outcome: <strong>{trajectory.outcome}</strong></span>
						{/if}
						{#if totalTokensIn > 0 || totalTokensOut > 0}
							<span class="summary-item" data-testid="trajectory-tokens">Tokens: {totalTokensIn.toLocaleString()} in / {totalTokensOut.toLocaleString()} out</span>
						{/if}
						{#if trajectory.duration > 0}
							<span class="summary-item" data-testid="trajectory-duration">Duration: {formatMs(trajectory.duration)}</span>
						{/if}
					</div>
				{/if}

				{#if steps.length === 0}
					<div class="empty-state" data-testid="trajectory-empty-steps">
						<p>No steps recorded yet.</p>
						<p>Steps will appear here as the quest progresses.</p>
					</div>
				{:else}
					<div class="timeline" data-testid="trajectory-timeline">
						{#each steps as step}
							{@const detail = stepDetail(step)}
							<div class="timeline-event" data-testid="timeline-event" data-step-type={step.step_type}>
								<div class="event-marker"></div>
								<div class="event-content">
									<div class="event-header">
										<span class="event-type" data-testid="event-type">{stepLabel(step)}</span>
										<span class="event-time" data-testid="event-time">{formatTime(step.timestamp)}</span>
										{#if step.duration > 0}
											<span class="event-delta" data-testid="event-duration">{formatMs(step.duration)}</span>
										{/if}
										{#if step.tokens_in || step.tokens_out}
											<span class="event-tokens" data-testid="event-tokens">{step.tokens_in ?? 0}/{step.tokens_out ?? 0} tok</span>
										{/if}
									</div>
									{#if detail}
										<pre class="event-data" data-testid="event-data">{detail}</pre>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel">
			<header class="panel-header">
				<h2>Related</h2>
			</header>
			<div class="details-content">
				<p class="empty-state">Trajectory context</p>
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	.trajectory-page {
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

	.trajectory-header {
		margin-bottom: var(--spacing-md);
	}

	.trajectory-header h1 {
		margin: 0 0 var(--spacing-xs);
	}

	.trajectory-id {
		font-family: monospace;
		font-size: 0.875rem;
		color: var(--ui-text-tertiary);
	}

	.quest-context {
		margin-bottom: var(--spacing-lg);
	}

	.quest-link {
		display: inline-flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		color: var(--ui-text-primary);
		text-decoration: none;
	}

	.quest-link:hover {
		border-color: var(--ui-border-interactive);
		text-decoration: none;
	}

	.quest-status {
		font-size: 0.75rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.quest-status[data-status='completed'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}

	.trajectory-summary {
		display: flex;
		flex-wrap: wrap;
		gap: var(--spacing-md);
		padding: var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		margin-bottom: var(--spacing-lg);
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
	}

	.summary-item strong {
		color: var(--ui-text-primary);
	}

	/* Timeline */
	.timeline {
		position: relative;
		padding-left: var(--spacing-xl);
	}

	.timeline::before {
		content: '';
		position: absolute;
		left: 8px;
		top: 0;
		bottom: 0;
		width: 2px;
		background: var(--ui-border-subtle);
	}

	.timeline-event {
		position: relative;
		margin-bottom: var(--spacing-md);
	}

	.event-marker {
		position: absolute;
		left: calc(-1 * var(--spacing-xl) + 4px);
		top: 4px;
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: var(--ui-interactive-primary);
		border: 2px solid var(--ui-surface-primary);
	}

	.event-content {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		padding: var(--spacing-md);
	}

	.event-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
		margin-bottom: var(--spacing-sm);
	}

	.event-type {
		font-weight: 600;
		font-size: 0.875rem;
	}

	.event-time {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.event-delta {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-tertiary);
		padding: 2px 6px;
		border-radius: var(--radius-sm);
	}

	.event-tokens {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		font-family: monospace;
	}

	.event-data {
		font-family: monospace;
		font-size: 0.75rem;
		background: var(--ui-surface-primary);
		padding: var(--spacing-sm);
		border-radius: var(--radius-sm);
		overflow-x: auto;
		margin: 0;
		color: var(--ui-text-secondary);
	}

	.loading,
	.error,
	.empty-state {
		text-align: center;
		padding: var(--spacing-xl);
		color: var(--ui-text-tertiary);
	}

	.error {
		color: var(--status-error);
	}

	.empty-state p {
		margin: var(--spacing-sm) 0;
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
</style>
