<script lang="ts">
	/**
	 * ChatPanel - Bottom-docked DM chat panel
	 *
	 * Collapsed: thin bar with "Ask the DM..." text
	 * Expanded: scrollable messages, context chips, input area
	 * Resizable vertically via VerticalResizeHandle
	 */

	import { chatStore } from '$lib/stores/chatStore.svelte';
	import type { QuestBrief, QuestChainBrief } from '$lib/stores/chatStore.svelte';
	import { pageContext } from '$lib/stores/pageContext.svelte';
	import ChatMessageComponent from './ChatMessage.svelte';
	import ContextChip from './ContextChip.svelte';
	import VerticalResizeHandle from './VerticalResizeHandle.svelte';

	let input = $state('');
	let messagesContainer: HTMLElement | undefined = $state();

	// Auto-scroll to bottom when new messages arrive
	$effect(() => {
		// Touch messages array to trigger on changes
		const _ = chatStore.messages.length;
		if (messagesContainer) {
			// Use setTimeout to ensure DOM has updated
			setTimeout(() => {
				if (messagesContainer) {
					messagesContainer.scrollTop = messagesContainer.scrollHeight;
				}
			}, 0);
		}
	});

	function handleSend() {
		if (!input.trim() || chatStore.loading) return;
		const text = input;
		input = '';
		chatStore.sendMessage(text);
	}

	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Enter' && !event.shiftKey) {
			event.preventDefault();
			handleSend();
		}
	}

	async function handlePostQuest(brief: QuestBrief) {
		const quest = await chatStore.postQuest(brief);
		if (quest) {
			chatStore.sendMessage(`Quest "${brief.title}" posted successfully!`);
		}
	}

	async function handlePostChain(chain: QuestChainBrief) {
		const quests = await chatStore.postChain(chain);
		if (quests) {
			chatStore.sendMessage(
				`Quest chain posted: ${quests.length} quests created.`
			);
		}
	}
</script>

<div class="chat-panel" class:open={chatStore.open} data-testid="chat-panel">
	{#if chatStore.open}
		<VerticalResizeHandle
			onResize={(delta) => chatStore.setHeight(chatStore.height + delta)}
		/>
	{/if}

	<!-- Page context strip (shown even when collapsed) -->
	{#if !chatStore.open && pageContext.items.length > 0}
		<div class="page-context-strip" data-testid="page-context-strip">
			{#each pageContext.items as item}
				<ContextChip
					type={item.type}
					label={item.label}
					variant="page"
					onPin={() => chatStore.addContext({ type: item.type, id: item.id, label: item.label })}
				/>
			{/each}
		</div>
	{/if}

	<!-- Collapsed bar / Header -->
	<button
		type="button"
		class="chat-header"
		onclick={() => chatStore.toggle()}
		data-testid="chat-toggle"
	>
		<span class="header-icon">{chatStore.open ? 'v' : '^'}</span>
		<span class="header-text">Ask the DM...</span>
		{#if chatStore.messages.length > 0}
			<span class="header-count">{chatStore.messages.length}</span>
		{/if}
	</button>

	{#if chatStore.open}
		<div class="chat-body" style="height: {chatStore.height}px">
			<!-- Messages -->
			<div class="messages-scroll" bind:this={messagesContainer}>
				{#each chatStore.messages as msg}
					<ChatMessageComponent
						role={msg.role}
						content={msg.content}
						questBrief={msg.questBrief}
						questChain={msg.questChain}
						onPostQuest={handlePostQuest}
						onPostChain={handlePostChain}
					/>
				{:else}
					<div class="empty-chat">
						Start a conversation with the Dungeon Master to create quests.
					</div>
				{/each}

				{#if chatStore.loading}
					<div class="loading-indicator" data-testid="chat-loading">
						DM is thinking...
					</div>
				{/if}

				{#if chatStore.error}
					<div class="error-message" data-testid="chat-error">
						{chatStore.error}
					</div>
				{/if}
			</div>

			<!-- Page context chips -->
			{#if pageContext.items.length > 0}
				<div class="page-context-bar" data-testid="page-context-bar">
					{#each pageContext.items as item}
						<ContextChip
							type={item.type}
							label={item.label}
							variant="page"
							onPin={() => chatStore.addContext({ type: item.type, id: item.id, label: item.label })}
						/>
					{/each}
				</div>
			{/if}

			<!-- Pinned context chips -->
			{#if chatStore.contextItems.length > 0}
				<div class="context-bar" data-testid="context-bar">
					{#each chatStore.contextItems as item}
						<ContextChip
							type={item.type}
							label={item.label}
							onRemove={() => chatStore.removeContext(item.id)}
						/>
					{/each}
				</div>
			{/if}

			<!-- Input area -->
			<div class="input-area">
				<textarea
					class="chat-input"
					placeholder="Type a message..."
					bind:value={input}
					onkeydown={handleKeyDown}
					disabled={chatStore.loading}
					rows={1}
					data-testid="chat-input"
				></textarea>
				<button
					type="button"
					class="send-button"
					onclick={handleSend}
					disabled={!input.trim() || chatStore.loading}
					data-testid="chat-send"
				>
					Send
				</button>
			</div>
		</div>
	{/if}
</div>

<style>
	.chat-panel {
		flex-shrink: 0;
		border-top: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-primary);
		display: flex;
		flex-direction: column;
	}

	/* Header / collapsed bar */
	.chat-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-xs) var(--spacing-md);
		border: none;
		background: var(--ui-surface-tertiary);
		cursor: pointer;
		width: 100%;
		text-align: left;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		transition: background-color 150ms ease;
		min-height: 36px;
	}

	.chat-header:hover {
		background: var(--ui-surface-secondary);
	}

	.header-icon {
		font-size: 0.75rem;
		opacity: 0.6;
	}

	.header-text {
		flex: 1;
	}

	.header-count {
		font-size: 0.625rem;
		padding: 1px 6px;
		border-radius: var(--radius-full);
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
	}

	/* Chat body */
	.chat-body {
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.messages-scroll {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-sm);
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.empty-chat {
		text-align: center;
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
		padding: var(--spacing-lg);
	}

	.loading-indicator {
		text-align: center;
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
		font-style: italic;
		padding: var(--spacing-sm);
	}

	.error-message {
		text-align: center;
		color: var(--status-error);
		font-size: 0.75rem;
		padding: var(--spacing-sm);
		background: var(--status-error-container);
		border-radius: var(--radius-md);
	}

	/* Page context bars */
	.page-context-strip {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		padding: 4px var(--spacing-sm);
		background: var(--ui-surface-primary);
		border-top: 1px solid var(--ui-border-subtle);
	}

	.page-context-bar {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		padding: var(--spacing-xs) var(--spacing-sm);
		border-top: 1px solid var(--ui-border-subtle);
	}

	/* Context bar */
	.context-bar {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		padding: var(--spacing-xs) var(--spacing-sm);
		border-top: 1px solid var(--ui-border-subtle);
	}

	/* Input area */
	.input-area {
		display: flex;
		align-items: flex-end;
		gap: var(--spacing-xs);
		padding: var(--spacing-sm);
		border-top: 1px solid var(--ui-border-subtle);
	}

	.chat-input {
		flex: 1;
		padding: var(--spacing-xs) var(--spacing-sm);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
		font-size: 0.875rem;
		font-family: inherit;
		resize: none;
		min-height: 36px;
		max-height: 100px;
	}

	.chat-input:focus {
		outline: 2px solid var(--ui-interactive-primary);
		outline-offset: -1px;
	}

	.chat-input:disabled {
		opacity: 0.5;
	}

	.send-button {
		padding: var(--spacing-xs) var(--spacing-md);
		border: none;
		border-radius: var(--radius-md);
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
		min-height: 36px;
		transition: opacity 150ms ease;
	}

	.send-button:hover:not(:disabled) {
		opacity: 0.9;
	}

	.send-button:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
</style>
