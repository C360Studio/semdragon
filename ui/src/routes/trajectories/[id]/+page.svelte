<script lang="ts">
	/**
	 * Trajectory Timeline View - Step-by-step timeline for a trajectory
	 *
	 * Loads trimmed data by default (messages/tool_calls stripped).
	 * Toggle "Full detail" to re-fetch with ?detail=full and see the
	 * complete conversation thread per model_call step.
	 */

	import { page } from '$app/state';
	import { api } from '$services/api';
	import type { Trajectory, TrajectoryStep } from '$types';
	import { worldStore } from '$stores/worldStore.svelte';
	import { pageContext } from '$lib/stores/pageContext.svelte';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import CopyButton from '$components/CopyButton.svelte';

	const trajectoryId = $derived(page.params.id ?? '');
	const quest = $derived(worldStore.questList.find((q) => q.loop_id === trajectoryId));
	const battle = $derived(worldStore.battleList.find((b) => b.loop_id === trajectoryId));

	$effect(() => {
		if (quest) {
			pageContext.set([{ type: 'quest', id: quest.id, label: quest.title }]);
		} else if (battle) {
			pageContext.set([{ type: 'battle', id: battle.id, label: `Battle #${String(battle.id).slice(-6)}` }]);
		}
		return () => pageContext.clear();
	});

	let trajectory = $state<Trajectory | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let expandedSteps = $state<Set<number>>(new Set());
	let fullDetail = $state(false);
	let fullLoading = $state(false);

	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	const steps = $derived(trajectory?.steps ?? []);
	const totalTokensIn = $derived(trajectory?.total_tokens_in ?? 0);
	const totalTokensOut = $derived(trajectory?.total_tokens_out ?? 0);

	// Side effect: fetch trajectory when route param or detail level changes.
	// Only trajectoryId and fullDetail are read synchronously (tracked deps).
	// Writes to trajectory/loading/error happen in async callbacks (untracked).
	$effect(() => {
		const tid = trajectoryId;
		const detail = fullDetail ? ('full' as const) : undefined;
		if (!tid) return;
		const controller = new AbortController();
		loading = true;
		fullLoading = !!detail;
		error = null;
		api.getTrajectory(tid, detail)
			.then((t) => {
				if (!controller.signal.aborted) trajectory = t;
			})
			.catch((err) => {
				if (!controller.signal.aborted) {
					console.error('Failed to load trajectory:', err);
					error = 'Failed to load trajectory';
				}
			})
			.finally(() => {
				if (!controller.signal.aborted) {
					loading = false;
					fullLoading = false;
				}
			});
		return () => controller.abort();
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
		if (step.step_type === 'model_call') return 'model_call';
		if (step.prompt) return 'request';
		if (step.response) return 'response';
		return step.step_type ?? 'unknown';
	}

	function stepIcon(step: TrajectoryStep): string {
		if (step.step_type === 'tool_call') return 'T';
		return '\u2192'; // →
	}

	function hasExpandableContent(step: TrajectoryStep): boolean {
		return !!(
			step.prompt ||
			step.response ||
			step.tool_result ||
			step.tool_arguments ||
			step.messages?.length ||
			step.tool_calls?.length
		);
	}

	function toggleStep(index: number) {
		const next = new Set(expandedSteps);
		if (next.has(index)) next.delete(index);
		else next.add(index);
		expandedSteps = next;
	}

	function expandAll() {
		expandedSteps = new Set(steps.map((_, i) => i));
	}

	function collapseAll() {
		expandedSteps = new Set();
	}

	function preview(text: string, max = 120): string {
		if (text.length <= max) return text;
		return text.slice(0, max) + '\u2026';
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
				<a href="/trajectories" class="back-link" data-testid="trajectory-back-link"
					>Back to Trajectories</a
				>
			</header>

			<div class="trajectory-header">
				<h1 data-testid="trajectory-heading">Trajectory Timeline</h1>
				<span class="trajectory-id" data-testid="trajectory-id">{trajectoryId}<CopyButton text={trajectoryId} variant="inline" label="Copy trajectory ID" /></span>
			</div>

			{#if quest}
				<div class="quest-context" data-testid="trajectory-quest-context">
					<a href="/quests/{quest.id}" class="quest-link">
						<span class="quest-title" data-testid="trajectory-quest-title"
							>{quest.title}</span
						>
						<span
							class="quest-status"
							data-testid="trajectory-quest-status"
							data-status={quest.status}>{quest.status}</span
						>
					</a>
				</div>
			{:else if battle}
				<div class="quest-context" data-testid="trajectory-battle-context">
					<a href="/battles/{battle.id}" class="quest-link">
						<span class="quest-title">Battle #{String(battle.id).slice(-6)}</span>
						<span class="quest-status" data-status={battle.status}>{battle.status}</span>
					</a>
				</div>
			{/if}

			{#if quest && (quest.context_token_count || quest.context_sources?.length || quest.context_entities?.length)}
				<div class="context-metadata" data-testid="trajectory-context-metadata">
					{#if quest.context_token_count}
						<span class="context-chip"
							>~{quest.context_token_count.toLocaleString()} context tokens</span
						>
					{/if}
					{#if quest.context_entities?.length}
						<span class="context-chip"
							>{quest.context_entities.length} entities</span
						>
					{/if}
					{#if quest.context_sources?.length}
						<details class="context-sources-detail">
							<summary class="context-chip"
								>{quest.context_sources.length} prompt fragments</summary
							>
							<ul class="context-source-list">
								{#each quest.context_sources as src}
									<li><code>{src}</code></li>
								{/each}
							</ul>
						</details>
					{/if}
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
							<span class="summary-item" data-testid="trajectory-outcome"
								>Outcome: <strong>{trajectory.outcome}</strong></span
							>
						{/if}
						{#if totalTokensIn > 0 || totalTokensOut > 0}
							<span class="summary-item" data-testid="trajectory-tokens"
								>Tokens: {totalTokensIn.toLocaleString()} in / {totalTokensOut.toLocaleString()}
								out</span
							>
						{/if}
						{#if trajectory.duration > 0}
							<span class="summary-item" data-testid="trajectory-duration"
								>Duration: {formatMs(trajectory.duration)}</span
							>
						{/if}
						{#if steps.length > 0}
							<span class="summary-actions">
								<button
									class="text-btn"
									class:active={fullDetail}
									disabled={fullLoading}
									data-testid="trajectory-full-toggle"
									onclick={() => (fullDetail = !fullDetail)}
								>
									{#if fullLoading}
										Loading...
									{:else if fullDetail}
										Trimmed view
									{:else}
										Full detail
									{/if}
								</button>
								<button class="text-btn" onclick={expandAll}>Expand all</button>
								<button class="text-btn" onclick={collapseAll}>Collapse all</button>
							</span>
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
						{#each steps as step, i}
							{@const expanded = expandedSteps.has(i)}
							{@const expandable = hasExpandableContent(step)}
							<div
								class="timeline-event"
								data-testid="timeline-event"
								data-step-type={step.step_type}
							>
								<div class="event-marker" data-step-type={step.step_type}
									>{stepIcon(step)}</div
								>
								<!-- svelte-ignore a11y_click_events_have_key_events -->
								<!-- svelte-ignore a11y_no_static_element_interactions -->
								<div
									class="event-content"
									class:expandable
									class:expanded
									onclick={expandable ? () => toggleStep(i) : undefined}
								>
									<div class="event-header">
										<span class="event-type" data-testid="event-type"
											>{stepLabel(step)}</span
										>
										{#if step.model}
											<span class="event-model">{step.model}</span>
										{/if}
										<span class="event-time" data-testid="event-time"
											>{formatTime(step.timestamp)}</span
										>
										{#if step.duration > 0}
											<span class="event-delta" data-testid="event-duration"
												>{formatMs(step.duration)}</span
											>
										{/if}
										{#if step.tokens_in || step.tokens_out}
											<span class="event-tokens" data-testid="event-tokens"
												>{step.tokens_in ?? 0}/{step.tokens_out ?? 0} tok</span
											>
										{/if}
										{#if expandable}
											<span class="expand-indicator"
												>{expanded ? '\u25BC' : '\u25B6'}</span
											>
										{/if}
									</div>

									{#if !expanded && expandable}
										<div class="event-preview">
											{#if step.prompt}{preview(step.prompt)}{/if}
											{#if step.response}{preview(step.response)}{/if}
											{#if step.tool_result}{preview(step.tool_result)}{/if}
										</div>
									{/if}

									{#if expanded}
										<div class="event-details">
											{#if step.messages?.length}
												<div class="detail-section">
													<span class="detail-label"
														>Messages ({step.messages.length})</span
													>
													<div class="chat-thread">
														{#each step.messages as msg}
															<div
																class="chat-message"
																data-role={msg.role}
															>
																<span class="chat-role"
																	>{msg.role}</span
																>
																{#if msg.reasoning_content}
																	<details
																		class="reasoning-block"
																	>
																		<summary
																			class="reasoning-label"
																			>Reasoning</summary
																		>
																		<div class="copyable">
																			<CopyButton text={msg.reasoning_content} />
																			<pre
																				class="detail-content">{msg.reasoning_content}</pre>
																		</div>
																	</details>
																{/if}
																{#if msg.content}
																	<div class="copyable">
																		<CopyButton text={msg.content} />
																		<pre
																			class="detail-content">{msg.content}</pre>
																	</div>
																{/if}
																{#if msg.tool_calls?.length}
																	<div class="msg-tool-calls">
																		{#each msg.tool_calls as tc}
																			<div
																				class="tool-call-chip"
																			>
																				<code
																					>{tc.name}</code
																				>
																				{#if tc.arguments}
																					<div class="copyable">
																						<CopyButton text={JSON.stringify(tc.arguments, null, 2)} />
																						<pre
																							class="detail-content">{JSON.stringify(tc.arguments, null, 2)}</pre>
																					</div>
																				{/if}
																			</div>
																		{/each}
																	</div>
																{/if}
																{#if msg.tool_call_id}
																	<span class="tool-call-ref"
																		>tool_call_id: {msg.tool_call_id}</span
																	>
																{/if}
															</div>
														{/each}
													</div>
												</div>
											{:else}
												{#if step.prompt}
													<div class="detail-section">
														<span class="detail-label">Prompt</span>
														<div class="copyable">
															<CopyButton text={step.prompt} />
															<pre
																class="detail-content"
																data-testid="event-prompt">{step.prompt}</pre>
														</div>
													</div>
												{/if}
												{#if step.response}
													<div class="detail-section">
														<span class="detail-label">Response</span>
														<div class="copyable">
															<CopyButton text={step.response} />
															<pre
																class="detail-content"
																data-testid="event-response">{step.response}</pre>
														</div>
													</div>
												{/if}
											{/if}

											{#if step.tool_calls?.length}
												<div class="detail-section">
													<span class="detail-label"
														>Tool Calls ({step.tool_calls.length})</span
													>
													{#each step.tool_calls as tc}
														<div class="tool-call-block">
															<div class="tool-call-header">
																<code>{tc.name}</code>
																<span class="tool-call-ref"
																	>id: {tc.id}</span
																>
															</div>
															{#if tc.arguments}
																<div class="copyable">
																	<CopyButton text={JSON.stringify(tc.arguments, null, 2)} />
																	<pre
																		class="detail-content">{JSON.stringify(tc.arguments, null, 2)}</pre>
																</div>
															{/if}
														</div>
													{/each}
												</div>
											{/if}

											{#if step.tool_name}
												<div class="detail-section">
													<span class="detail-label">Tool</span>
													<code class="detail-inline"
														>{step.tool_name}</code
													>
												</div>
											{/if}
											{#if step.tool_arguments}
												<div class="detail-section">
													<span class="detail-label">Arguments</span>
													<div class="copyable">
														<CopyButton text={JSON.stringify(step.tool_arguments, null, 2)} />
														<pre
															class="detail-content"
															data-testid="event-tool-args">{JSON.stringify(step.tool_arguments, null, 2)}</pre>
													</div>
												</div>
											{/if}
											{#if step.tool_result}
												<div class="detail-section">
													<span class="detail-label">Result</span>
													<div class="copyable">
														<CopyButton text={step.tool_result} />
														<pre
															class="detail-content"
															data-testid="event-tool-result">{step.tool_result}</pre>
													</div>
												</div>
											{/if}
											{#if step.request_id}
												<div class="detail-meta">
													request_id: {step.request_id}
												</div>
											{/if}
										</div>
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
				{#if quest?.context_entities?.length}
					<div class="related-section">
						<h3 class="related-heading">Context Entities</h3>
						<ul class="entity-list">
							{#each quest.context_entities as entityId}
								<li><code>{entityId}</code></li>
							{/each}
						</ul>
					</div>
				{/if}
				{#if !quest?.context_entities?.length}
					<p class="empty-state">No context data available</p>
				{/if}
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
		display: inline-flex;
		align-items: center;
		gap: var(--spacing-xs);
	}

	.copyable {
		position: relative;
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

	/* Context metadata chips */
	.context-metadata {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		gap: var(--spacing-sm);
		margin-bottom: var(--spacing-lg);
	}

	.context-chip {
		display: inline-flex;
		align-items: center;
		font-size: 0.75rem;
		padding: 2px 10px;
		background: var(--ui-surface-tertiary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-full);
		color: var(--ui-text-secondary);
		font-family: monospace;
	}

	.context-sources-detail {
		display: inline-flex;
		flex-direction: column;
	}

	.context-sources-detail summary:focus-visible {
		outline: 2px solid var(--ui-border-interactive);
		outline-offset: 2px;
		border-radius: var(--radius-full);
	}

	.context-source-list {
		list-style: none;
		margin: var(--spacing-xs) 0 0;
		padding: var(--spacing-xs) var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		font-size: 0.75rem;
	}

	.context-source-list li {
		padding: 2px 0;
		color: var(--ui-text-tertiary);
	}

	.context-source-list code {
		font-size: 0.6875rem;
	}

	.trajectory-summary {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
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

	.summary-actions {
		margin-left: auto;
		display: flex;
		gap: var(--spacing-sm);
	}

	.text-btn {
		background: none;
		border: none;
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
		cursor: pointer;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
	}

	.text-btn:hover {
		color: var(--ui-text-primary);
		background: var(--ui-surface-tertiary);
	}

	.text-btn.active {
		color: var(--ui-interactive-primary);
		background: var(--ui-surface-tertiary);
	}

	.text-btn:disabled {
		opacity: 0.5;
		cursor: default;
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
		left: calc(-1 * var(--spacing-xl) + 2px);
		top: 4px;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--ui-interactive-primary);
		border: 2px solid var(--ui-surface-primary);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.5rem;
		font-weight: 700;
		color: var(--ui-surface-primary);
	}

	.event-marker[data-step-type='tool_call'] {
		background: var(--ui-text-tertiary);
	}

	.event-content {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		padding: var(--spacing-md);
	}

	.event-content.expandable {
		cursor: pointer;
	}

	.event-content.expandable:hover {
		border-color: var(--ui-border-interactive);
	}

	.event-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
	}

	.event-type {
		font-weight: 600;
		font-size: 0.875rem;
	}

	.event-model {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-tertiary);
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		font-family: monospace;
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

	.expand-indicator {
		margin-left: auto;
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
	}

	.event-preview {
		margin-top: var(--spacing-xs);
		font-size: 0.8125rem;
		color: var(--ui-text-tertiary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.event-details {
		margin-top: var(--spacing-md);
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.detail-section {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.detail-label {
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
	}

	.detail-content {
		font-family: monospace;
		font-size: 0.8125rem;
		background: var(--ui-surface-primary);
		padding: var(--spacing-sm) var(--spacing-md);
		border-radius: var(--radius-sm);
		overflow-x: auto;
		margin: 0;
		color: var(--ui-text-secondary);
		white-space: pre-wrap;
		word-break: break-word;
		max-height: 400px;
		overflow-y: auto;
	}

	.detail-inline {
		font-size: 0.8125rem;
		background: var(--ui-surface-primary);
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		color: var(--ui-text-secondary);
	}

	.detail-meta {
		font-size: 0.6875rem;
		font-family: monospace;
		color: var(--ui-text-tertiary);
		padding-top: var(--spacing-xs);
		border-top: 1px solid var(--ui-border-subtle);
	}

	/* Chat thread (full detail mode) */
	.chat-thread {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
		max-height: 600px;
		overflow-y: auto;
	}

	.chat-message {
		padding: var(--spacing-sm) var(--spacing-md);
		border-radius: var(--radius-sm);
		border-left: 3px solid var(--ui-border-subtle);
	}

	.chat-message[data-role='system'] {
		border-left-color: var(--ui-text-tertiary);
		background: var(--ui-surface-primary);
	}

	.chat-message[data-role='user'] {
		border-left-color: var(--ui-interactive-primary);
	}

	.chat-message[data-role='assistant'] {
		border-left-color: var(--status-success, #4caf50);
	}

	.chat-message[data-role='tool'] {
		border-left-color: var(--ui-text-tertiary);
		background: var(--ui-surface-primary);
	}

	.chat-role {
		display: block;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin-bottom: 2px;
	}

	.chat-message .detail-content {
		max-height: 400px;
	}

	.reasoning-block {
		margin: var(--spacing-xs) 0;
	}

	.reasoning-label {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		cursor: pointer;
	}

	.msg-tool-calls {
		margin-top: var(--spacing-xs);
	}

	.tool-call-chip {
		margin-top: var(--spacing-xs);
	}

	/* Tool call blocks (full detail mode) */
	.tool-call-block {
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		padding: var(--spacing-sm);
		margin-top: var(--spacing-xs);
	}

	.tool-call-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
	}

	.tool-call-ref {
		font-size: 0.6875rem;
		font-family: monospace;
		color: var(--ui-text-tertiary);
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

	.related-section {
		margin-bottom: var(--spacing-lg);
	}

	.related-heading {
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: 0 0 var(--spacing-sm);
	}

	.entity-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.entity-list li {
		padding: 4px 0;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.entity-list li:last-child {
		border-bottom: none;
	}

	.entity-list code {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		word-break: break-all;
	}
</style>
