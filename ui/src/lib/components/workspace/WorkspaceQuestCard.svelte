<script lang="ts">
	/**
	 * WorkspaceQuestCard - Per-quest card with isolated worldStore reactivity
	 *
	 * Each card independently subscribes to its own quest entity in worldStore,
	 * so status badge updates don't trigger re-renders of sibling cards or the
	 * parent tree structure. This prevents the reactive cascade that locks up
	 * the UI during heavy SSE traffic (e.g., E2E runs).
	 */

	import { worldStore } from '$stores/worldStore.svelte';
	import { extractInstance } from '$types';
	import type { WorkspaceQuest } from '$services/api';

	interface Props {
		quest: WorkspaceQuest;
		variant?: 'parent' | 'sub';
		onclick: () => void;
	}

	let { quest, variant = 'parent', onclick }: Props = $props();

	// Fine-grained reactive lookup: only this card re-renders when its quest changes.
	// worldStore.quests is keyed by full entity ID; workspace API uses instance IDs,
	// so we match by suffix. Each card does its own lookup — O(n) per card but only
	// re-evaluates when the quests map reference changes.
	const liveQuest = $derived.by(() => {
		// Try direct lookup first (works if quest_id happens to be a full entity ID)
		const direct = worldStore.quests.get(quest.quest_id as any);
		if (direct) return direct;
		// Fall back to instance ID match
		for (const [, q] of worldStore.quests) {
			if (extractInstance(q.id) === quest.quest_id) return q;
		}
		return null;
	});

	const title = $derived(
		quest.title || liveQuest?.title || quest.quest_id
	);

	const status = $derived(
		(liveQuest?.status as string) || quest.status
	);

	const agentDisplay = $derived.by(() => {
		if (quest.agent_name) return quest.agent_name;
		if (quest.agent) {
			return worldStore.agentName(quest.agent as any) || quest.agent;
		}
		return '';
	});
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="quest-card"
	class:sub-quest={variant === 'sub'}
	{onclick}
	data-testid="workspace-quest-{quest.quest_id}"
>
	<div class="quest-card-body">
		<div class="quest-card-header">
			<span class="quest-card-title">{title}</span>
			{#if status}
				<span class="status-badge" data-status={status}>{status}</span>
			{/if}
		</div>
		<div class="quest-card-meta">
			{#if agentDisplay}
				<span>{agentDisplay}</span>
			{/if}
			<span>{quest.file_count} files</span>
		</div>
	</div>
</div>

<style>
	.quest-card {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-md);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		background: var(--ui-surface-secondary);
		text-align: left;
		cursor: pointer;
		transition: border-color 150ms ease;
		width: 100%;
		font-family: inherit;
		color: var(--ui-text-primary);
	}

	.quest-card:hover {
		border-color: var(--ui-border-interactive);
	}

	.quest-card.sub-quest {
		padding-left: calc(var(--spacing-md) + 20px);
		border-radius: 0;
		border-top: none;
	}

	.quest-card.sub-quest:last-child {
		border-radius: 0 0 var(--radius-md) var(--radius-md);
	}

	.quest-card-body {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
		overflow: hidden;
	}

	.quest-card-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.quest-card-title {
		flex: 1;
		font-weight: 600;
		font-size: 0.875rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.quest-card-meta {
		display: flex;
		gap: var(--spacing-md);
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.status-badge {
		padding: 2px 8px;
		border-radius: var(--radius-full);
		font-size: 0.6875rem;
		font-weight: 500;
		flex-shrink: 0;
	}

	.status-badge[data-status='completed'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}
	.status-badge[data-status='failed'] {
		background: var(--quest-failed-container);
		color: var(--quest-failed);
	}
	.status-badge[data-status='in_review'] {
		background: var(--quest-in-review-container);
		color: var(--quest-in-review);
	}
	.status-badge[data-status='in_progress'] {
		background: var(--quest-in-progress-container);
		color: var(--quest-in-progress);
	}
	.status-badge[data-status='escalated'] {
		background: var(--quest-escalated-container);
		color: var(--quest-escalated);
	}
	.status-badge[data-status='pending_triage'] {
		background: var(--quest-pending-triage-container);
		color: var(--quest-pending-triage);
	}
	.status-badge[data-status='posted'] {
		background: var(--quest-posted-container, color-mix(in srgb, var(--quest-posted, #1976d2) 15%, transparent));
		color: var(--quest-posted, #1976d2);
	}
</style>
