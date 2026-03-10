<script lang="ts">
	/**
	 * SettingsHelp - Right panel contextual help for the active settings section.
	 */

	let { activeSection = '' }: { activeSection?: string } = $props();

	const helpContent: Record<string, { title: string; body: string }> = {
		connection: {
			title: 'Connection',
			body: 'Shows NATS message broker connection status. NATS is the backbone — all entity state, events, and agent communication flows through it. A healthy connection with low latency is essential.'
		},
		platform: {
			title: 'Platform Identity',
			body: 'Org, platform, and board define the entity ID namespace (e.g., org.platform.game.board.type.instance). These are set at startup and cannot be changed at runtime — a restart is required.'
		},
		providers: {
			title: 'LLM Providers',
			body: 'Each endpoint defines a model backend that agents can use. The "Key" column shows whether the required API key environment variable is set. Ollama endpoints run locally and need no key.'
		},
		capabilities: {
			title: 'Capability Routing',
			body: 'Capabilities map agent tasks to LLM endpoints. "agent-work" handles quest execution, "boss-battle" handles reviews, "quest-design" handles decomposition, and "dm-chat" powers the Dungeon Master. Preferred endpoints are tried first, then fallbacks.'
		},
		components: {
			title: 'Components',
			body: 'Processors are reactive components that watch NATS KV for entity state changes and respond. Each has its own health status, uptime, and error count. Disabled components are configured but not running.'
		},
		workspace: {
			title: 'Workspace',
			body: 'The workspace directory is where agents read and write files during quest execution. It must exist and be writable. In Docker, it is mounted automatically at /workspace. For local dev, create .workspace in the project root.'
		},
		budget: {
			title: 'Token Budget',
			body: 'The global hourly limit controls how many tokens agents can consume per hour across all LLM calls. Set to 0 for unlimited. When the limit is reached, a circuit breaker trips and quests queue until the next hourly window.'
		},
		websocket: {
			title: 'WebSocket Input',
			body: 'The WebSocket input connects to a semsource server to receive entity data (repos, docs, spec sheets) for injection into the entity graph. Toggle it on/off and set the URL to your semsource instance. Changes take effect immediately — the component manager watches for config updates via NATS KV.'
		},
		checklist: {
			title: 'Getting Started',
			body: 'The onboarding checklist shows prerequisites for running your first quest. Green checkmarks mean that item is ready. Red items need attention — follow the help text to fix them.'
		}
	};

	const help = $derived(helpContent[activeSection]);
</script>

<div class="help-panel">
	<header class="help-header">
		<h2>Help</h2>
	</header>
	<div class="help-body" aria-live="polite">
		{#if help}
			<h3>{help.title}</h3>
			<p>{help.body}</p>
		{:else}
			<p class="help-placeholder">Select a settings section to see help.</p>
		{/if}
	</div>
</div>

<style>
	.help-panel {
		height: 100%;
		display: flex;
		flex-direction: column;
	}

	.help-header {
		padding: var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.help-header h2 {
		font-size: 0.875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-secondary);
		margin: 0;
	}

	.help-body {
		padding: var(--spacing-md);
		flex: 1;
		overflow-y: auto;
	}

	.help-body h3 {
		margin: 0 0 var(--spacing-sm);
		font-size: 0.9375rem;
		font-weight: 600;
	}

	.help-body p {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
		line-height: 1.5;
	}

	.help-placeholder {
		color: var(--ui-text-tertiary);
		font-style: italic;
	}
</style>
