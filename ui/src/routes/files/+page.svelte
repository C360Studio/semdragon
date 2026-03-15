<script lang="ts">
	/**
	 * Files - Read-only artifact file browser for quest outputs
	 *
	 * Takes a ?quest={id} URL param. Shows a file tree and preview pane
	 * for the flat artifact list returned by listQuestArtifacts. No worldStore
	 * dependency — purely API-driven.
	 */

	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import CopyButton from '$components/CopyButton.svelte';
	import { listQuestArtifacts, getArtifactFile, getArtifactsDownloadUrl } from '$services/api';

	// ---------------------------------------------------------------------------
	// Types
	// ---------------------------------------------------------------------------

	interface FileNode {
		name: string;
		path: string;
		is_dir: boolean;
		children?: FileNode[];
	}

	// ---------------------------------------------------------------------------
	// Panel layout state
	// ---------------------------------------------------------------------------

	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// ---------------------------------------------------------------------------
	// Page state
	// ---------------------------------------------------------------------------

	let questId = $derived(page.url.searchParams.get('quest') ?? '');

	let fileTree = $state<FileNode[]>([]);
	let fileCount = $state(0);
	let selectedFile = $state<string | null>(null);
	let fileContent = $state<string | null>(null);

	let loadingTree = $state(false);
	let loadingFile = $state(false);
	let treeError = $state<string | null>(null);
	let fileError = $state<string | null>(null);

	// Expanded dir paths
	let expandedDirs = $state(new Set<string>());

	// ---------------------------------------------------------------------------
	// Tree builder from flat paths
	// ---------------------------------------------------------------------------

	function buildTree(paths: string[]): FileNode[] {
		const root: FileNode[] = [];
		const dirs = new Map<string, FileNode>();

		for (const path of paths.sort()) {
			const parts = path.split('/');
			let parent = root;
			let currentPath = '';

			for (let i = 0; i < parts.length; i++) {
				currentPath = currentPath ? `${currentPath}/${parts[i]}` : parts[i];
				const isLast = i === parts.length - 1;

				if (isLast) {
					parent.push({ name: parts[i], path: currentPath, is_dir: false });
				} else {
					let dir = dirs.get(currentPath);
					if (!dir) {
						dir = { name: parts[i], path: currentPath, is_dir: true, children: [] };
						dirs.set(currentPath, dir);
						parent.push(dir);
					}
					parent = dir.children!;
				}
			}
		}
		return root;
	}

	// ---------------------------------------------------------------------------
	// Load artifact list when questId changes
	// ---------------------------------------------------------------------------

	$effect(() => {
		const qid = questId;
		if (!qid) {
			untrack(() => {
				fileTree = [];
				fileCount = 0;
				selectedFile = null;
				fileContent = null;
				treeError = null;
			});
			return;
		}

		untrack(() => {
			loadingTree = true;
			treeError = null;
			fileTree = [];
			fileCount = 0;
			selectedFile = null;
			fileContent = null;
		});

		listQuestArtifacts(qid)
			.then((result) => {
				fileTree = buildTree(result.files);
				fileCount = result.count;
				loadingTree = false;
				// Auto-expand top-level directories
				const topDirs = result.files
					.map((f) => f.split('/')[0])
					.filter((p, i, arr) => arr.indexOf(p) === i && result.files.some((f) => f.startsWith(p + '/')));
				expandedDirs = new Set(topDirs);
			})
			.catch((err: unknown) => {
				treeError = err instanceof Error ? err.message : 'Failed to load artifacts';
				loadingTree = false;
			});
	});

	// ---------------------------------------------------------------------------
	// Load file content when a file is selected
	// ---------------------------------------------------------------------------

	$effect(() => {
		const path = selectedFile;
		const qid = questId;
		if (!path || !qid) {
			untrack(() => { fileContent = null; fileError = null; });
			return;
		}

		untrack(() => {
			loadingFile = true;
			fileError = null;
			fileContent = null;
		});

		getArtifactFile(qid, path)
			.then((content) => {
				fileContent = content;
				loadingFile = false;
			})
			.catch((err: unknown) => {
				fileError = err instanceof Error ? err.message : 'Failed to load file';
				loadingFile = false;
			});
	});

	// ---------------------------------------------------------------------------
	// Helpers
	// ---------------------------------------------------------------------------

	function toggleDir(path: string) {
		const next = new Set(expandedDirs);
		if (next.has(path)) {
			next.delete(path);
		} else {
			next.add(path);
		}
		expandedDirs = next;
	}

	function selectFile(path: string) {
		selectedFile = path;
	}

	function fileIcon(name: string): string {
		const ext = name.split('.').pop()?.toLowerCase() ?? '';
		if (['go', 'ts', 'js', 'py', 'rs', 'c', 'cpp', 'h', 'java', 'rb', 'sh', 'bash'].includes(ext)) return '\u2318';
		if (['md', 'txt', 'rst'].includes(ext)) return '\u2750';
		if (['json', 'yaml', 'yml', 'toml'].includes(ext)) return '\u2606';
		if (['html', 'svelte', 'vue', 'jsx', 'tsx'].includes(ext)) return '\u25C6';
		return '\u2022';
	}

	function isTextFile(name: string): boolean {
		const ext = name.split('.').pop()?.toLowerCase() ?? '';
		const textExts = [
			'go', 'ts', 'js', 'py', 'rs', 'c', 'cpp', 'h', 'java', 'rb', 'sh', 'bash',
			'md', 'txt', 'rst', 'json', 'yaml', 'yml', 'toml', 'html', 'svelte', 'vue',
			'jsx', 'tsx', 'css', 'scss', 'less', 'sql', 'xml', 'env', 'gitignore',
			'dockerfile', 'makefile', 'log', 'csv'
		];
		return textExts.includes(ext) || !name.includes('.');
	}

	function downloadUrl(): string {
		return getArtifactsDownloadUrl(questId);
	}
</script>

<svelte:head>
	<title>Files - Semdragons</title>
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

	{#snippet rightPanel()}
		<div class="right-panel-placeholder"></div>
	{/snippet}

	{#snippet centerPanel()}
		<div class="files-page">
			<header class="page-header">
				<div class="header-row">
					<h1>Files</h1>
					{#if questId && fileCount > 0}
						<div class="header-actions">
							<span class="file-count">{fileCount} file{fileCount === 1 ? '' : 's'}</span>
							<a href={downloadUrl()} class="download-btn" download aria-label="Download all as ZIP">
								Download ZIP
							</a>
						</div>
					{/if}
				</div>
				{#if questId}
					<p class="quest-id-label">Quest: <code>{questId}</code></p>
				{/if}
			</header>

			{#if !questId}
				<div class="empty-state no-quest" data-testid="files-empty">
					<p>Select a quest to browse its files.</p>
					<p>Navigate to a completed quest and click <strong>Browse files</strong>.</p>
				</div>

			{:else if loadingTree}
				<div class="loading-state">
					<p>Loading artifacts&hellip;</p>
				</div>

			{:else if treeError}
				<div class="error-state">
					<p>Failed to load artifacts: {treeError}</p>
				</div>

			{:else if fileTree.length === 0}
				<div class="empty-state" data-testid="files-empty">
					<p>No artifact files found for this quest.</p>
				</div>

			{:else}
				<div class="browser-split">
					<!-- File tree -->
					<aside class="file-tree" data-testid="files-tree" aria-label="File tree">
						{#snippet renderNode(node: FileNode)}
							{#if node.is_dir}
								<li class="tree-dir">
									<button
										class="tree-item dir-item"
										class:expanded={expandedDirs.has(node.path)}
										onclick={() => toggleDir(node.path)}
										aria-expanded={expandedDirs.has(node.path)}
									>
										<span class="tree-icon dir-icon">{expandedDirs.has(node.path) ? '\u25BC' : '\u25B6'}</span>
										<span class="tree-name">{node.name}</span>
									</button>
									{#if expandedDirs.has(node.path) && node.children}
										<ul class="tree-children">
											{#each node.children as child}
												{@render renderNode(child)}
											{/each}
										</ul>
									{/if}
								</li>
							{:else}
								<li class="tree-file">
									<button
										class="tree-item file-item"
										class:selected={selectedFile === node.path}
										onclick={() => selectFile(node.path)}
										title={node.path}
									>
										<span class="tree-icon">{fileIcon(node.name)}</span>
										<span class="tree-name">{node.name}</span>
									</button>
								</li>
							{/if}
						{/snippet}
						<ul class="tree-root">
							{#each fileTree as node}
								{@render renderNode(node)}
							{/each}
						</ul>
					</aside>

					<!-- Preview pane -->
					<div class="file-preview" data-testid="files-preview">
						{#if !selectedFile}
							<div class="preview-empty">
								<p>Select a file from the tree to preview its contents.</p>
							</div>

						{:else if loadingFile}
							<div class="preview-loading">
								<p>Loading&hellip;</p>
							</div>

						{:else if fileError}
							<div class="preview-error">
								<p>{fileError}</p>
							</div>

						{:else if fileContent !== null}
							<div class="preview-header">
								<span class="preview-path">{selectedFile}</span>
								<CopyButton text={fileContent} label="Copy file content" variant="inline" />
							</div>
							{#if isTextFile(selectedFile)}
								<pre class="preview-code copyable"><code>{fileContent}</code></pre>
							{:else}
								<div class="preview-binary">
									<p>Binary file — cannot preview.</p>
								</div>
							{/if}
						{/if}
					</div>
				</div>
			{/if}
		</div>
	{/snippet}
</ThreePanelLayout>

<style>
	/* Page shell */
	.files-page {
		height: 100%;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	/* Header */
	.page-header {
		padding: var(--spacing-md) var(--spacing-lg);
		border-bottom: 1px solid var(--ui-border-subtle);
		flex-shrink: 0;
	}

	.header-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--spacing-md);
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 600;
	}

	.header-actions {
		display: flex;
		align-items: center;
		gap: var(--spacing-md);
	}

	.file-count {
		font-size: 0.875rem;
		color: var(--ui-text-tertiary);
	}

	.download-btn {
		font-size: 0.8125rem;
		padding: 4px 12px;
		border-radius: var(--radius-md);
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-primary);
		text-decoration: none;
		border: 1px solid var(--ui-border-subtle);
		transition: background 150ms ease;
	}

	.download-btn:hover {
		background: var(--ui-surface-secondary);
		text-decoration: none;
	}

	.quest-id-label {
		margin: var(--spacing-xs) 0 0;
		font-size: 0.8125rem;
		color: var(--ui-text-tertiary);
	}

	.quest-id-label code {
		font-family: var(--font-mono, monospace);
		font-size: 0.8rem;
	}

	/* State views */
	.empty-state,
	.loading-state,
	.error-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		flex: 1;
		padding: var(--spacing-xl);
		color: var(--ui-text-secondary);
		text-align: center;
		gap: var(--spacing-sm);
	}

	.error-state {
		color: var(--status-error);
	}

	/* Browser layout */
	.browser-split {
		flex: 1;
		display: flex;
		overflow: hidden;
		min-height: 0;
	}

	/* File tree */
	.file-tree {
		width: 240px;
		min-width: 160px;
		border-right: 1px solid var(--ui-border-subtle);
		overflow-y: auto;
		flex-shrink: 0;
		padding: var(--spacing-sm) 0;
	}

	.tree-root,
	.tree-children {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.tree-children {
		padding-left: var(--spacing-md);
	}

	.tree-item {
		display: flex;
		align-items: center;
		gap: var(--spacing-xs);
		width: 100%;
		padding: 3px var(--spacing-md);
		background: none;
		border: none;
		cursor: pointer;
		font-size: 0.8125rem;
		color: var(--ui-text-primary);
		text-align: left;
		border-radius: var(--radius-sm);
		transition: background 100ms ease;
		white-space: nowrap;
		overflow: hidden;
	}

	.tree-item:hover {
		background: var(--ui-surface-tertiary);
	}

	.tree-item.selected {
		background: var(--ui-surface-tertiary);
		font-weight: 500;
	}

	.tree-icon {
		font-size: 0.75rem;
		flex-shrink: 0;
		color: var(--ui-text-tertiary);
		width: 14px;
		text-align: center;
	}

	.tree-name {
		overflow: hidden;
		text-overflow: ellipsis;
	}

	/* Preview pane */
	.file-preview {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		min-width: 0;
	}

	.preview-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-sm) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
		flex-shrink: 0;
		background: var(--ui-surface-secondary);
	}

	.preview-path {
		flex: 1;
		font-family: var(--font-mono, monospace);
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.preview-empty,
	.preview-loading,
	.preview-error,
	.preview-binary {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
		text-align: center;
		padding: var(--spacing-xl);
	}

	.preview-error {
		color: var(--status-error);
	}

	.preview-code {
		flex: 1;
		overflow: auto;
		margin: 0;
		padding: var(--spacing-md);
		font-family: var(--font-mono, monospace);
		font-size: 0.8125rem;
		line-height: 1.5;
		background: var(--ui-surface-primary);
		position: relative;
		white-space: pre;
	}

	.preview-code code {
		background: none;
		padding: 0;
		font-size: inherit;
	}
</style>
