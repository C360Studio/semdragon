<script lang="ts">
	/**
	 * AgentBattleHistory — shows all boss battles an agent has faced.
	 *
	 * BossBattle.agent_id is a branded AgentID (six-part dotted string).
	 * Uses substring matching so both full IDs and instance-only IDs resolve.
	 */

	import { worldStore } from '$stores/worldStore.svelte';
	import { questId as toQuestId } from '$types';
	import type { BattleStatus } from '$types';
	import { formatDate } from '$lib/utils/format';

	let { agentId }: { agentId: string } = $props();

	const agentBattles = $derived.by(() => {
		return worldStore.battleList
			.filter((b) => {
				const battleAgentId = String(b.agent_id);
				return (
					battleAgentId === agentId ||
					battleAgentId.includes(agentId) ||
					agentId.includes(battleAgentId)
				);
			})
			.sort((a, b) => {
				const aTime = new Date(a.created_at ?? 0).getTime();
				const bTime = new Date(b.created_at ?? 0).getTime();
				return bTime - aTime;
			});
	});

	const battleSummary = $derived.by(() => {
		const won = agentBattles.filter((b) => b.status === 'victory').length;
		const lost = agentBattles.filter((b) => b.status === 'defeat').length;
		const total = won + lost;
		return { won, lost, total, winRate: total > 0 ? Math.round((won / total) * 100) : 0 };
	});

	function resolveQuestTitle(questId: string): string {
		const quest = worldStore.quests.get(toQuestId(questId));
		return quest?.title ?? questId.split('.').pop() ?? questId;
	}

	function outcomeBadgeStatus(status: BattleStatus): 'victory' | 'defeat' | 'active' | 'other' {
		if (status === 'victory') return 'victory';
		if (status === 'defeat') return 'defeat';
		if (status === 'active') return 'active';
		return 'other';
	}
</script>

<div class="battle-history">
	{#if agentBattles.length > 0}
		<div class="summary-row">
			<span class="summary-chip victory">{battleSummary.won}W</span>
			<span class="summary-chip defeat">{battleSummary.lost}L</span>
			{#if battleSummary.total > 0}
				<span class="summary-chip rate">{battleSummary.winRate}% win rate</span>
			{/if}
		</div>

		<ul class="history-list">
			{#each agentBattles as battle (battle.id)}
				<li class="history-item">
					<a href="/battles/{battle.id}" class="item-link">
						<div class="outcome-indicator" data-outcome={outcomeBadgeStatus(battle.status)} aria-hidden="true"></div>
						<div class="item-main">
							<span class="item-title">{resolveQuestTitle(battle.quest_id)}</span>
							<span class="outcome-badge" data-outcome={outcomeBadgeStatus(battle.status)}>
								{battle.status}
							</span>
						</div>
						<span class="item-time">{formatDate(battle.created_at)}</span>
					</a>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="empty-state">No boss battles yet</p>
	{/if}
</div>

<style>
	.battle-history {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.summary-row {
		display: flex;
		gap: var(--spacing-sm);
		flex-wrap: wrap;
		align-items: center;
	}

	.summary-chip {
		padding: 2px 10px;
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		font-weight: 600;
	}

	.summary-chip.victory {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.summary-chip.defeat {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.summary-chip.rate {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
		font-weight: 400;
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

	.outcome-indicator {
		width: 4px;
		height: 32px;
		border-radius: 2px;
		flex-shrink: 0;
		background: var(--ui-border-subtle);
	}

	.outcome-indicator[data-outcome='victory'] {
		background: var(--status-success);
	}

	.outcome-indicator[data-outcome='defeat'] {
		background: var(--status-error);
	}

	.outcome-indicator[data-outcome='active'] {
		background: var(--status-warning);
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

	.outcome-badge {
		font-size: 0.625rem;
		padding: 1px 6px;
		border-radius: var(--radius-sm);
		text-transform: capitalize;
		font-weight: 600;
		width: fit-content;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
	}

	.outcome-badge[data-outcome='victory'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.outcome-badge[data-outcome='defeat'] {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.outcome-badge[data-outcome='active'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
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
