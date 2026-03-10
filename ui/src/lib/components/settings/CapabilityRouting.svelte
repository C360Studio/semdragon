<script lang="ts">
	/**
	 * CapabilityRouting - Shows and edits which LLM endpoints are mapped to each capability.
	 */
	import type { CapabilityInfo } from '$types';
	import type { CapabilityUpdate } from '$services/api';

	let {
		capabilities,
		defaultCapability,
		endpointNames = [],
		onSave,
		saving = false
	}: {
		capabilities: Record<string, CapabilityInfo>;
		defaultCapability?: string;
		endpointNames?: string[];
		onSave?: (name: string, update: CapabilityUpdate) => Promise<void>;
		saving?: boolean;
	} = $props();

	const capEntries = $derived(Object.entries(capabilities));

	// Editing state
	let editingName = $state<string | null>(null);
	let draftPreferred = $state<string[]>([]);
	let draftFallback = $state<string[]>([]);

	function startEditing(name: string, cap: CapabilityInfo) {
		editingName = name;
		draftPreferred = [...cap.preferred];
		draftFallback = [...(cap.fallback ?? [])];
	}

	function cancelEditing() {
		editingName = null;
	}

	async function saveEdit() {
		if (!onSave || !editingName || saving) return;
		await onSave(editingName, {
			preferred: draftPreferred,
			fallback: draftFallback
		});
		editingName = null;
	}

	function moveUp(list: string[], index: number): string[] {
		if (index <= 0) return list;
		const copy = [...list];
		[copy[index - 1], copy[index]] = [copy[index], copy[index - 1]];
		return copy;
	}

	function moveDown(list: string[], index: number): string[] {
		if (index >= list.length - 1) return list;
		const copy = [...list];
		[copy[index], copy[index + 1]] = [copy[index + 1], copy[index]];
		return copy;
	}

	function removeFrom(list: string[], index: number): string[] {
		return list.filter((_, i) => i !== index);
	}

	// Available endpoints not already in the chain
	const availableEndpoints = $derived(() => {
		const used = new Set([...draftPreferred, ...draftFallback]);
		return endpointNames.filter(n => !used.has(n));
	});
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
				{#if onSave && editingName !== name}
					<button class="cap-edit-btn" onclick={() => startEditing(name, cap)} disabled={saving}>Edit</button>
				{/if}
			</div>
			{#if cap.description}
				<p class="cap-description">{cap.description}</p>
			{/if}

			{#if editingName === name}
				<!-- Editing mode -->
				<div class="chain-editor">
					<div class="chain-section">
						<span class="chain-label">Preferred</span>
						<div class="chain-edit-items">
							{#each draftPreferred as ep, i (ep + i)}
								<div class="chain-edit-item">
									<span class="chain-ep">{ep}</span>
									<button class="chain-btn" onclick={() => (draftPreferred = moveUp(draftPreferred, i))} disabled={i === 0} title="Move up">&uarr;</button>
									<button class="chain-btn" onclick={() => (draftPreferred = moveDown(draftPreferred, i))} disabled={i === draftPreferred.length - 1} title="Move down">&darr;</button>
									<button class="chain-btn remove" onclick={() => (draftPreferred = removeFrom(draftPreferred, i))} title="Remove">&times;</button>
								</div>
							{/each}
							{#if availableEndpoints().length > 0}
								<select class="chain-add-select" onchange={(e) => { const v = (e.target as HTMLSelectElement).value; if (v) { draftPreferred = [...draftPreferred, v]; (e.target as HTMLSelectElement).value = ''; } }}>
									<option value="">+ Add...</option>
									{#each availableEndpoints() as ep}
										<option value={ep}>{ep}</option>
									{/each}
								</select>
							{/if}
						</div>
					</div>
					<div class="chain-section">
						<span class="chain-label">Fallback</span>
						<div class="chain-edit-items">
							{#each draftFallback as ep, i (ep + i)}
								<div class="chain-edit-item">
									<span class="chain-ep">{ep}</span>
									<button class="chain-btn" onclick={() => (draftFallback = moveUp(draftFallback, i))} disabled={i === 0} title="Move up">&uarr;</button>
									<button class="chain-btn" onclick={() => (draftFallback = moveDown(draftFallback, i))} disabled={i === draftFallback.length - 1} title="Move down">&darr;</button>
									<button class="chain-btn remove" onclick={() => (draftFallback = removeFrom(draftFallback, i))} title="Remove">&times;</button>
								</div>
							{/each}
							{#if availableEndpoints().length > 0}
								<select class="chain-add-select" onchange={(e) => { const v = (e.target as HTMLSelectElement).value; if (v) { draftFallback = [...draftFallback, v]; (e.target as HTMLSelectElement).value = ''; } }}>
									<option value="">+ Add...</option>
									{#each availableEndpoints() as ep}
										<option value={ep}>{ep}</option>
									{/each}
								</select>
							{/if}
						</div>
					</div>
					<div class="chain-actions">
						<button class="chain-save-btn" onclick={saveEdit} disabled={saving || draftPreferred.length === 0}>Save</button>
						<button class="chain-cancel-btn" onclick={cancelEditing}>Cancel</button>
					</div>
				</div>
			{:else}
				<!-- Read-only mode -->
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

	.cap-edit-btn {
		margin-left: auto;
		padding: 2px 8px;
		font-size: 0.6875rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
		cursor: pointer;
	}

	.cap-edit-btn:hover:not(:disabled) {
		border-color: var(--ui-border-interactive);
	}

	.cap-edit-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
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

	/* Editing styles */
	.chain-editor {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
		margin-top: var(--spacing-xs);
	}

	.chain-section {
		display: flex;
		align-items: flex-start;
		gap: var(--spacing-sm);
	}

	.chain-edit-items {
		display: flex;
		flex-direction: column;
		gap: 4px;
		flex: 1;
	}

	.chain-edit-item {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.chain-edit-item .chain-ep {
		flex: 1;
	}

	.chain-btn {
		width: 22px;
		height: 22px;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0;
		font-size: 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
		cursor: pointer;
	}

	.chain-btn:hover:not(:disabled) {
		border-color: var(--ui-border-interactive);
	}

	.chain-btn:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	.chain-btn.remove {
		color: var(--status-error);
	}

	.chain-btn.remove:hover:not(:disabled) {
		border-color: var(--status-error);
	}

	.chain-add-select {
		padding: 3px 4px;
		font-size: 0.6875rem;
		border: 1px dashed var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: none;
		color: var(--ui-text-secondary);
		cursor: pointer;
	}

	.chain-actions {
		display: flex;
		gap: var(--spacing-xs);
		margin-top: var(--spacing-xs);
	}

	.chain-save-btn,
	.chain-cancel-btn {
		padding: 3px 12px;
		font-size: 0.6875rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		cursor: pointer;
	}

	.chain-save-btn {
		background: var(--status-success-container);
		color: var(--status-success);
		border-color: var(--status-success);
	}

	.chain-cancel-btn {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
	}

	.chain-save-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
