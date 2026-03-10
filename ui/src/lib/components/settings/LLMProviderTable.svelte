<script lang="ts">
	/**
	 * LLMProviderTable - Displays configured LLM endpoints with inline editing.
	 */
	import type { ModelEndpointInfo } from '$types';
	import type { EndpointUpdate } from '$services/api';

	let {
		endpoints,
		defaultModel,
		onSave,
		onRemove,
		saving = false
	}: {
		endpoints: ModelEndpointInfo[];
		defaultModel: string;
		onSave?: (name: string, update: EndpointUpdate) => Promise<void>;
		onRemove?: (name: string) => Promise<void>;
		saving?: boolean;
	} = $props();

	// Editing state
	let editingName = $state<string | null>(null);
	let addingNew = $state(false);
	let draft = $state<EndpointUpdate>({
		provider: 'openai',
		model: '',
		max_tokens: 128000,
		supports_tools: true
	});
	let draftName = $state('');

	function providerLabel(provider: string): string {
		const labels: Record<string, string> = {
			anthropic: 'Anthropic',
			openai: 'OpenAI',
			ollama: 'Ollama',
			openrouter: 'OpenRouter',
			google: 'Google'
		};
		return labels[provider] ?? provider;
	}

	function formatTokens(n: number): string {
		if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(0)}M`;
		if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
		return String(n);
	}

	function startEditing(ep: ModelEndpointInfo) {
		editingName = ep.name;
		addingNew = false;
		draft = {
			provider: ep.provider,
			model: ep.model,
			url: ep.url,
			max_tokens: ep.max_tokens,
			supports_tools: ep.supports_tools,
			tool_format: ep.tool_format,
			api_key_env: ep.api_key_env,
			stream: ep.stream,
			reasoning_effort: ep.reasoning_effort,
			input_price_per_1m_tokens: ep.input_price_per_1m_tokens,
			output_price_per_1m_tokens: ep.output_price_per_1m_tokens
		};
	}

	function startAdding() {
		addingNew = true;
		editingName = null;
		draftName = '';
		draft = {
			provider: 'openai',
			model: '',
			max_tokens: 128000,
			supports_tools: true
		};
	}

	function cancelEditing() {
		editingName = null;
		addingNew = false;
	}

	async function saveEdit() {
		if (!onSave || saving) return;
		const name = addingNew ? draftName.trim() : editingName;
		if (!name || !draft.model.trim() || !draft.provider.trim()) return;
		await onSave(name, draft);
		editingName = null;
		addingNew = false;
	}

	async function handleRemove(name: string) {
		if (!onRemove || saving) return;
		await onRemove(name);
	}
</script>

<div class="provider-table-wrapper">
	<table class="provider-table" data-testid="llm-provider-table">
		<thead>
			<tr>
				<th>Name</th>
				<th>Provider</th>
				<th>Model</th>
				<th>Tokens</th>
				<th>Tools</th>
				<th>Key</th>
				{#if onSave}<th class="actions-col"></th>{/if}
			</tr>
		</thead>
		<tbody>
			{#each endpoints as ep (ep.name)}
				{#if editingName === ep.name}
					<tr class="editing-row">
						<td class="name-cell">{ep.name}</td>
						<td>
							<select class="edit-select" bind:value={draft.provider}>
								<option value="openai">OpenAI</option>
								<option value="anthropic">Anthropic</option>
								<option value="google">Google</option>
								<option value="ollama">Ollama</option>
								<option value="openrouter">OpenRouter</option>
							</select>
						</td>
						<td><input class="edit-input" bind:value={draft.model} placeholder="model-id" /></td>
						<td><input class="edit-input narrow" type="number" bind:value={draft.max_tokens} min="1" /></td>
						<td>
							<label class="toggle-label">
								<input type="checkbox" bind:checked={draft.supports_tools} />
							</label>
						</td>
						<td><input class="edit-input" bind:value={draft.api_key_env} placeholder="ENV_VAR" /></td>
						<td class="actions-cell">
							<button class="row-btn save" onclick={saveEdit} disabled={saving || !draft.model.trim()}>Save</button>
							<button class="row-btn cancel" onclick={cancelEditing}>Cancel</button>
						</td>
					</tr>
				{:else}
					<tr class:default-row={ep.name === defaultModel}>
						<td class="name-cell">
							{ep.name}
							{#if ep.name === defaultModel}
								<span class="default-badge">default</span>
							{/if}
						</td>
						<td>{providerLabel(ep.provider)}</td>
						<td class="model-cell" title={ep.model}>{ep.model}</td>
						<td class="mono">{formatTokens(ep.max_tokens)}</td>
						<td>
							<span class="bool-indicator" data-active={ep.supports_tools}>
								{ep.supports_tools ? 'Yes' : 'No'}
							</span>
						</td>
						<td>
							{#if !ep.api_key_env}
								<span class="key-status" data-status="none" title="No API key required">--</span>
							{:else if ep.api_key_set}
								<span class="key-status" data-status="set" title="{ep.api_key_env} is set">
									<span class="key-dot ok"></span>
									Set
								</span>
							{:else}
								<span class="key-status" data-status="missing" title="{ep.api_key_env} is not set">
									<span class="key-dot error"></span>
									Missing
								</span>
							{/if}
						</td>
						{#if onSave}
							<td class="actions-cell">
								<button class="row-btn edit" onclick={() => startEditing(ep)} disabled={saving} title="Edit endpoint">Edit</button>
								{#if onRemove && ep.name !== defaultModel}
									<button class="row-btn remove" onclick={() => handleRemove(ep.name)} disabled={saving} title="Remove endpoint">Del</button>
								{/if}
							</td>
						{/if}
					</tr>
				{/if}
			{:else}
				<tr>
					<td colspan={onSave ? 7 : 6} class="empty-row">No endpoints configured</td>
				</tr>
			{/each}
			{#if addingNew}
				<tr class="editing-row">
					<td><input class="edit-input" bind:value={draftName} placeholder="endpoint-name" /></td>
					<td>
						<select class="edit-select" bind:value={draft.provider}>
							<option value="openai">OpenAI</option>
							<option value="anthropic">Anthropic</option>
							<option value="google">Google</option>
							<option value="ollama">Ollama</option>
							<option value="openrouter">OpenRouter</option>
						</select>
					</td>
					<td><input class="edit-input" bind:value={draft.model} placeholder="model-id" /></td>
					<td><input class="edit-input narrow" type="number" bind:value={draft.max_tokens} min="1" /></td>
					<td>
						<label class="toggle-label">
							<input type="checkbox" bind:checked={draft.supports_tools} />
						</label>
					</td>
					<td><input class="edit-input" bind:value={draft.api_key_env} placeholder="ENV_VAR" /></td>
					<td class="actions-cell">
						<button class="row-btn save" onclick={saveEdit} disabled={saving || !draftName.trim() || !draft.model.trim()}>Save</button>
						<button class="row-btn cancel" onclick={cancelEditing}>Cancel</button>
					</td>
				</tr>
			{/if}
		</tbody>
	</table>
	{#if onSave && !addingNew && editingName === null}
		<button class="add-btn" onclick={startAdding} disabled={saving}>+ Add Endpoint</button>
	{/if}
</div>

<style>
	.provider-table-wrapper {
		overflow-x: auto;
	}

	.provider-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.8125rem;
	}

	.provider-table th {
		text-align: left;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-sm) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.provider-table td {
		padding: var(--spacing-sm) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
		vertical-align: middle;
	}

	.provider-table tbody tr:last-child td {
		border-bottom: none;
	}

	.provider-table tbody tr:hover {
		background: var(--ui-surface-tertiary);
	}

	.default-row {
		background: var(--ui-surface-primary);
	}

	.name-cell {
		font-weight: 500;
		display: flex;
		align-items: center;
		gap: var(--spacing-xs);
	}

	.default-badge {
		font-size: 0.625rem;
		font-weight: 500;
		padding: 1px 5px;
		border-radius: var(--radius-full);
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
	}

	.model-cell {
		font-family: monospace;
		font-size: 0.75rem;
		max-width: 200px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.mono {
		font-family: monospace;
	}

	.bool-indicator {
		font-size: 0.75rem;
	}

	.bool-indicator[data-active='true'] {
		color: var(--status-success);
	}

	.bool-indicator[data-active='false'] {
		color: var(--ui-text-tertiary);
	}

	.key-status {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-size: 0.75rem;
	}

	.key-status[data-status='none'] {
		color: var(--ui-text-tertiary);
	}

	.key-dot {
		width: 6px;
		height: 6px;
		border-radius: var(--radius-full);
	}

	.key-dot.ok {
		background: var(--status-success);
	}

	.key-dot.error {
		background: var(--status-error);
	}

	.empty-row {
		text-align: center;
		color: var(--ui-text-tertiary);
		padding: var(--spacing-lg) !important;
	}

	/* Editing styles */
	.actions-col {
		width: 100px;
	}

	.actions-cell {
		display: flex;
		gap: var(--spacing-xs);
		white-space: nowrap;
	}

	.editing-row {
		background: var(--ui-surface-primary);
	}

	.edit-input {
		width: 100%;
		padding: 3px 6px;
		font-size: 0.75rem;
		font-family: monospace;
		border: 1px solid var(--ui-border-interactive);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		outline: none;
	}

	.edit-input.narrow {
		width: 80px;
	}

	.edit-input:focus {
		border-color: var(--ui-border-focus);
	}

	.edit-select {
		padding: 3px 4px;
		font-size: 0.75rem;
		border: 1px solid var(--ui-border-interactive);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		outline: none;
	}

	.toggle-label {
		cursor: pointer;
	}

	.row-btn {
		padding: 2px 8px;
		font-size: 0.6875rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		cursor: pointer;
		white-space: nowrap;
	}

	.row-btn.save {
		background: var(--status-success-container);
		color: var(--status-success);
		border-color: var(--status-success);
	}

	.row-btn.cancel {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
	}

	.row-btn.edit {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
	}

	.row-btn.edit:hover:not(:disabled) {
		border-color: var(--ui-border-interactive);
	}

	.row-btn.remove {
		background: none;
		color: var(--status-error);
		border-color: transparent;
	}

	.row-btn.remove:hover:not(:disabled) {
		border-color: var(--status-error);
	}

	.row-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.add-btn {
		margin-top: var(--spacing-sm);
		padding: var(--spacing-xs) var(--spacing-md);
		font-size: 0.75rem;
		border: 1px dashed var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: none;
		color: var(--ui-text-secondary);
		cursor: pointer;
		width: 100%;
		transition: border-color 150ms ease;
	}

	.add-btn:hover:not(:disabled) {
		border-color: var(--ui-border-interactive);
		color: var(--ui-text-primary);
	}

	.add-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
