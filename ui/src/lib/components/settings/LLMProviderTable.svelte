<script lang="ts">
	/**
	 * LLMProviderTable - Displays configured LLM endpoints with API key status.
	 */
	import type { ModelEndpointInfo } from '$types';

	let {
		endpoints,
		defaultModel
	}: {
		endpoints: ModelEndpointInfo[];
		defaultModel: string;
	} = $props();

	function providerLabel(provider: string): string {
		const labels: Record<string, string> = {
			anthropic: 'Anthropic',
			openai: 'OpenAI',
			ollama: 'Ollama',
			openrouter: 'OpenRouter'
		};
		return labels[provider] ?? provider;
	}

	function formatTokens(n: number): string {
		if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(0)}M`;
		if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
		return String(n);
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
			</tr>
		</thead>
		<tbody>
			{#each endpoints as ep (ep.name)}
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
				</tr>
			{:else}
				<tr>
					<td colspan="6" class="empty-row">No endpoints configured</td>
				</tr>
			{/each}
		</tbody>
	</table>
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
</style>
