<script lang="ts">
	/**
	 * ActivityFeed — shows recent events related to a specific entity.
	 * Filters worldStore.recentEvents by matching entity IDs.
	 */

	import { worldStore } from '$stores/worldStore.svelte';
	import type { AgentID, QuestID, GuildID, PartyID, BattleID, GameEvent, GameEventType } from '$types';

	let {
		agentId = undefined,
		questId = undefined,
		guildId = undefined,
		partyId = undefined,
		battleId = undefined,
		limit = 20
	}: {
		agentId?: AgentID;
		questId?: QuestID;
		guildId?: GuildID;
		partyId?: PartyID;
		battleId?: BattleID;
		limit?: number;
	} = $props();

	const events = $derived.by(() => {
		return worldStore.recentEvents
			.filter((e) => {
				if (agentId && e.agent_id === agentId) return true;
				if (questId && e.quest_id === questId) return true;
				if (guildId && e.guild_id === guildId) return true;
				if (partyId && e.party_id === partyId) return true;
				if (battleId && e.battle_id === battleId) return true;
				return false;
			})
			.slice(0, limit);
	});

	function eventLabel(type: GameEventType): string {
		const parts = type.split('.');
		return parts.slice(-2).join(' ').replaceAll('_', ' ');
	}

	function eventIcon(type: GameEventType): string {
		if (type.startsWith('quest.')) return 'Q';
		if (type.startsWith('agent.')) return 'A';
		if (type.startsWith('battle.')) return 'B';
		if (type.startsWith('guild.') || type.startsWith('party.')) return 'G';
		if (type.startsWith('store.')) return 'S';
		if (type.startsWith('dm.')) return 'D';
		if (type.startsWith('review.')) return 'R';
		return '*';
	}

	function formatTime(timestamp: number): string {
		const now = Date.now();
		const diff = now - timestamp;
		if (diff < 60000) return 'just now';
		if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
		if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
		return new Date(timestamp).toLocaleDateString();
	}

	function eventDetail(event: GameEvent): string {
		if (event.agent_id && event.agent_id !== agentId) {
			return worldStore.agentName(event.agent_id);
		}
		if (event.quest_id && event.quest_id !== questId) {
			const quest = worldStore.quests.get(event.quest_id);
			return quest?.title ?? '';
		}
		return '';
	}
</script>

<section class="activity-feed">
	<h2>Recent Activity</h2>
	<ul class="activity-list">
		{#each events as event}
			<li class="activity-item">
				<span class="activity-icon" data-category={eventIcon(event.type)}>{eventIcon(event.type)}</span>
				<div class="activity-body">
					<span class="activity-label">{eventLabel(event.type)}</span>
					{#if eventDetail(event)}
						<span class="activity-detail">{eventDetail(event)}</span>
					{/if}
				</div>
				<span class="activity-time">{formatTime(event.timestamp)}</span>
			</li>
		{:else}
			<li class="activity-empty">No recent activity</li>
		{/each}
	</ul>
</section>

<style>
	.activity-feed {
		margin-top: var(--spacing-md);
	}

	.activity-feed h2 {
		font-size: 0.875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin: 0 0 var(--spacing-md);
	}

	.activity-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}

	.activity-item {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-secondary);
	}

	.activity-icon {
		width: 20px;
		height: 20px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-sm);
		font-size: 0.625rem;
		font-weight: 700;
		flex-shrink: 0;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
	}

	.activity-icon[data-category='Q'] {
		background: var(--tier-journeyman-container);
		color: var(--tier-journeyman);
	}
	.activity-icon[data-category='A'] {
		background: var(--tier-expert-container);
		color: var(--tier-expert);
	}
	.activity-icon[data-category='B'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}
	.activity-icon[data-category='R'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.activity-body {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}

	.activity-label {
		font-size: 0.8125rem;
		text-transform: capitalize;
	}

	.activity-detail {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.activity-time {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.activity-empty {
		padding: var(--spacing-md);
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
		font-style: italic;
		text-align: center;
	}
</style>
