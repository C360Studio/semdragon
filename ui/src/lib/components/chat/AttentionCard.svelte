<script lang="ts">
	/**
	 * AttentionCard - Inline action card for quest escalations and triage decisions
	 *
	 * Rendered inside ChatMessage when a system message has an attentionCard.
	 * Two variants:
	 *   - escalation: text input for DM's clarification answer
	 *   - triage: buttons for salvage/tpk/terminal with analysis input
	 */

	import type { AttentionCard } from '$lib/stores/chatStore.svelte';

	interface AttentionCardProps {
		card: AttentionCard;
		onRespondEscalation?: (questId: string, answer: string) => void;
		onSubmitTriage?: (questId: string, decision: { path: 'salvage' | 'tpk' | 'escalate' | 'terminal'; analysis: string; anti_patterns?: string[] }) => void;
		loading?: boolean;
	}

	let { card, onRespondEscalation, onSubmitTriage, loading = false }: AttentionCardProps = $props();

	let escalationAnswer = $state('');
	let triageAnalysis = $state('');
	let selectedPath = $state<'salvage' | 'tpk' | 'terminal' | null>(null);

	function handleEscalationSubmit() {
		if (!escalationAnswer.trim() || !onRespondEscalation) return;
		onRespondEscalation(card.questId, escalationAnswer.trim());
	}

	function handleTriageSubmit() {
		if (!selectedPath || !onSubmitTriage) return;
		onSubmitTriage(card.questId, {
			path: selectedPath,
			analysis: triageAnalysis.trim() || `DM chose ${selectedPath}`
		});
	}

	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			if (card.type === 'escalation') handleEscalationSubmit();
		}
	}

	const attemptLabel = $derived(
		card.attempts && card.maxAttempts ? `Attempt ${card.attempts}/${card.maxAttempts}` : null
	);
</script>

<div class="attention-card" data-type={card.type} data-resolved={card.resolved} data-testid="attention-card-{card.type}">
	{#if card.type === 'escalation'}
		<div class="card-header escalation">Quest Needs Clarification</div>

		{#if card.agentName}
			<div class="card-agent">Agent: {card.agentName}</div>
		{/if}

		{#if card.question}
			<div class="card-question">{card.question}</div>
		{/if}

		{#if card.resolved}
			<div class="card-resolved">
				<span class="resolved-label">Answered:</span>
				<span class="resolved-value">{card.resolvedAnswer ?? 'Clarification sent'}</span>
			</div>
		{:else}
			<div class="card-input-group">
				<input
					type="text"
					class="card-input"
					placeholder="Type your answer..."
					bind:value={escalationAnswer}
					onkeydown={handleKeyDown}
					disabled={loading}
					data-testid="escalation-input"
				/>
				<button
					class="card-submit"
					onclick={handleEscalationSubmit}
					disabled={!escalationAnswer.trim() || loading}
					data-testid="escalation-submit"
				>
					{loading ? '...' : 'Send Answer'}
				</button>
			</div>
		{/if}
	{:else if card.type === 'triage'}
		<div class="card-header triage">Quest Needs Triage</div>

		{#if card.agentName}
			<div class="card-agent">Agent: {card.agentName}</div>
		{/if}

		{#if attemptLabel}
			<div class="card-meta">{attemptLabel}</div>
		{/if}

		{#if card.failureType}
			<div class="card-meta">Failure: {card.failureType}</div>
		{/if}

		{#if card.failureReason}
			<div class="card-reason">{card.failureReason}</div>
		{/if}

		{#if card.resolved}
			<div class="card-resolved">
				<span class="resolved-label">Decision:</span>
				<span class="resolved-value">{card.resolvedPath ?? 'Triaged'}</span>
			</div>
		{:else}
			<div class="triage-buttons">
				<button
					class="triage-btn salvage"
					class:selected={selectedPath === 'salvage'}
					onclick={() => selectedPath = selectedPath === 'salvage' ? null : 'salvage'}
					disabled={loading}
					title="Preserve partial work and retry with enriched context"
					data-testid="triage-salvage"
				>Salvage</button>
				<button
					class="triage-btn tpk"
					class:selected={selectedPath === 'tpk'}
					onclick={() => selectedPath = selectedPath === 'tpk' ? null : 'tpk'}
					disabled={loading}
					title="Clear output, add anti-pattern warnings, retry"
					data-testid="triage-tpk"
				>TPK</button>
				<button
					class="triage-btn terminal"
					class:selected={selectedPath === 'terminal'}
					onclick={() => selectedPath = selectedPath === 'terminal' ? null : 'terminal'}
					disabled={loading}
					title="Mark quest as permanently failed"
					data-testid="triage-terminal"
				>Terminal</button>
			</div>

			{#if selectedPath}
				<div class="card-input-group">
					<input
						type="text"
						class="card-input"
						placeholder="Analysis (optional)..."
						bind:value={triageAnalysis}
						disabled={loading}
						data-testid="triage-analysis"
					/>
					<button
						class="card-submit"
						onclick={handleTriageSubmit}
						disabled={loading}
						data-testid="triage-submit"
					>
						{loading ? '...' : `Apply ${selectedPath}`}
					</button>
				</div>
			{/if}
		{/if}
	{/if}
</div>

<style>
	.attention-card {
		margin-top: var(--spacing-sm);
		padding: var(--spacing-sm);
		border-radius: var(--radius-md);
		border: 1px solid var(--ui-border-subtle);
		border-left: 3px solid var(--ui-border-subtle);
	}

	.attention-card[data-type='escalation'] {
		border-left-color: var(--quest-escalated);
		background: color-mix(in srgb, var(--quest-escalated-container) 30%, var(--ui-surface-secondary));
	}

	.attention-card[data-type='triage'] {
		border-left-color: var(--status-warning, #ff832b);
		background: color-mix(in srgb, var(--quest-failed-container) 30%, var(--ui-surface-secondary));
	}

	.attention-card[data-resolved='true'] {
		opacity: 0.6;
	}

	.card-header {
		font-size: 0.625rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-bottom: var(--spacing-xs);
	}

	.card-header.escalation {
		color: var(--quest-escalated);
	}

	.card-header.triage {
		color: var(--status-warning, #ff832b);
	}

	.card-agent {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		margin-bottom: 2px;
	}

	.card-meta {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
	}

	.card-question,
	.card-reason {
		font-size: 0.8125rem;
		line-height: 1.4;
		margin: var(--spacing-xs) 0;
		padding: var(--spacing-xs);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
		white-space: pre-wrap;
		word-break: break-word;
	}

	.card-resolved {
		font-size: 0.75rem;
		margin-top: var(--spacing-xs);
		padding: var(--spacing-xs);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
	}

	.resolved-label {
		font-weight: 600;
		color: var(--ui-text-tertiary);
	}

	.resolved-value {
		color: var(--ui-text-secondary);
		margin-left: 4px;
	}

	.card-input-group {
		display: flex;
		gap: var(--spacing-xs);
		margin-top: var(--spacing-xs);
	}

	.card-input {
		flex: 1;
		padding: 4px var(--spacing-sm);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		font-size: 0.8125rem;
		font-family: inherit;
	}

	.card-input:focus {
		outline: none;
		border-color: var(--ui-border-interactive);
	}

	.card-submit {
		padding: 4px var(--spacing-md);
		border: none;
		border-radius: var(--radius-sm);
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
		white-space: nowrap;
		transition: opacity 150ms ease;
	}

	.card-submit:hover:not(:disabled) {
		opacity: 0.9;
	}

	.card-submit:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.triage-buttons {
		display: flex;
		gap: var(--spacing-xs);
		margin-top: var(--spacing-xs);
	}

	.triage-btn {
		padding: 4px var(--spacing-sm);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--ui-text-secondary);
		font-size: 0.6875rem;
		font-weight: 600;
		cursor: pointer;
		transition: all 150ms ease;
	}

	.triage-btn:hover:not(:disabled) {
		border-color: var(--ui-border-interactive);
		color: var(--ui-text-primary);
	}

	.triage-btn.selected {
		color: var(--ui-text-on-primary);
	}

	.triage-btn.salvage.selected {
		background: var(--quest-posted, #1976d2);
		border-color: var(--quest-posted, #1976d2);
	}

	.triage-btn.tpk.selected {
		background: var(--status-warning, #ff832b);
		border-color: var(--status-warning, #ff832b);
	}

	.triage-btn.terminal.selected {
		background: var(--quest-failed);
		border-color: var(--quest-failed);
	}

	.triage-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
