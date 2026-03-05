<script lang="ts">
	/**
	 * ChatMessage - Single message bubble in the DM chat
	 *
	 * Role-based styling (user vs DM). Shows quest preview card when a
	 * quest_brief or quest_chain is attached to the message.
	 */

	import type { QuestBrief, QuestChainBrief } from '$lib/stores/chatStore.svelte';

	interface ChatMessageProps {
		role: 'user' | 'dm';
		content: string;
		mode?: string;
		questBrief?: QuestBrief | null;
		questChain?: QuestChainBrief | null;
		onPostQuest?: (brief: QuestBrief) => void;
		onPostChain?: (chain: QuestChainBrief) => void;
	}

	let { role, content, mode, questBrief, questChain, onPostQuest, onPostChain }: ChatMessageProps =
		$props();

	const QuestDifficultyNames: Record<number, string> = {
		0: 'Trivial',
		1: 'Easy',
		2: 'Moderate',
		3: 'Hard',
		4: 'Epic',
		5: 'Legendary'
	};
</script>

<div class="chat-message" data-role={role} data-testid="chat-message">
	<div class="message-header">
		<span class="message-role">{role === 'dm' ? 'DM' : 'You'}</span>
	</div>
	<div class="message-content">
		{content}
	</div>

	{#if mode === 'quest' && questBrief}
		<div class="quest-preview" data-testid="quest-preview">
			<div class="preview-header">Quest Brief</div>
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
			{#if onPostQuest}
				<button
					type="button"
					class="post-button"
					onclick={() => onPostQuest?.(questBrief!)}
					data-testid="post-quest-button"
				>
					Post Quest
				</button>
			{/if}
		</div>
	{/if}

	{#if mode === 'quest' && questChain}
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
			{#if onPostChain}
				<button
					type="button"
					class="post-button"
					onclick={() => onPostChain?.(questChain!)}
					data-testid="post-chain-button"
				>
					Post Chain
				</button>
			{/if}
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

	/* Post button */
	.post-button {
		margin-top: var(--spacing-sm);
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
</style>
