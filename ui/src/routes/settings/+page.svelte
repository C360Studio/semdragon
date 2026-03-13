<script lang="ts">
	/**
	 * Settings Page - System configuration, health, and onboarding
	 *
	 * Three-panel layout:
	 * - Left: ExplorerNav
	 * - Center: Settings sections (collapsible cards)
	 * - Right: Contextual help for active section
	 */

	import { onMount, onDestroy } from 'svelte';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import {
		SettingsSection,
		ConnectionCard,
		LLMProviderTable,
		CapabilityRouting,
		ComponentTable,
		OnboardingChecklist,
		SettingsHelp
	} from '$components/settings';
	import { getSettings, getSettingsHealth, updateSettings, ApiError } from '$services/api';
	import type { EndpointUpdate, CapabilityUpdate } from '$services/api';
	import type { SettingsResponse, HealthResponse, ModelEndpointInfo, CapabilityInfo } from '$types';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(240);
	let rightPanelWidth = $state(280);

	// Data state
	let settings = $state<SettingsResponse | null>(null);
	let health = $state<HealthResponse | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let backendUnavailable = $state(false);

	// Active section for help panel
	let activeSection = $state('');

	// Websocket input editing state
	let wsUrlEditing = $state(false);
	let wsUrlDraft = $state('');
	let wsSaving = $state(false);

	// Token budget editing state
	let budgetEditing = $state(false);
	let budgetDraft = $state(0);
	let budgetSaving = $state(false);

	// Search config editing state
	let searchEditing = $state(false);
	let searchProviderDraft = $state('');
	let searchApiKeyDraft = $state('');
	let searchBaseUrlDraft = $state('');
	let searchSaving = $state(false);

	// General saving flag for provider/capability updates
	let saving = $state(false);

	// Health auto-refresh
	let healthInterval: ReturnType<typeof setInterval> | undefined;

	// Derived: checklist items that are not met
	const unmetCount = $derived(
		health ? health.checklist.filter((i) => !i.met).length : 0
	);

	async function loadData() {
		loading = true;
		error = null;
		backendUnavailable = false;

		const [settingsResult, healthResult] = await Promise.allSettled([
			getSettings(),
			getSettingsHealth()
		]);

		if (settingsResult.status === 'fulfilled') {
			settings = settingsResult.value;
		} else {
			// Network errors (fetch throws TypeError), not API errors
			if (!(settingsResult.reason instanceof ApiError)) {
				backendUnavailable = true;
			}
			error =
				settingsResult.reason instanceof Error
					? settingsResult.reason.message
					: 'Failed to load settings';
			console.error('Failed to load settings:', settingsResult.reason);
		}

		if (healthResult.status === 'fulfilled') {
			health = healthResult.value;
		} else {
			console.warn('Failed to load health:', healthResult.reason);
		}

		loading = false;
	}

	async function refreshHealth() {
		try {
			health = await getSettingsHealth();
		} catch (err) {
			console.warn('Health refresh failed:', err);
		}
	}

	async function toggleWebsocket() {
		if (!settings || wsSaving) return;
		wsSaving = true;
		try {
			settings = await updateSettings({
				websocket_input: { enabled: !settings.websocket_input.enabled }
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update websocket';
		} finally {
			wsSaving = false;
		}
	}

	function startEditingWsUrl() {
		if (!settings) return;
		wsUrlDraft = settings.websocket_input.url;
		wsUrlEditing = true;
	}

	function cancelEditingWsUrl() {
		wsUrlEditing = false;
		wsUrlDraft = '';
	}

	async function saveWsUrl() {
		if (!settings || wsSaving || !wsUrlDraft.trim()) return;
		wsSaving = true;
		try {
			settings = await updateSettings({
				websocket_input: { url: wsUrlDraft.trim() }
			});
			wsUrlEditing = false;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update websocket URL';
		} finally {
			wsSaving = false;
		}
	}

	// Token budget editing
	function startEditingBudget() {
		if (!settings?.token_budget) return;
		budgetDraft = settings.token_budget.global_hourly_limit;
		budgetEditing = true;
	}

	function cancelEditingBudget() {
		budgetEditing = false;
	}

	async function saveBudget() {
		if (budgetSaving || budgetDraft < 0) return;
		budgetSaving = true;
		try {
			settings = await updateSettings({
				token_budget: { global_hourly_limit: budgetDraft }
			});
			budgetEditing = false;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update token budget';
		} finally {
			budgetSaving = false;
		}
	}

	// LLM Provider updates
	async function saveEndpoint(name: string, update: EndpointUpdate) {
		if (saving) return;
		saving = true;
		try {
			settings = await updateSettings({
				model_registry: { endpoints: { [name]: update } }
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update endpoint';
		} finally {
			saving = false;
		}
	}

	async function removeEndpoint(name: string) {
		if (saving) return;
		saving = true;
		try {
			settings = await updateSettings({
				model_registry: { endpoints: { [name]: { remove: true } as EndpointUpdate } }
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to remove endpoint';
		} finally {
			saving = false;
		}
	}

	// Capability updates
	async function saveCapability(name: string, update: CapabilityUpdate) {
		if (saving) return;
		saving = true;
		try {
			settings = await updateSettings({
				model_registry: { capabilities: { [name]: update } }
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update capability';
		} finally {
			saving = false;
		}
	}

	// Search config editing
	function startEditingSearch() {
		if (!settings) return;
		searchProviderDraft = settings.search_config.provider || 'brave';
		searchApiKeyDraft = '';
		searchBaseUrlDraft = settings.search_config.base_url || '';
		searchEditing = true;
	}

	function cancelEditingSearch() {
		searchEditing = false;
	}

	async function saveSearchConfig() {
		if (searchSaving) return;
		searchSaving = true;
		try {
			const update: Record<string, string> = { provider: searchProviderDraft };
			if (searchApiKeyDraft) update.api_key = searchApiKeyDraft;
			if (searchBaseUrlDraft) update.base_url = searchBaseUrlDraft;
			settings = await updateSettings({ search_config: update });
			searchEditing = false;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update search config';
		} finally {
			searchSaving = false;
		}
	}

	onMount(() => {
		loadData();
		healthInterval = setInterval(refreshHealth, 30_000);
	});

	onDestroy(() => {
		if (healthInterval) clearInterval(healthInterval);
	});
</script>

<svelte:head>
	<title>Settings - Semdragons</title>
</svelte:head>

<ThreePanelLayout
	{leftPanelOpen}
	{rightPanelOpen}
	{leftPanelWidth}
	{rightPanelWidth}
	onLeftWidthChange={(w) => (leftPanelWidth = w)}
	onRightWidthChange={(w) => (rightPanelWidth = w)}
	onToggleLeft={() => (leftPanelOpen = !leftPanelOpen)}
	onToggleRight={() => (rightPanelOpen = !rightPanelOpen)}
>
	{#snippet leftPanel()}
		<ExplorerNav />
	{/snippet}

	{#snippet centerPanel()}
		<div class="settings-main">
			<header class="settings-header">
				<h1 data-testid="settings-heading">Settings</h1>
				<button class="refresh-btn" data-testid="settings-refresh-btn" onclick={loadData} disabled={loading} aria-label="Refresh settings">
					Refresh
				</button>
			</header>

			{#if error}
				<div class="error-banner" data-testid="settings-error-banner" role="alert">
					<span>{error}</span>
					<button onclick={() => (error = null)} aria-label="Dismiss error">&times;</button>
				</div>
			{/if}

			{#if loading}
				<div class="loading-state" data-testid="settings-loading">
					{#each [0, 1, 2, 3] as _}
						<div class="skeleton-section"></div>
					{/each}
				</div>
			{:else if backendUnavailable}
				<div class="unavailable-state" data-testid="settings-unavailable">
					<div class="unavailable-icon">*</div>
					<h2>Backend Unavailable</h2>
					<p>Cannot reach the Semdragons API. Make sure the backend is running.</p>
					<button class="retry-btn" onclick={loadData}>Retry</button>
				</div>
			{:else if settings}
				<div class="sections" data-testid="settings-sections">
					<!-- Onboarding Checklist (shown prominently if items are unmet) -->
					{#if health && unmetCount > 0}
						<SettingsSection
							title="Getting Started"
							badge="{unmetCount} remaining"
							badgeVariant="warning"
							onfocus={() => (activeSection = 'checklist')}
						>
							<OnboardingChecklist items={health.checklist} />
						</SettingsSection>
					{/if}

					<!-- Connection Status -->
					<SettingsSection
						title="Connection"
						badge={settings.nats.connected ? 'Connected' : 'Disconnected'}
						badgeVariant={settings.nats.connected ? 'success' : 'error'}
						onfocus={() => (activeSection = 'connection')}
					>
						<ConnectionCard nats={settings.nats} overall={health?.overall} />
					</SettingsSection>

					<!-- Platform Identity -->
					<SettingsSection
						title="Platform Identity"
						badge="Restart required"
						badgeVariant="neutral"
						onfocus={() => (activeSection = 'platform')}
					>
						<div class="kv-grid">
							<div class="kv-pair">
								<span class="kv-key">Org</span>
								<span class="kv-value">{settings.platform.org}</span>
							</div>
							<div class="kv-pair">
								<span class="kv-key">Platform</span>
								<span class="kv-value">{settings.platform.platform}</span>
							</div>
							<div class="kv-pair">
								<span class="kv-key">Board</span>
								<span class="kv-value">{settings.platform.board}</span>
							</div>
						</div>
					</SettingsSection>

					<!-- LLM Providers -->
					<SettingsSection
						title="LLM Providers"
						badge="{settings.models.endpoints.length} endpoints"
						onfocus={() => (activeSection = 'providers')}
					>
						<LLMProviderTable
							endpoints={settings.models.endpoints}
							defaultModel={settings.models.defaults.model}
							onSave={saveEndpoint}
							onRemove={removeEndpoint}
							{saving}
						/>
					</SettingsSection>

					<!-- Capability Routing -->
					<SettingsSection
						title="Capability Routing"
						badge="{Object.keys(settings.models.capabilities).length} capabilities"
						onfocus={() => (activeSection = 'capabilities')}
					>
						<CapabilityRouting
							capabilities={settings.models.capabilities}
							defaultCapability={settings.models.defaults.capability}
							endpointNames={settings.models.endpoints.map(e => e.name)}
							onSave={saveCapability}
							{saving}
						/>
					</SettingsSection>

					<!-- Components -->
					<SettingsSection
						title="Components"
						badge="{settings.components.filter(c => c.running).length}/{settings.components.length} running"
						onfocus={() => (activeSection = 'components')}
					>
						<ComponentTable components={settings.components} />
					</SettingsSection>

					<!-- Web Search -->
					<SettingsSection
						title="Web Search"
						badge={settings.search_config.provider
							? settings.search_config.api_key_set ? 'Configured' : 'Key missing'
							: 'Not configured'}
						badgeVariant={settings.search_config.provider
							? settings.search_config.api_key_set ? 'success' : 'warning'
							: 'neutral'}
						onfocus={() => (activeSection = 'search')}
					>
						{#if searchEditing}
							<div class="search-edit">
								<div class="kv-grid">
									<div class="kv-pair">
										<span class="kv-key">Provider</span>
										<select class="inline-select" bind:value={searchProviderDraft}>
											<option value="brave">Brave Search</option>
										</select>
									</div>
									<div class="kv-pair">
										<span class="kv-key">API Key</span>
										<input
											type="password"
											class="inline-input"
											bind:value={searchApiKeyDraft}
											placeholder={settings.search_config.api_key_set ? '(unchanged)' : 'Enter API key'}
										/>
									</div>
									<div class="kv-pair">
										<span class="kv-key">Base URL (optional)</span>
										<input
											class="inline-input"
											bind:value={searchBaseUrlDraft}
											placeholder="Default endpoint"
										/>
									</div>
								</div>
								<div class="search-actions">
									<button class="inline-btn save" onclick={saveSearchConfig} disabled={searchSaving || !searchProviderDraft}>Save</button>
									<button class="inline-btn cancel" onclick={cancelEditingSearch}>Cancel</button>
								</div>
							</div>
						{:else}
							<div class="kv-grid">
								<div class="kv-pair">
									<span class="kv-key">Provider</span>
									<span class="kv-value">{settings.search_config.provider || 'None'}</span>
								</div>
								<div class="kv-pair">
									<span class="kv-key">API Key</span>
									<span class="kv-value">
										{#if !settings.search_config.provider}
											--
										{:else if settings.search_config.api_key_set}
											<span class="key-dot ok"></span> Set
										{:else}
											<span class="key-dot error"></span> Missing
										{/if}
									</span>
								</div>
								{#if settings.search_config.base_url}
									<div class="kv-pair">
										<span class="kv-key">Base URL</span>
										<span class="kv-value mono">{settings.search_config.base_url}</span>
									</div>
								{/if}
							</div>
							<button class="section-edit-btn" onclick={startEditingSearch}>
								{settings.search_config.provider ? 'Edit' : 'Configure'}
							</button>
						{/if}
					</SettingsSection>

					<!-- WebSocket Input (semsource) -->
					<SettingsSection
						title="WebSocket Input"
						badge={settings.websocket_input.enabled
							? settings.websocket_input.connected ? 'Connected' : 'Disconnected'
							: 'Disabled'}
						badgeVariant={settings.websocket_input.enabled
							? settings.websocket_input.connected ? 'success' : 'warning'
							: 'neutral'}
						onfocus={() => (activeSection = 'websocket')}
					>
						<div class="ws-section">
							<div class="ws-toggle-row">
								<span class="ws-toggle-label">WebSocket Input</span>
								<button
									class="ws-toggle"
									class:active={settings.websocket_input.enabled}
									data-testid="ws-toggle-btn"
									onclick={toggleWebsocket}
									disabled={wsSaving}
									aria-label={settings.websocket_input.enabled ? 'Disable websocket input' : 'Enable websocket input'}
								>
									{settings.websocket_input.enabled ? 'Enabled' : 'Disabled'}
								</button>
							</div>

							<div class="kv-grid">
								<div class="kv-pair ws-url-pair">
									<span class="kv-key">URL</span>
									{#if wsUrlEditing}
										<div class="ws-url-edit">
											<input
												type="text"
												class="ws-url-input"
												data-testid="ws-url-input"
												bind:value={wsUrlDraft}
												placeholder="ws://host:port/path"
												onkeydown={(e) => { if (e.key === 'Enter') saveWsUrl(); if (e.key === 'Escape') cancelEditingWsUrl(); }}
											/>
											<button class="ws-url-btn save" data-testid="ws-url-save" onclick={saveWsUrl} disabled={wsSaving || !wsUrlDraft.trim()}>Save</button>
											<button class="ws-url-btn cancel" data-testid="ws-url-cancel" onclick={cancelEditingWsUrl}>Cancel</button>
										</div>
									{:else}
										<button class="ws-url-display" data-testid="ws-url-display" onclick={startEditingWsUrl}>
											<span class="mono">{settings.websocket_input.url || '(not configured)'}</span>
											<span class="edit-hint">edit</span>
										</button>
									{/if}
								</div>
								{#if settings.websocket_input.enabled}
									<div class="kv-pair">
										<span class="kv-key">Status</span>
										<span class="kv-value">{settings.websocket_input.status || (settings.websocket_input.healthy ? 'Healthy' : 'Unhealthy')}</span>
									</div>
								{/if}
							</div>
						</div>
					</SettingsSection>

					<!-- Workspace (Artifact Store) -->
					<SettingsSection
						title="Workspace"
						badge={settings.workspace.available ? 'OK' : 'Unavailable'}
						badgeVariant={settings.workspace.available ? 'success' : 'warning'}
						onfocus={() => (activeSection = 'workspace')}
					>
						<div class="kv-grid">
							<div class="kv-pair">
								<span class="kv-key">Artifact Store</span>
								<span class="kv-value">{settings.workspace.available ? 'Available' : 'Not configured'}</span>
							</div>
						</div>
					</SettingsSection>

					<!-- Token Budget -->
					{#if settings.token_budget}
						<SettingsSection
							title="Token Budget"
							onfocus={() => (activeSection = 'budget')}
						>
							<div class="kv-grid">
								<div class="kv-pair budget-pair">
									<span class="kv-key">Hourly Limit</span>
									{#if budgetEditing}
										<div class="inline-edit">
											<input
												type="number"
												class="inline-input"
												data-testid="budget-input"
												bind:value={budgetDraft}
												min="0"
												placeholder="0 = unlimited"
												onkeydown={(e) => { if (e.key === 'Enter') saveBudget(); if (e.key === 'Escape') cancelEditingBudget(); }}
											/>
											<button class="inline-btn save" data-testid="budget-save" onclick={saveBudget} disabled={budgetSaving || budgetDraft < 0}>Save</button>
											<button class="inline-btn cancel" data-testid="budget-cancel" onclick={cancelEditingBudget}>Cancel</button>
										</div>
									{:else}
										<button class="editable-value" data-testid="budget-display" onclick={startEditingBudget}>
											<span class="mono">
												{settings.token_budget.global_hourly_limit === 0
													? 'Unlimited'
													: settings.token_budget.global_hourly_limit.toLocaleString()}
											</span>
											<span class="edit-hint">edit</span>
										</button>
									{/if}
								</div>
							</div>
						</SettingsSection>
					{/if}

					<!-- All-clear checklist (collapsed when all met) -->
					{#if health && unmetCount === 0}
						<SettingsSection
							title="Getting Started"
							open={false}
							badge="All clear"
							badgeVariant="success"
							onfocus={() => (activeSection = 'checklist')}
						>
							<OnboardingChecklist items={health.checklist} />
						</SettingsSection>
					{/if}
				</div>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<SettingsHelp {activeSection} />
	{/snippet}
</ThreePanelLayout>

<style>
	.settings-main {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow-y: auto;
	}

	.settings-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
		flex-shrink: 0;
	}

	.settings-header h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.refresh-btn {
		padding: var(--spacing-xs) var(--spacing-md);
		font-size: 0.8125rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
		cursor: pointer;
		transition: border-color 150ms ease;
	}

	.refresh-btn:hover:not(:disabled) {
		border-color: var(--ui-border-interactive);
	}

	.refresh-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.error-banner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--status-error-container);
		color: var(--status-error);
		font-size: 0.875rem;
		flex-shrink: 0;
	}

	.error-banner button {
		padding: var(--spacing-xs);
		background: none;
		border: none;
		color: inherit;
		font-size: 1.25rem;
		cursor: pointer;
		opacity: 0.7;
	}

	.error-banner button:hover {
		opacity: 1;
	}

	.sections {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
		padding: var(--spacing-lg);
	}

	/* Loading */
	.loading-state {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
		padding: var(--spacing-lg);
	}

	.skeleton-section {
		height: 80px;
		background: linear-gradient(
			90deg,
			var(--ui-surface-secondary) 0%,
			var(--ui-surface-tertiary) 50%,
			var(--ui-surface-secondary) 100%
		);
		background-size: 200% 100%;
		border-radius: var(--radius-lg);
		animation: shimmer 1.5s infinite;
	}

	@keyframes shimmer {
		0% { background-position: 200% 0; }
		100% { background-position: -200% 0; }
	}

	/* Unavailable */
	.unavailable-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		flex: 1;
		padding: var(--spacing-xl);
		text-align: center;
		color: var(--ui-text-secondary);
	}

	.unavailable-icon {
		width: 48px;
		height: 48px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-lg);
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-tertiary);
		font-size: 1.5rem;
		font-weight: 700;
		margin-bottom: var(--spacing-md);
	}

	.unavailable-state h2 {
		margin: 0 0 var(--spacing-sm);
		font-size: 1.125rem;
		color: var(--ui-text-primary);
	}

	.unavailable-state p {
		margin: 0 0 var(--spacing-lg);
		font-size: 0.875rem;
		max-width: 400px;
		line-height: 1.5;
	}

	.retry-btn {
		padding: var(--spacing-xs) var(--spacing-lg);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
		font-size: 0.875rem;
		cursor: pointer;
		transition: border-color 150ms ease;
	}

	.retry-btn:hover {
		border-color: var(--ui-border-interactive);
	}

	/* Key-value grid */
	.kv-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
		gap: var(--spacing-sm);
	}

	.kv-pair {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-primary);
		border-radius: var(--radius-md);
		border: 1px solid var(--ui-border-subtle);
	}

	.kv-key {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.kv-value {
		font-size: 0.875rem;
		font-weight: 500;
	}

	.mono {
		font-family: monospace;
		font-size: 0.8125rem;
	}

	/* WebSocket section */
	.ws-section {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.ws-toggle-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--spacing-md);
	}

	.ws-toggle-label {
		font-size: 0.8125rem;
		font-weight: 500;
	}

	.ws-toggle {
		padding: var(--spacing-xs) var(--spacing-md);
		font-size: 0.75rem;
		font-weight: 500;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-full);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
		cursor: pointer;
		transition: all 150ms ease;
	}

	.ws-toggle.active {
		background: var(--status-success-container);
		color: var(--status-success);
		border-color: var(--status-success);
	}

	.ws-toggle:hover:not(:disabled) {
		border-color: var(--ui-border-interactive);
	}

	.ws-toggle:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.ws-url-pair {
		grid-column: 1 / -1;
	}

	.ws-url-display {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		background: none;
		border: none;
		padding: 0;
		cursor: pointer;
		text-align: left;
		color: var(--ui-text-primary);
		font-size: 0.875rem;
		font-weight: 500;
	}

	.ws-url-display:hover .edit-hint {
		opacity: 1;
	}

	.edit-hint {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		opacity: 0;
		transition: opacity 150ms ease;
	}

	.ws-url-edit {
		display: flex;
		gap: var(--spacing-xs);
		align-items: center;
	}

	.ws-url-input {
		flex: 1;
		padding: var(--spacing-xs) var(--spacing-sm);
		font-size: 0.8125rem;
		font-family: monospace;
		border: 1px solid var(--ui-border-interactive);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		outline: none;
	}

	.ws-url-input:focus {
		border-color: var(--ui-border-focus);
	}

	.ws-url-btn {
		padding: var(--spacing-xs) var(--spacing-sm);
		font-size: 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		cursor: pointer;
		white-space: nowrap;
	}

	.ws-url-btn.save {
		background: var(--status-success-container);
		color: var(--status-success);
		border-color: var(--status-success);
	}

	.ws-url-btn.cancel {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
	}

	.ws-url-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* Shared inline edit styles (budget, etc.) */
	.editable-value {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		background: none;
		border: none;
		padding: 0;
		cursor: pointer;
		text-align: left;
		color: var(--ui-text-primary);
		font-size: 0.875rem;
		font-weight: 500;
	}

	.editable-value:hover .edit-hint {
		opacity: 1;
	}

	.budget-pair {
		grid-column: 1 / -1;
	}

	.inline-edit {
		display: flex;
		gap: var(--spacing-xs);
		align-items: center;
	}

	.inline-input {
		width: 140px;
		padding: var(--spacing-xs) var(--spacing-sm);
		font-size: 0.8125rem;
		font-family: monospace;
		border: 1px solid var(--ui-border-interactive);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		outline: none;
	}

	.inline-input:focus {
		border-color: var(--ui-border-focus);
	}

	.inline-btn {
		padding: var(--spacing-xs) var(--spacing-sm);
		font-size: 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		cursor: pointer;
		white-space: nowrap;
	}

	.inline-btn.save {
		background: var(--status-success-container);
		color: var(--status-success);
		border-color: var(--status-success);
	}

	.inline-btn.cancel {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
	}

	.inline-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.inline-select {
		padding: var(--spacing-xs) var(--spacing-sm);
		font-size: 0.8125rem;
		border: 1px solid var(--ui-border-interactive);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		outline: none;
	}

	/* Search config section */
	.search-edit {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-md);
	}

	.search-actions {
		display: flex;
		gap: var(--spacing-xs);
	}

	.section-edit-btn {
		margin-top: var(--spacing-sm);
		padding: var(--spacing-xs) var(--spacing-md);
		font-size: 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
		cursor: pointer;
		transition: border-color 150ms ease;
	}

	.section-edit-btn:hover {
		border-color: var(--ui-border-interactive);
	}

	/* Key dots (reused from provider table pattern) */
	.key-dot {
		display: inline-block;
		width: 6px;
		height: 6px;
		border-radius: var(--radius-full);
		margin-right: 4px;
	}

	.key-dot.ok {
		background: var(--status-success);
	}

	.key-dot.error {
		background: var(--status-error);
	}
</style>
