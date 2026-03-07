<script lang="ts">
	/**
	 * Workspace Page - Read-only file browser for agent workspace artifacts
	 *
	 * Two-panel layout:
	 * - Left: ExplorerNav + file tree
	 * - Center: File preview with metadata
	 */

	import { untrack } from 'svelte';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import { getWorkspaceTree, getWorkspaceFile, ApiError } from '$services/api';
	import type { WorkspaceEntry } from '$services/api';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// Workspace state
	let tree = $state<WorkspaceEntry[]>([]);
	let loading = $state(true);
	let treeError = $state<string | null>(null);
	let notConfigured = $state(false);

	// File preview state
	let selectedPath = $state<string | null>(null);
	let selectedEntry = $state<WorkspaceEntry | null>(null);
	let fileContent = $state<string | null>(null);
	let fileLoading = $state(false);
	let fileError = $state<string | null>(null);

	// Track expanded directories
	let expanded = $state(new Set<string>());

	// Load workspace tree on mount — untrack prevents reactive re-runs if loadTree
	// reads reactive state internally; this is a one-shot mount-time call.
	$effect(() => {
		untrack(() => loadTree());
	});

	async function loadTree() {
		loading = true;
		treeError = null;
		notConfigured = false;

		try {
			tree = await getWorkspaceTree();
		} catch (err) {
			if (err instanceof ApiError && err.status === 404) {
				notConfigured = true;
			} else {
				treeError = err instanceof Error ? err.message : 'Failed to load workspace';
			}
		} finally {
			loading = false;
		}
	}

	async function selectFile(entry: WorkspaceEntry) {
		if (entry.is_dir) {
			toggleDir(entry.path);
			return;
		}

		selectedPath = entry.path;
		selectedEntry = entry;
		fileContent = null;
		fileError = null;
		fileLoading = true;

		try {
			fileContent = await getWorkspaceFile(entry.path);
		} catch (err) {
			if (err instanceof ApiError && err.status === 415) {
				fileError = 'Binary file — preview not available';
			} else if (err instanceof ApiError && err.status === 413) {
				fileError = 'File too large to preview (max 1 MB)';
			} else {
				fileError = err instanceof Error ? err.message : 'Failed to load file';
			}
		} finally {
			fileLoading = false;
		}
	}

	function toggleDir(path: string) {
		const next = new Set(expanded);
		if (next.has(path)) {
			next.delete(path);
		} else {
			next.add(path);
		}
		expanded = next;
	}

	function handleTreeKeyDown(e: KeyboardEvent, entry: WorkspaceEntry) {
		if (e.key === 'ArrowRight' && entry.is_dir && !expanded.has(entry.path)) {
			e.preventDefault();
			toggleDir(entry.path);
		} else if (e.key === 'ArrowLeft' && entry.is_dir && expanded.has(entry.path)) {
			e.preventDefault();
			toggleDir(entry.path);
		} else if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			selectFile(entry);
		}
	}

	function formatSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleString();
	}

	function fileExtension(name: string): string {
		const dot = name.lastIndexOf('.');
		return dot > 0 ? name.substring(dot + 1).toLowerCase() : '';
	}

	function fileIcon(entry: WorkspaceEntry): string {
		if (entry.is_dir) return expanded.has(entry.path) ? 'v' : '>';
		const ext = fileExtension(entry.name);
		if (['go', 'py', 'js', 'ts', 'svelte', 'rs'].includes(ext)) return '#';
		if (['md', 'txt', 'log'].includes(ext)) return '=';
		if (['json', 'yaml', 'yml', 'toml'].includes(ext)) return '{';
		return '.';
	}

	// Count total files in tree recursively
	function countFiles(entries: WorkspaceEntry[]): number {
		let count = 0;
		for (const e of entries) {
			if (e.is_dir && e.children) {
				count += countFiles(e.children);
			} else if (!e.is_dir) {
				count++;
			}
		}
		return count;
	}

	const totalFiles = $derived(countFiles(tree));
</script>

<svelte:head>
	<title>Workspace - Semdragons</title>
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
		<div class="workspace-main">
			<header class="workspace-header">
				<h1>Workspace</h1>
				{#if !loading && !notConfigured && !treeError}
					<span class="file-count">{totalFiles} files</span>
				{/if}
				<button class="refresh-btn" onclick={loadTree} aria-label="Refresh workspace" disabled={loading}>
					R
				</button>
			</header>

			{#if loading}
				<div class="loading-state">
					<p>Loading workspace...</p>
				</div>
			{:else if notConfigured}
				<div class="empty-state" data-testid="workspace-not-configured">
					<div class="empty-icon">W</div>
					<h2>No Workspace Configured</h2>
					<p>Set <code>workspace_dir</code> in your game service config to enable the file browser.</p>
				</div>
			{:else if treeError}
				<div class="error-state" role="alert">
					<p>{treeError}</p>
					<button class="retry-btn" onclick={loadTree}>Retry</button>
				</div>
			{:else}
				<div class="workspace-content">
					<div class="file-tree" role="tree" aria-label="Workspace file tree" data-testid="workspace-tree">
						{#if tree.length === 0}
							<div class="empty-state">
								<p>No workspace files yet. Agents will produce artifacts here when completing quests.</p>
							</div>
						{:else}
							{#snippet renderTree(entries: WorkspaceEntry[], depth: number)}
								{#each entries as entry (entry.path)}
									<button
										class="tree-item"
										role="treeitem"
										aria-level={depth + 1}
										aria-label="{entry.is_dir ? 'Directory' : 'File'}: {entry.name}"
										aria-expanded={entry.is_dir ? expanded.has(entry.path) : undefined}
										aria-selected={selectedPath === entry.path}
										class:selected={selectedPath === entry.path}
										class:directory={entry.is_dir}
										style="padding-left: {12 + depth * 16}px"
										onclick={() => selectFile(entry)}
										onkeydown={(e) => handleTreeKeyDown(e, entry)}
										data-testid="tree-item-{entry.path}"
									>
										<span class="tree-icon">{fileIcon(entry)}</span>
										<span class="tree-name">{entry.name}</span>
										{#if !entry.is_dir}
											<span class="tree-size">{formatSize(entry.size)}</span>
										{/if}
									</button>
									{#if entry.is_dir && expanded.has(entry.path) && entry.children}
										{@render renderTree(entry.children, depth + 1)}
									{/if}
								{/each}
							{/snippet}
							{@render renderTree(tree, 0)}
						{/if}
					</div>

					<div class="file-preview" data-testid="workspace-preview">
						{#if !selectedPath}
							<div class="preview-empty">
								<p>Select a file to preview</p>
							</div>
						{:else if fileLoading}
							<div class="preview-loading">
								<p>Loading...</p>
							</div>
						{:else if fileError}
							<div class="preview-error" role="alert">
								<p>{fileError}</p>
							</div>
						{:else if fileContent !== null && selectedEntry}
							<header class="preview-header">
								<span class="preview-path">{selectedEntry.path}</span>
								<span class="preview-meta">{formatSize(selectedEntry.size)} | {formatDate(selectedEntry.modified)}</span>
							</header>
							<pre class="preview-content"><code>{fileContent}</code></pre>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	{/snippet}

	{#snippet rightPanel()}
		<div></div>
	{/snippet}
</ThreePanelLayout>

<style>
	.workspace-main {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.workspace-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.workspace-header h1 {
		margin: 0;
		font-size: 1.25rem;
	}

	.file-count {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-tertiary);
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.refresh-btn {
		margin-left: auto;
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
		font-size: 0.75rem;
		font-weight: 600;
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

	/* Content area: tree on left, preview on right */
	.workspace-content {
		flex: 1;
		display: flex;
		overflow: hidden;
	}

	.file-tree {
		width: 280px;
		min-width: 200px;
		border-right: 1px solid var(--ui-border-subtle);
		overflow-y: auto;
		padding: var(--spacing-xs) 0;
	}

	.tree-item {
		display: flex;
		align-items: center;
		gap: var(--spacing-xs);
		width: 100%;
		padding: 4px 12px;
		border: none;
		background: none;
		color: var(--ui-text-primary);
		font-size: 0.8125rem;
		font-family: inherit;
		text-align: left;
		cursor: pointer;
		transition: background-color 100ms ease;
	}

	.tree-item:hover {
		background: var(--ui-surface-tertiary);
	}

	.tree-item.selected {
		background: var(--ui-surface-tertiary);
		font-weight: 600;
	}

	.tree-item.directory {
		font-weight: 500;
	}

	.tree-icon {
		width: 16px;
		text-align: center;
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
		font-weight: 600;
		flex-shrink: 0;
	}

	.tree-name {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.tree-size {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		flex-shrink: 0;
	}

	/* File preview */
	.file-preview {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.preview-empty,
	.preview-loading,
	.preview-error {
		display: flex;
		align-items: center;
		justify-content: center;
		flex: 1;
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
	}

	.preview-error {
		color: var(--ui-text-secondary);
	}

	.preview-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-tertiary);
		border-bottom: 1px solid var(--ui-border-subtle);
		gap: var(--spacing-md);
	}

	.preview-path {
		font-size: 0.8125rem;
		font-weight: 600;
		font-family: monospace;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.preview-meta {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.preview-content {
		flex: 1;
		overflow: auto;
		margin: 0;
		padding: var(--spacing-md);
		font-size: 0.8125rem;
		line-height: 1.5;
		background: var(--ui-surface-primary);
	}

	.preview-content code {
		font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
		white-space: pre;
	}

	/* Empty / error states */
	.loading-state,
	.error-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		flex: 1;
		padding: var(--spacing-xl);
		color: var(--ui-text-tertiary);
	}

	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		flex: 1;
		padding: var(--spacing-xl);
		text-align: center;
		color: var(--ui-text-secondary);
	}

	.empty-icon {
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

	.empty-state h2 {
		margin: 0 0 var(--spacing-sm);
		font-size: 1.125rem;
		color: var(--ui-text-primary);
	}

	.empty-state p {
		margin: 0;
		font-size: 0.875rem;
		max-width: 400px;
		line-height: 1.5;
	}

	.empty-state code {
		padding: 2px 6px;
		background: var(--ui-surface-tertiary);
		border-radius: var(--radius-sm);
		font-size: 0.8125rem;
	}

	.retry-btn {
		margin-top: var(--spacing-md);
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
</style>
