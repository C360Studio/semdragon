<script lang="ts">
	/**
	 * GraphFilters - Filter controls for the graph visualization
	 *
	 * Provides:
	 * - Entity type checkboxes (quest/agent/party/guild/battle/peer_review)
	 *   that map to graphStore.toggleEntityType
	 * - Text search input for filtering by entity ID substring
	 * - "All / None" quick-select buttons
	 *
	 * The filter state is owned by graphStore; this component only dispatches
	 * changes via callback props to keep it stateless and testable.
	 */

	import type { GameEntityType } from '$lib/api/graph-types';
	import { ENTITY_TYPE_COLORS } from '$lib/utils/entity-colors';

	interface GraphFiltersProps {
		/** Set of currently visible entity types from graphStore.visibleTypes */
		visibleTypes: Set<GameEntityType>;
		/** Current search string from graphStore.filters.search */
		search: string;
		/** Callback when user toggles a type checkbox */
		onToggleType: (type: GameEntityType) => void;
		/** Callback when user changes the search text */
		onSearchChange: (search: string) => void;
		/** Callback to show all types */
		onShowAll: () => void;
		/** Callback to hide all types */
		onHideAll: () => void;
	}

	let {
		visibleTypes,
		search,
		onToggleType,
		onSearchChange,
		onShowAll,
		onHideAll
	}: GraphFiltersProps = $props();

	const GAME_TYPES: GameEntityType[] = ['quest', 'agent', 'party', 'guild', 'battle', 'peer_review'];

	function handleSearchInput(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		onSearchChange(input.value);
	}

	function handleSearchKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			onSearchChange('');
		}
	}
</script>

<div class="graph-filters" data-testid="graph-filters">
	<!-- Search -->
	<div class="filter-section">
		<label class="section-label" for="graph-search">Search</label>
		<div class="search-wrapper">
			<input
				id="graph-search"
				type="search"
				class="search-input"
				placeholder="Filter by ID or name…"
				value={search}
				oninput={handleSearchInput}
				onkeydown={handleSearchKeydown}
				aria-label="Filter entities by ID or name"
			/>
			{#if search}
				<button
					class="clear-search"
					onclick={() => onSearchChange('')}
					aria-label="Clear search"
					title="Clear"
				>
					×
				</button>
			{/if}
		</div>
	</div>

	<!-- Entity type toggles -->
	<div class="filter-section">
		<div class="section-header">
			<span class="section-label">Entity Types</span>
			<div class="quick-actions">
				<button class="quick-btn" onclick={onShowAll} title="Show all entity types">All</button>
				<button class="quick-btn" onclick={onHideAll} title="Hide all entity types">None</button>
			</div>
		</div>
		<div class="type-list" role="group" aria-label="Entity type filters">
			{#each GAME_TYPES as type (type)}
				{@const checked = visibleTypes.has(type)}
				{@const color = ENTITY_TYPE_COLORS[type] ?? ENTITY_TYPE_COLORS.unknown}
				<label
					class="type-checkbox"
					class:type-checked={checked}
					style="--type-color: {color}"
					data-testid="filter-type-{type}"
				>
					<input
						type="checkbox"
						{checked}
						onchange={() => onToggleType(type)}
						aria-label="Show {type} entities"
					/>
					<span class="type-dot" aria-hidden="true"></span>
					<span class="type-name">{type.replaceAll('_', ' ')}</span>
				</label>
			{/each}
		</div>
	</div>
</div>

<style>
	.graph-filters {
		display: flex;
		flex-direction: column;
		gap: 0;
		background: var(--ui-surface-secondary);
		border-right: 1px solid var(--ui-border-subtle);
		min-width: 180px;
	}

	.filter-section {
		padding: 10px 12px;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.section-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 6px;
	}

	.section-label {
		font-size: 10px;
		font-weight: 600;
		color: var(--ui-text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
		display: block;
		margin-bottom: 6px;
	}

	.section-header .section-label {
		margin-bottom: 0;
	}

	/* Search */
	.search-wrapper {
		position: relative;
	}

	.search-input {
		width: 100%;
		padding: 5px 28px 5px 8px;
		font-size: 12px;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		color: var(--ui-text-primary);
		outline: none;
		transition: border-color 150ms ease;
		box-sizing: border-box;
	}

	.search-input:focus {
		border-color: var(--ui-border-interactive, #4a9eff);
	}

	.search-input::placeholder {
		color: var(--ui-text-tertiary);
	}

	.clear-search {
		position: absolute;
		right: 4px;
		top: 50%;
		transform: translateY(-50%);
		width: 20px;
		height: 20px;
		border: none;
		background: transparent;
		color: var(--ui-text-tertiary);
		font-size: 14px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 3px;
	}

	.clear-search:hover {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-primary);
	}

	/* Quick-action buttons */
	.quick-actions {
		display: flex;
		gap: 4px;
	}

	.quick-btn {
		font-size: 10px;
		padding: 1px 6px;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 3px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-secondary);
		cursor: pointer;
		transition: background-color 150ms ease, border-color 150ms ease;
	}

	.quick-btn:hover {
		background: var(--ui-surface-tertiary);
		border-color: var(--ui-border-strong);
		color: var(--ui-text-primary);
	}

	/* Type checkboxes */
	.type-list {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.type-checkbox {
		display: flex;
		align-items: center;
		gap: 7px;
		padding: 4px 6px;
		border-radius: 4px;
		cursor: pointer;
		transition: background-color 150ms ease;
		font-size: 12px;
		color: var(--ui-text-secondary);
	}

	.type-checkbox:hover {
		background: var(--ui-surface-tertiary);
	}

	.type-checkbox.type-checked {
		color: var(--ui-text-primary);
	}

	.type-checkbox input[type='checkbox'] {
		/* Visually hidden but accessible */
		position: absolute;
		width: 1px;
		height: 1px;
		opacity: 0;
		margin: 0;
	}

	.type-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		border: 2px solid var(--type-color, #6b7280);
		flex-shrink: 0;
		background: transparent;
		transition: background-color 150ms ease;
	}

	.type-checked .type-dot {
		background: var(--type-color, #6b7280);
	}

	.type-name {
		text-transform: capitalize;
	}
</style>
