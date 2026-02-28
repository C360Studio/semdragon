<script lang="ts">
	/**
	 * Store Page - Agent marketplace for purchasing tools and consumables
	 *
	 * Three-panel layout:
	 * - Left: Category filters and agent selector
	 * - Center: Item grid
	 * - Right: Selected item details and inventory
	 */

	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import { StoreGrid, ItemDetail, InventoryPanel, XPBalance } from '$components/store';
	import { worldStore } from '$stores/worldStore.svelte';
	import {
		TrustTierNames,
		type StoreItem,
		type AgentInventory,
		type AgentID,
		type TrustTier,
		type ItemType
	} from '$types';
	import { getStoreItems, getInventory, purchase, useConsumable } from '$services/api';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(true);
	let leftPanelWidth = $state(240);
	let rightPanelWidth = $state(320);

	// Store state
	let storeItems = $state<StoreItem[]>([]);
	let inventory = $state<AgentInventory | null>(null);
	let selectedItemId = $state<string | null>(null);
	let loading = $state(true);
	let purchasing = $state(false);
	let error = $state<string | null>(null);

	// Filter state
	let selectedCategory = $state<ItemType | 'all'>('all');
	let selectedAgentId = $state<AgentID | null>(null);

	// Derived: Selected agent
	const selectedAgent = $derived(
		selectedAgentId ? worldStore.agents.get(selectedAgentId) : null
	);

	// Derived: Agent's XP
	const agentXP = $derived(selectedAgent?.xp ?? 0);

	// Derived: Filtered items
	const filteredItems = $derived.by(() => {
		if (selectedCategory === 'all') return storeItems;
		return storeItems.filter((item) => item.item_type === selectedCategory);
	});

	// Derived: Selected item
	const selectedItem = $derived(
		selectedItemId ? storeItems.find((i) => i.id === selectedItemId) : null
	);

	// Derived: Owned count for selected item (consumables)
	const ownedCount = $derived.by(() => {
		if (!selectedItem || !inventory) return 0;
		if (selectedItem.item_type === 'consumable') {
			return inventory.consumables[selectedItem.id] ?? 0;
		}
		return 0;
	});

	// Load store data when agent changes
	async function loadStoreData() {
		if (!selectedAgentId) {
			storeItems = [];
			inventory = null;
			return;
		}

		loading = true;
		error = null;

		// Use Promise.allSettled for graceful degradation - show store items even if inventory fails
		const [itemsResult, invResult] = await Promise.allSettled([
			getStoreItems(selectedAgentId),
			getInventory(selectedAgentId)
		]);

		if (itemsResult.status === 'fulfilled') {
			storeItems = itemsResult.value;
		} else {
			error = itemsResult.reason instanceof Error
				? itemsResult.reason.message
				: 'Failed to load store items';
			console.error('Failed to load store items:', itemsResult.reason);
		}

		if (invResult.status === 'fulfilled') {
			inventory = invResult.value;
		} else {
			// Log but don't block - user can still browse items without inventory
			console.warn('Failed to load inventory:', invResult.reason);
		}

		loading = false;
	}

	// Auto-select first agent on mount
	$effect(() => {
		if (worldStore.agentList.length > 0 && !selectedAgentId) {
			selectedAgentId = worldStore.agentList[0].id;
		}
	});

	// Load store data when agent changes
	$effect(() => {
		if (selectedAgentId) {
			loadStoreData();
		}
	});

	// Handle item selection
	function selectItem(item: StoreItem) {
		selectedItemId = item.id;
	}

	// Handle purchase
	async function handlePurchase() {
		if (!selectedAgentId || !selectedItemId) return;

		purchasing = true;
		error = null;

		try {
			const response = await purchase({
				agent_id: selectedAgentId,
				item_id: selectedItemId
			});

			if (!response.success) {
				error = response.error ?? 'Purchase failed';
				return;
			}

			// Update local inventory
			inventory = response.inventory;

			// Update agent XP in world store
			const agent = worldStore.agents.get(selectedAgentId);
			if (agent) {
				worldStore.updateAgent({
					...agent,
					xp: response.xp_remaining
				});
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Purchase failed';
			console.error('Purchase failed:', err);
		} finally {
			purchasing = false;
		}
	}

	// Handle consumable use
	async function handleUseConsumable(consumableId: string) {
		if (!selectedAgentId) return;

		try {
			const response = await useConsumable({
				agent_id: selectedAgentId,
				consumable_id: consumableId
			});

			if (!response.success) {
				error = response.error ?? 'Failed to use consumable';
				return;
			}

			// Update local inventory
			if (inventory) {
				inventory = {
					...inventory,
					consumables: {
						...inventory.consumables,
						[consumableId]: response.remaining
					}
				};
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to use consumable';
			console.error('Failed to use consumable:', err);
		}
	}
</script>

<svelte:head>
	<title>Store - Semdragons</title>
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
		<div class="filters-panel">
			<header class="panel-header">
				<h2>Store</h2>
			</header>

			<div class="filters-content">
				<!-- Agent Selector -->
				<section class="filter-section">
					<label class="filter-label" for="agent-select">Shopping as</label>
					<select
						id="agent-select"
						class="agent-select"
						bind:value={selectedAgentId}
					>
						{#each worldStore.agentList as agent (agent.id)}
							<option value={agent.id}>
								{agent.name} (Lvl {agent.level})
							</option>
						{/each}
					</select>

					{#if selectedAgent}
						<div class="agent-xp">
							<XPBalance xp={selectedAgent.xp} />
							<span class="tier-label">
								{TrustTierNames[selectedAgent.tier as TrustTier]}
							</span>
						</div>
					{/if}
				</section>

				<!-- Category Filter -->
				<section class="filter-section">
					<span class="filter-label">Categories</span>
					<div class="category-buttons">
						<button
							class="category-btn"
							class:active={selectedCategory === 'all'}
							onclick={() => (selectedCategory = 'all')}
						>
							All
						</button>
						<button
							class="category-btn"
							class:active={selectedCategory === 'tool'}
							onclick={() => (selectedCategory = 'tool')}
						>
							Tools
						</button>
						<button
							class="category-btn"
							class:active={selectedCategory === 'consumable'}
							onclick={() => (selectedCategory = 'consumable')}
						>
							Consumables
						</button>
					</div>
				</section>

				<!-- Tier Info -->
				{#if selectedAgent}
					<section class="filter-section tier-info">
						<span class="filter-label">Your Tier</span>
						<div class="tier-badge" data-tier={selectedAgent.tier}>
							{TrustTierNames[selectedAgent.tier as TrustTier]}
						</div>
						<p class="tier-hint">
							Higher tier items will appear as you level up
						</p>
					</section>
				{/if}
			</div>
		</div>
	{/snippet}

	{#snippet centerPanel()}
		<div class="store-main">
			<header class="store-header">
				<h1>Agent Store</h1>
				{#if selectedAgent}
					<XPBalance xp={selectedAgent.xp} large />
				{/if}
			</header>

			{#if error}
				<div class="error-banner" role="alert">
					<span>{error}</span>
					<button onclick={() => (error = null)} aria-label="Dismiss error">×</button>
				</div>
			{/if}

			{#if loading}
				<div class="loading-state">
					<div class="loading-grid">
						{#each Array(6) as _}
							<div class="skeleton-card"></div>
						{/each}
					</div>
				</div>
			{:else if !selectedAgentId}
				<div class="empty-state">
					<p>Select an agent to browse the store</p>
				</div>
			{:else}
				<StoreGrid
					items={filteredItems}
					selectedId={selectedItemId}
					{agentXP}
					onSelect={selectItem}
				/>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div class="details-panel">
			<header class="panel-header">
				<h2>Details</h2>
			</header>

			<div class="details-content">
				{#if selectedItem}
					<ItemDetail
						item={selectedItem}
						{agentXP}
						owned={ownedCount}
						{purchasing}
						onPurchase={handlePurchase}
					/>
				{:else}
					<p class="empty-state">Select an item to view details</p>
				{/if}
			</div>

			<div class="inventory-section">
				<header class="section-header">
					<h3>Your Inventory</h3>
				</header>
				<div class="inventory-content">
					<InventoryPanel
						{inventory}
						onUseConsumable={handleUseConsumable}
					/>
				</div>
			</div>
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	/* Filters Panel */
	.filters-panel {
		height: 100%;
		display: flex;
		flex-direction: column;
	}

	.panel-header {
		padding: var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.panel-header h2 {
		font-size: 0.875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-secondary);
		margin: 0;
	}

	.filters-content {
		flex: 1;
		padding: var(--spacing-md);
		display: flex;
		flex-direction: column;
		gap: var(--spacing-lg);
		overflow-y: auto;
	}

	.filter-section {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.filter-label {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
	}

	.agent-select {
		width: 100%;
		padding: var(--spacing-sm);
		font-size: 0.875rem;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		color: var(--ui-text-primary);
	}

	.agent-xp {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--spacing-sm);
	}

	.tier-label {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.category-buttons {
		display: flex;
		gap: var(--spacing-xs);
	}

	.category-btn {
		flex: 1;
		padding: var(--spacing-xs) var(--spacing-sm);
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--ui-text-secondary);
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		cursor: pointer;
		transition: all 150ms ease;
	}

	.category-btn:hover {
		border-color: var(--ui-border-interactive);
	}

	.category-btn.active {
		color: var(--ui-text-on-primary);
		background: var(--ui-interactive-primary);
		border-color: var(--ui-interactive-primary);
	}

	.tier-info {
		padding: var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-md);
	}

	.tier-badge {
		display: inline-block;
		font-size: 0.75rem;
		padding: var(--spacing-xs) var(--spacing-sm);
		border-radius: var(--radius-sm);
		text-transform: uppercase;
		font-weight: 600;
	}

	.tier-badge[data-tier='0'] {
		background: var(--tier-apprentice-container, #e0f2fe);
		color: var(--tier-apprentice, #0284c7);
	}
	.tier-badge[data-tier='1'] {
		background: var(--tier-journeyman-container, #d1fae5);
		color: var(--tier-journeyman, #059669);
	}
	.tier-badge[data-tier='2'] {
		background: var(--tier-expert-container, #fef3c7);
		color: var(--tier-expert, #d97706);
	}
	.tier-badge[data-tier='3'] {
		background: var(--tier-master-container, #ede9fe);
		color: var(--tier-master, #7c3aed);
	}
	.tier-badge[data-tier='4'] {
		background: var(--tier-grandmaster-container, #fce7f3);
		color: var(--tier-grandmaster, #db2777);
	}

	.tier-hint {
		margin: 0;
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	/* Store Main */
	.store-main {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.store-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.store-header h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.error-banner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-error-container, #fee2e2);
		color: var(--ui-error, #dc2626);
		font-size: 0.875rem;
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

	/* Loading State */
	.loading-state {
		flex: 1;
		padding: var(--spacing-md);
	}

	.loading-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
		gap: var(--spacing-md);
	}

	.skeleton-card {
		height: 120px;
		background: linear-gradient(
			90deg,
			var(--ui-surface-secondary) 0%,
			var(--ui-surface-tertiary) 50%,
			var(--ui-surface-secondary) 100%
		);
		background-size: 200% 100%;
		border-radius: var(--radius-md);
		animation: shimmer 1.5s infinite;
	}

	@keyframes shimmer {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}

	.empty-state {
		text-align: center;
		padding: var(--spacing-xl);
		color: var(--ui-text-tertiary);
	}

	/* Details Panel */
	.details-panel {
		height: 100%;
		display: flex;
		flex-direction: column;
	}

	.details-content {
		flex: 1;
		overflow-y: auto;
	}

	.inventory-section {
		border-top: 1px solid var(--ui-border-subtle);
		max-height: 40%;
		display: flex;
		flex-direction: column;
	}

	.section-header {
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.section-header h3 {
		margin: 0;
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-tertiary);
	}

	.inventory-content {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
	}
</style>
