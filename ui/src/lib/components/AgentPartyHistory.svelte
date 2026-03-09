<script lang="ts">
	/**
	 * AgentPartyHistory — shows all parties an agent has participated in.
	 *
	 * Derives filtered party list from worldStore. Members use branded AgentID
	 * which are six-part dotted strings; substring matching handles cases where
	 * IDs are stored with different segment counts across subsystems.
	 */

	import { worldStore } from '$stores/worldStore.svelte';
	import { partyId as toPartyId, questId as toQuestId } from '$types';
	import type { PartyStatus } from '$types';
	import { formatDate } from '$lib/utils/format';

	let { agentId }: { agentId: string } = $props();

	const agentParties = $derived.by(() => {
		return worldStore.partyList
			.filter((p) =>
				p.members.some((m) => {
					const memberId = String(m.agent_id);
					return memberId === agentId || memberId.includes(agentId) || agentId.includes(memberId);
				})
			)
			.sort((a, b) => new Date(b.formed_at).getTime() - new Date(a.formed_at).getTime());
	});

	function agentRole(partyId: string): string {
		const party = worldStore.parties.get(toPartyId(partyId));
		if (!party) return 'member';
		const member = party.members.find((m) => {
			const memberId = String(m.agent_id);
			return memberId === agentId || memberId.includes(agentId) || agentId.includes(memberId);
		});
		return member?.role ?? 'member';
	}

	function resolveQuestTitle(questId: string): string {
		const quest = worldStore.quests.get(toQuestId(questId));
		return quest?.title ?? questId.split('.').pop() ?? questId;
	}

	function resolveTeammates(partyId: string): string[] {
		const party = worldStore.parties.get(toPartyId(partyId));
		if (!party) return [];
		return party.members
			.filter((m) => {
				const memberId = String(m.agent_id);
				return memberId !== agentId && !memberId.includes(agentId) && !agentId.includes(memberId);
			})
			.map((m) => worldStore.agentName(m.agent_id));
	}

	function extractInstance(id: string): string {
		return id.split('.').pop() ?? id;
	}

	function statusLabel(status: PartyStatus): string {
		return status.charAt(0).toUpperCase() + status.slice(1);
	}
</script>

<div class="party-history">
	{#if agentParties.length > 0}
		<ul class="history-list">
			{#each agentParties as party (party.id)}
				{@const teammates = resolveTeammates(party.id)}
				{@const role = agentRole(party.id)}
				<li class="history-item">
					<a href="/parties/{party.id}" class="item-link">
						<div class="item-main">
							<div class="item-header">
								<span class="item-title">{party.name || `Party #${extractInstance(party.id)}`}</span>
								<span class="status-badge" data-status={party.status}>
									{statusLabel(party.status)}
								</span>
							</div>
							<div class="item-meta">
								<span class="role-badge" data-role={role}>{role}</span>
								<span class="quest-ref">
									{resolveQuestTitle(party.quest_id)}
								</span>
							</div>
							{#if teammates.length > 0}
								<div class="teammates">
									<span class="teammates-label">with</span>
									{#each teammates as name, i}
										<span class="teammate-name">{name}{i < teammates.length - 1 ? ',' : ''}</span>
									{/each}
								</div>
							{/if}
						</div>
						<div class="item-dates">
							<span class="date-label">Formed</span>
							<span class="date-value">{formatDate(party.formed_at)}</span>
							{#if party.disbanded_at}
								<span class="date-label">Disbanded</span>
								<span class="date-value">{formatDate(party.disbanded_at)}</span>
							{/if}
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="empty-state">No parties yet</p>
	{/if}
</div>

<style>
	.party-history {
		display: flex;
		flex-direction: column;
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
		align-items: flex-start;
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
		gap: 3px;
	}

	.item-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.item-title {
		font-size: 0.875rem;
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.status-badge {
		font-size: 0.625rem;
		padding: 1px 6px;
		border-radius: var(--radius-full);
		font-weight: 600;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.status-badge[data-status='active'] {
		background: var(--status-success-container);
		color: var(--status-success);
	}

	.status-badge[data-status='forming'] {
		background: var(--status-warning-container);
		color: var(--status-warning);
	}

	.item-meta {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.role-badge {
		font-size: 0.625rem;
		padding: 1px 6px;
		border-radius: var(--radius-sm);
		text-transform: capitalize;
		font-weight: 600;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
	}

	.role-badge[data-role='lead'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}

	.quest-ref {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.teammates {
		display: flex;
		flex-wrap: wrap;
		gap: 3px;
		align-items: center;
	}

	.teammates-label {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
	}

	.teammate-name {
		font-size: 0.6875rem;
		color: var(--ui-text-secondary);
	}

	.item-dates {
		display: grid;
		grid-template-columns: auto auto;
		gap: 1px var(--spacing-sm);
		align-content: start;
		flex-shrink: 0;
	}

	.date-label {
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
	}

	.date-value {
		font-size: 0.6875rem;
		color: var(--ui-text-secondary);
		text-align: right;
	}

	.empty-state {
		color: var(--ui-text-tertiary);
		font-style: italic;
		text-align: center;
		padding: var(--spacing-lg);
	}
</style>
