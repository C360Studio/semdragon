<script lang="ts">
	/**
	 * AgentQuestHistory — shows all quests an agent has worked on.
	 *
	 * Derives filtered quest list from worldStore using the agent's full ID.
	 * Supports both exact claimed_by matches and substring matches since entity
	 * IDs are six-part dotted strings and some paths store only the instance segment.
	 */

	import { worldStore } from '$stores/worldStore.svelte';
	import type { Quest, QuestDifficulty, QuestStatus } from '$types';
	import { QuestDifficultyNames } from '$types';
	import { formatDate } from '$lib/utils/format';

	let { agentId }: { agentId: string } = $props();

	const agentQuests = $derived.by(() => {
		return worldStore.questList
			.filter((q) => {
				if (!q.claimed_by) return false;
				const claimedBy = String(q.claimed_by);
				return claimedBy === agentId || claimedBy.includes(agentId) || agentId.includes(claimedBy);
			})
			.sort((a, b) => {
				const aTime = new Date(a.completed_at ?? a.started_at ?? a.claimed_at ?? a.posted_at ?? 0).getTime();
				const bTime = new Date(b.completed_at ?? b.started_at ?? b.claimed_at ?? b.posted_at ?? 0).getTime();
				return bTime - aTime;
			});
	});

	const questCounts = $derived.by(() => {
		const counts = { completed: 0, failed: 0, active: 0, other: 0 };
		for (const q of agentQuests) {
			if (q.status === 'completed') counts.completed++;
			else if (q.status === 'failed') counts.failed++;
			else if (['claimed', 'in_progress', 'in_review'].includes(q.status)) counts.active++;
			else counts.other++;
		}
		return counts;
	});

	function questStatusLabel(status: QuestStatus): string {
		return status.replaceAll('_', ' ');
	}

	function difficultyLabel(difficulty: QuestDifficulty): string {
		return QuestDifficultyNames[difficulty] ?? String(difficulty);
	}


</script>

<div class="quest-history">
	{#if agentQuests.length > 0}
		<div class="summary-row">
			<span class="summary-chip success">{questCounts.completed} completed</span>
			<span class="summary-chip error">{questCounts.failed} failed</span>
			{#if questCounts.active > 0}
				<span class="summary-chip active">{questCounts.active} active</span>
			{/if}
		</div>

		<ul class="history-list">
			{#each agentQuests as quest (quest.id)}
				<li class="history-item">
					<a href="/quests/{quest.id}" class="item-link">
						<div class="item-main">
							<span class="item-title">{quest.title}</span>
							<div class="item-badges">
								<span class="difficulty-badge" data-difficulty={quest.difficulty}>
									{difficultyLabel(quest.difficulty)}
								</span>
								<span class="status-badge" data-status={quest.status}>
									{questStatusLabel(quest.status)}
								</span>
							</div>
						</div>
						<span class="item-time">{formatDate(quest.completed_at ?? quest.started_at ?? quest.claimed_at ?? quest.posted_at)}</span>
					</a>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="empty-state">No quests yet</p>
	{/if}
</div>

<style>
	.quest-history {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.summary-row {
		display: flex;
		gap: var(--spacing-sm);
		flex-wrap: wrap;
	}

	.summary-chip {
		padding: 2px 10px;
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		font-weight: 500;
	}

	.summary-chip.success {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.summary-chip.error {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.summary-chip.active {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.history-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
		background: var(--ui-border-subtle);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}

	.history-item {
		background: var(--ui-surface-secondary);
	}

	.item-link {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		text-decoration: none;
		color: var(--ui-text-primary);
		transition: background 150ms ease;
	}

	.item-link:hover {
		background: var(--ui-surface-tertiary);
		text-decoration: none;
	}

	.item-main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.item-title {
		font-size: 0.875rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.item-badges {
		display: flex;
		gap: var(--spacing-xs);
		flex-wrap: wrap;
	}

	.difficulty-badge,
	.status-badge {
		font-size: 0.625rem;
		padding: 1px 6px;
		border-radius: var(--radius-sm);
		text-transform: capitalize;
		font-weight: 600;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
	}

	.status-badge[data-status='completed'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.status-badge[data-status='failed'] {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.status-badge[data-status='in_progress'],
	.status-badge[data-status='in_review'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.difficulty-badge[data-difficulty='4'],
	.difficulty-badge[data-difficulty='5'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}

	.item-time {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.empty-state {
		color: var(--ui-text-tertiary);
		font-style: italic;
		text-align: center;
		padding: var(--spacing-lg);
	}
</style>
