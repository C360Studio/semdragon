<script lang="ts">
	/**
	 * CapabilityRouting - Shows which LLM endpoints are mapped to each capability.
	 */
	import type { CapabilityInfo } from '$types';

	let {
		capabilities,
		defaultCapability
	}: {
		capabilities: Record<string, CapabilityInfo>;
		defaultCapability?: string;
	} = $props();

	const capEntries = $derived(Object.entries(capabilities));
</script>

<div class="capability-list" data-testid="capability-routing">
	{#each capEntries as [name, cap] (name)}
		<div class="capability-card" class:is-default={name === defaultCapability}>
			<div class="cap-header">
				<span class="cap-name">{name}</span>
				{#if name === defaultCapability}
					<span class="cap-default-badge">default</span>
				{/if}
				{#if cap.requires_tools}
					<span class="cap-tools-badge">tools required</span>
				{/if}
			</div>
			{#if cap.description}
				<p class="cap-description">{cap.description}</p>
			{/if}
			<div class="cap-chain">
				<span class="chain-label">Preferred</span>
				<div class="chain-items">
					{#each cap.preferred as ep, i}
						<span class="chain-ep">{ep}</span>
						{#if i < cap.preferred.length - 1}
							<span class="chain-arrow">&rarr;</span>
						{/if}
					{/each}
				</div>
			</div>
			{#if cap.fallback && cap.fallback.length > 0}
				<div class="cap-chain fallback">
					<span class="chain-label">Fallback</span>
					<div class="chain-items">
						{#each cap.fallback as ep, i}
							<span class="chain-ep">{ep}</span>
							{#if i < cap.fallback.length - 1}
								<span class="chain-arrow">&rarr;</span>
							{/if}
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{:else}
		<p class="empty-state">No capabilities configured</p>
	{/each}
</div>

<style>
	.capability-list {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.capability-card {
		padding: var(--spacing-md);
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
	}

	.capability-card.is-default {
		border-color: var(--ui-interactive-primary);
	}

	.cap-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		margin-bottom: var(--spacing-xs);
	}

	.cap-name {
		font-weight: 600;
		font-size: 0.875rem;
	}

	.cap-default-badge {
		font-size: 0.625rem;
		padding: 1px 5px;
		border-radius: var(--radius-full);
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
	}

	.cap-tools-badge {
		font-size: 0.625rem;
		padding: 1px 5px;
		border-radius: var(--radius-full);
		background: var(--status-info-container);
		color: var(--status-info);
	}

	.cap-description {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		margin: 0 0 var(--spacing-sm);
	}

	.cap-chain {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.cap-chain.fallback {
		margin-top: var(--spacing-xs);
	}

	.chain-label {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		min-width: 60px;
	}

	.chain-items {
		display: flex;
		align-items: center;
		gap: var(--spacing-xs);
		flex-wrap: wrap;
	}

	.chain-ep {
		font-size: 0.75rem;
		font-family: monospace;
		padding: 2px 6px;
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
	}

	.chain-arrow {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.empty-state {
		text-align: center;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-md);
	}
</style>
