<script lang="ts">
	/**
	 * ContextChip - Small pill showing injected entity context
	 *
	 * Displays entity type icon + label with a remove button.
	 */

	interface ContextChipProps {
		type: 'agent' | 'quest' | 'battle' | 'guild';
		label: string;
		onRemove?: () => void;
	}

	let { type, label, onRemove }: ContextChipProps = $props();

	const typeIcons: Record<string, string> = {
		agent: 'A',
		quest: 'Q',
		battle: 'B',
		guild: 'G'
	};
</script>

<span class="context-chip" data-type={type} data-testid="context-chip">
	<span class="chip-icon">{typeIcons[type] ?? '?'}</span>
	<span class="chip-label">{label}</span>
	{#if onRemove}
		<button
			type="button"
			class="chip-remove"
			aria-label="Remove {label} from context"
			onclick={onRemove}
		>
			&times;
		</button>
	{/if}
</span>

<style>
	.context-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		background: var(--ui-surface-tertiary);
		border: 1px solid var(--ui-border-subtle);
		color: var(--ui-text-secondary);
	}

	.chip-icon {
		width: 16px;
		height: 16px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-sm);
		font-weight: 600;
		font-size: 0.625rem;
		background: var(--ui-surface-secondary);
	}

	.context-chip[data-type='agent'] .chip-icon {
		background: var(--status-success-container, #e8f5e9);
		color: var(--status-success, #388e3c);
	}

	.context-chip[data-type='quest'] .chip-icon {
		background: var(--quest-posted-container, #e3f2fd);
		color: var(--quest-posted, #1976d2);
	}

	.context-chip[data-type='battle'] .chip-icon {
		background: var(--status-warning-container, #fff3e0);
		color: var(--status-warning, #f57c00);
	}

	.context-chip[data-type='guild'] .chip-icon {
		background: var(--tier-master-container, #f3e5f5);
		color: var(--tier-master, #7b1fa2);
	}

	.chip-label {
		max-width: 120px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.chip-remove {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		height: 16px;
		border: none;
		background: none;
		padding: 0;
		cursor: pointer;
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
		line-height: 1;
		border-radius: 50%;
		transition: background-color 150ms ease;
	}

	.chip-remove:hover {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
	}
</style>
