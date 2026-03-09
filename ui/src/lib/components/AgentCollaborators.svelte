<script lang="ts">
	/**
	 * AgentCollaborators — shows agents who have co-participated in parties
	 * with the given agent, ranked by frequency.
	 *
	 * In compact mode shows the top 10; otherwise shows the full list.
	 * Useful in sidebars or as a standalone section on the agent detail page.
	 */

	import { worldStore } from '$stores/worldStore.svelte';
	import { agentId as toAgentId, TrustTierNames } from '$types';

	let {
		agentId,
		compact = false
	}: {
		agentId: string;
		compact?: boolean;
	} = $props();

	interface CollaboratorEntry {
		agentId: string;
		name: string;
		tier: number;
		partyCount: number;
	}

	const allCollaborators = $derived.by((): CollaboratorEntry[] => {
		const counts = new Map<string, number>();

		for (const party of worldStore.partyList) {
			// Check if this agent is in the party
			const inParty = party.members.some((m) => {
				const memberId = String(m.agent_id);
				return memberId === agentId || memberId.includes(agentId) || agentId.includes(memberId);
			});
			if (!inParty) continue;

			// Count co-members
			for (const member of party.members) {
				const memberId = String(member.agent_id);
				if (memberId === agentId || memberId.includes(agentId) || agentId.includes(memberId)) {
					continue;
				}
				counts.set(memberId, (counts.get(memberId) ?? 0) + 1);
			}
		}

		const entries: CollaboratorEntry[] = [];
		for (const [id, count] of counts.entries()) {
			const agent = worldStore.agents.get(toAgentId(id));
			entries.push({
				agentId: id,
				name: agent?.name ?? id.split('.').pop() ?? id,
				tier: agent?.tier ?? 0,
				partyCount: count
			});
		}

		entries.sort((a, b) => b.partyCount - a.partyCount);
		return entries;
	});

	const compactLimit = 10;
	const collaborators = $derived(
		compact ? allCollaborators.slice(0, compactLimit) : allCollaborators
	);
	const truncatedCount = $derived(
		compact ? Math.max(0, allCollaborators.length - compactLimit) : 0
	);

	function tierName(tier: number): string {
		return TrustTierNames[tier as 0 | 1 | 2 | 3 | 4] ?? 'Unknown';
	}

</script>

<div class="collaborators">
	{#if collaborators.length > 0}
		<ul class="collaborator-list">
			{#each collaborators as entry (entry.agentId)}
				<li class="collaborator-item">
					<a href="/agents/{entry.agentId}" class="collaborator-link">
						<div class="collaborator-info">
							<span class="collaborator-name">{entry.name}</span>
							<span class="tier-badge" data-tier={entry.tier}>{tierName(entry.tier)}</span>
						</div>
						<span class="party-count">
							{entry.partyCount}
							{entry.partyCount === 1 ? 'party' : 'parties'}
						</span>
					</a>
				</li>
			{/each}
		</ul>
		{#if truncatedCount > 0}
			<p class="truncation-note">+{truncatedCount} more — see Collaborators tab</p>
		{/if}
	{:else}
		<p class="empty-state">No collaborators yet</p>
	{/if}
</div>

<style>
	.collaborators {
		display: flex;
		flex-direction: column;
	}

	.collaborator-list {
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

	.collaborator-item {
		background: var(--ui-surface-secondary);
	}

	.collaborator-link {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		text-decoration: none;
		color: var(--ui-text-primary);
		transition: background 150ms ease;
	}

	.collaborator-link:hover {
		background: var(--ui-surface-tertiary);
		text-decoration: none;
	}

	.collaborator-info {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		min-width: 0;
	}

	.collaborator-name {
		font-size: 0.875rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.tier-badge {
		padding: 1px 6px;
		border-radius: var(--radius-full);
		font-size: 0.625rem;
		font-weight: 600;
		text-transform: uppercase;
		flex-shrink: 0;
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
	}

	.tier-badge[data-tier='0'] {
		background: var(--tier-apprentice-container);
		color: var(--tier-apprentice);
	}

	.tier-badge[data-tier='1'] {
		background: var(--tier-journeyman-container);
		color: var(--tier-journeyman);
	}

	.tier-badge[data-tier='2'] {
		background: var(--tier-expert-container);
		color: var(--tier-expert);
	}

	.tier-badge[data-tier='3'] {
		background: var(--tier-master-container);
		color: var(--tier-master);
	}

	.tier-badge[data-tier='4'] {
		background: var(--tier-grandmaster-container);
		color: var(--tier-grandmaster);
	}

	.party-count {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	.truncation-note {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		text-align: center;
		padding: var(--spacing-xs) var(--spacing-md);
		margin: 0;
	}

	.empty-state {
		color: var(--ui-text-tertiary);
		font-style: italic;
		text-align: center;
		padding: var(--spacing-lg);
	}
</style>
