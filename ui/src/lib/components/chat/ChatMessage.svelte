<script lang="ts">
	/**
	 * ChatMessage - Single message bubble in the DM chat
	 *
	 * Role-based styling (user vs DM). Shows quest preview card when a
	 * quest_brief or quest_chain is attached to the message.
	 */

	import type { QuestBrief, QuestChainBrief, QuestScenario } from '$lib/stores/chatStore.svelte';

	interface ChatMessageProps {
		role: 'user' | 'dm';
		content: string;
		questBrief?: QuestBrief | null;
		questChain?: QuestChainBrief | null;
		onPostQuest?: (brief: QuestBrief) => void;
		onPostChain?: (chain: QuestChainBrief) => void;
		onEditQuest?: (brief: QuestBrief) => void;
		onEditChain?: (chain: QuestChainBrief) => void;
		onDismissQuest?: () => void;
		onDismissChain?: () => void;
	}

	let {
		role,
		content,
		questBrief,
		questChain,
		onPostQuest,
		onPostChain,
		onEditQuest,
		onEditChain,
		onDismissQuest,
		onDismissChain
	}: ChatMessageProps = $props();

	const QuestDifficultyNames: Record<number, string> = {
		0: 'Trivial',
		1: 'Easy',
		2: 'Moderate',
		3: 'Hard',
		4: 'Epic',
		5: 'Legendary'
	};

	/**
	 * Classify quest staffing from scenarios — mirrors domain.ClassifyDecomposability().
	 * Returns true if the quest should be a party quest.
	 */
	function isPartyQuest(brief: QuestBrief): boolean {
		// Manual override
		if (brief.hints?.party_required) return true;

		const scenarios = brief.scenarios;
		// 0-1 scenarios → solo (trivial)
		if (!scenarios || scenarios.length <= 1) return false;

		// Count roots (scenarios with no dependencies)
		const roots = scenarios.filter(
			(s: QuestScenario) => !s.depends_on || s.depends_on.length === 0
		);

		// All scenarios are roots (no dependencies at all) → parallel → party
		if (roots.length === scenarios.length) return true;

		// Check if it's a linear chain (sequential → solo)
		// A linear chain: exactly 1 root, and each non-root depends on exactly 1 other
		if (roots.length === 1) {
			const allSingleDep = scenarios.every(
				(s: QuestScenario) => !s.depends_on || s.depends_on.length <= 1
			);
			if (allSingleDep) return false; // sequential → solo
		}

		// Mixed dependencies → party
		return true;
	}

	let questIsParty = $derived(questBrief ? isPartyQuest(questBrief) : false);
</script>

<div class="chat-message" data-role={role} data-testid="chat-message">
	<div class="message-header">
		<span class="message-role">{role === 'dm' ? 'DM' : 'You'}</span>
	</div>
	<div class="message-content">
		{content}
	</div>

	{#if questBrief}
		<div class="quest-preview" data-testid="quest-preview">
			<div class="preview-header">{questIsParty ? 'Party' : 'Solo'} Quest Brief</div>
			<div class="preview-title">{questBrief.title}</div>
			{#if questBrief.description}
				<div class="preview-desc">{questBrief.description}</div>
			{/if}
			<div class="preview-meta">
				{#if questBrief.difficulty != null}
					<span class="meta-tag">
						{QuestDifficultyNames[questBrief.difficulty] ?? `Difficulty ${questBrief.difficulty}`}
					</span>
				{/if}
				{#if questBrief.hints?.party_required}
					<span class="meta-tag party" data-testid="party-badge">
						Party ({questBrief.hints?.min_party_size ?? 2}+)
					</span>
				{/if}
				{#if questBrief.skills?.length}
					{#each questBrief.skills as skill}
						<span class="meta-tag skill">{skill}</span>
					{/each}
				{/if}
			</div>
			{#if questBrief.acceptance?.length}
				<ul class="preview-acceptance">
					{#each questBrief.acceptance as criterion}
						<li>{criterion}</li>
					{/each}
				</ul>
			{/if}
			<div class="card-actions">
				{#if onPostQuest}
					<button
						type="button"
						class="post-button"
						onclick={() => onPostQuest?.(questBrief!)}
						data-testid="post-quest-button"
					>
						{questIsParty ? 'Post Party Quest' : 'Post Solo Quest'}
					</button>
				{/if}
				{#if onEditQuest}
					<button
						type="button"
						class="edit-button"
						onclick={() => onEditQuest?.(questBrief!)}
						data-testid="edit-quest-button"
					>
						Edit
					</button>
				{/if}
				{#if onDismissQuest}
					<button
						type="button"
						class="dismiss-button"
						onclick={() => onDismissQuest?.()}
						title="Dismiss"
						data-testid="dismiss-quest-button"
					>
						&times;
					</button>
				{/if}
			</div>
		</div>
	{/if}

	{#if questChain}
		<div class="quest-preview chain" data-testid="quest-chain-preview">
			<div class="preview-header">Quest Chain ({questChain.quests.length} quests)</div>
			{#each questChain.quests as entry, i}
				<div class="chain-entry">
					<span class="chain-index">#{i + 1}</span>
					<span class="chain-title">{entry.title}</span>
					{#if entry.depends_on?.length}
						<span class="chain-deps">
							depends on #{entry.depends_on.map((d) => d + 1).join(', #')}
						</span>
					{/if}
				</div>
			{/each}
			<div class="card-actions">
				{#if onPostChain}
					<button
						type="button"
						class="post-button"
						onclick={() => onPostChain?.(questChain!)}
						data-testid="post-chain-button"
					>
						Post Quest Chain
					</button>
				{/if}
				{#if onEditChain}
					<button
						type="button"
						class="edit-button"
						onclick={() => onEditChain?.(questChain!)}
						data-testid="edit-chain-button"
					>
						Edit
					</button>
				{/if}
				{#if onDismissChain}
					<button
						type="button"
						class="dismiss-button"
						onclick={() => onDismissChain?.()}
						title="Dismiss"
						data-testid="dismiss-chain-button"
					>
						&times;
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.chat-message {
		padding: var(--spacing-sm) var(--spacing-md);
		border-radius: var(--radius-md);
		max-width: 85%;
	}

	.chat-message[data-role='user'] {
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		align-self: flex-end;
		margin-left: auto;
	}

	.chat-message[data-role='dm'] {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		align-self: flex-start;
	}

	.message-header {
		font-size: 0.625rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		opacity: 0.7;
		margin-bottom: 2px;
	}

	.message-content {
		font-size: 0.875rem;
		line-height: 1.5;
		white-space: pre-wrap;
		word-break: break-word;
	}

	/* Quest Preview */
	.quest-preview {
		margin-top: var(--spacing-sm);
		padding: var(--spacing-sm);
		background: var(--ui-surface-tertiary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
	}

	.preview-header {
		font-size: 0.625rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		margin-bottom: var(--spacing-xs);
	}

	.preview-title {
		font-weight: 600;
		font-size: 0.875rem;
		margin-bottom: var(--spacing-xs);
	}

	.preview-desc {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		margin-bottom: var(--spacing-xs);
	}

	.preview-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		margin-bottom: var(--spacing-xs);
	}

	.meta-tag {
		font-size: 0.625rem;
		padding: 1px 6px;
		border-radius: var(--radius-full);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
	}

	.meta-tag.skill {
		background: var(--quest-posted-container, #e3f2fd);
		color: var(--quest-posted, #1976d2);
	}

	.meta-tag.party {
		background: var(--quest-in_progress-container, #fff3e0);
		color: var(--quest-in_progress, #e65100);
	}

	.preview-acceptance {
		font-size: 0.75rem;
		padding-left: 1.2em;
		margin: 0;
		color: var(--ui-text-secondary);
	}

	/* Chain entries */
	.chain-entry {
		display: flex;
		align-items: center;
		gap: var(--spacing-xs);
		font-size: 0.75rem;
		padding: 2px 0;
	}

	.chain-index {
		font-weight: 600;
		color: var(--ui-text-tertiary);
		min-width: 20px;
	}

	.chain-title {
		flex: 1;
	}

	.chain-deps {
		font-size: 0.625rem;
		color: var(--ui-text-tertiary);
	}

	/* Card action buttons */
	.card-actions {
		display: flex;
		align-items: center;
		gap: var(--spacing-xs);
		margin-top: var(--spacing-sm);
	}

	.post-button {
		padding: var(--spacing-xs) var(--spacing-md);
		border: none;
		border-radius: var(--radius-md);
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
		transition: opacity 150ms ease;
	}

	.post-button:hover {
		opacity: 0.9;
	}

	.edit-button {
		padding: var(--spacing-xs) var(--spacing-sm);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		background: transparent;
		color: var(--ui-text-secondary);
		font-size: 0.75rem;
		cursor: pointer;
		transition: all 150ms ease;
	}

	.edit-button:hover {
		color: var(--ui-text-primary);
		border-color: var(--ui-text-secondary);
	}

	.dismiss-button {
		padding: var(--spacing-xs) var(--spacing-xs);
		border: none;
		border-radius: var(--radius-md);
		background: transparent;
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
		line-height: 1;
		cursor: pointer;
		transition: color 150ms ease;
		margin-left: auto;
	}

	.dismiss-button:hover {
		color: var(--ui-text-primary);
	}
</style>
