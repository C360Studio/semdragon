<script lang="ts">
	/**
	 * Workspace Page - Browse quest artifacts from the artifact store
	 *
	 * Two views:
	 * 1. Quest selector — shows quests that have artifacts
	 * 2. File browser — tree + preview for a selected quest's artifacts
	 *
	 * URL param ?quest={id} auto-selects a quest.
	 */

	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { worldStore } from '$stores/worldStore.svelte';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import CopyButton from '$components/CopyButton.svelte';
	import {
		getWorkspaceQuests,
		getWorkspaceTree,
		getWorkspaceFile,
		getArtifactsDownloadUrl,
		ApiError
	} from '$services/api';
	import type { WorkspaceEntry, WorkspaceQuest } from '$services/api';
	import { extractInstance } from '$types';

	// Panel state
	let leftPanelOpen = $state(true);
	let rightPanelOpen = $state(false);
	let leftPanelWidth = $state(280);
	let rightPanelWidth = $state(320);

	// Quest list state
	let quests = $state<WorkspaceQuest[]>([]);
	let questsLoading = $state(true);
	let questsError = $state<string | null>(null);

	// Selected quest
	let selectedQuestId = $state<string | null>(null);

	// File tree state
	let tree = $state<WorkspaceEntry[]>([]);
	let treeLoading = $state(false);
	let treeError = $state<string | null>(null);

	// File preview state
	let selectedPath = $state<string | null>(null);
	let selectedEntry = $state<WorkspaceEntry | null>(null);
	let fileContent = $state<string | null>(null);
	let fileLoading = $state(false);
	let fileError = $state<string | null>(null);

	// Track expanded directories
	let expanded = $state(new Set<string>());

	// Load quest list on mount
	$effect(() => {
		untrack(() => loadQuests());
	});

	// Auto-select quest from URL param
	$effect(() => {
		const questParam = page.url.searchParams.get('quest');
		if (questParam && !selectedQuestId) {
			selectQuest(questParam);
		}
	});

	async function loadQuests() {
		questsLoading = true;
		questsError = null;

		try {
			quests = await getWorkspaceQuests();
		} catch (err) {
			if (err instanceof ApiError && err.status === 503) {
				questsError = 'Artifact storage not available';
			} else {
				questsError = err instanceof Error ? err.message : 'Failed to load workspace';
			}
		} finally {
			questsLoading = false;
		}
	}

	async function selectQuest(questId: string) {
		selectedQuestId = questId;
		selectedPath = null;
		selectedEntry = null;
		fileContent = null;
		fileError = null;
		expanded = new Set();
		treeLoading = true;
		treeError = null;

		try {
			tree = await getWorkspaceTree(questId);
		} catch (err) {
			treeError = err instanceof Error ? err.message : 'Failed to load artifacts';
		} finally {
			treeLoading = false;
		}
	}

	function backToQuests() {
		selectedQuestId = null;
		tree = [];
		selectedPath = null;
		selectedEntry = null;
		fileContent = null;
	}

	async function selectFile(entry: WorkspaceEntry) {
		if (entry.is_dir) {
			toggleDir(entry.path);
			return;
		}

		if (!selectedQuestId) return;

		selectedPath = entry.path;
		selectedEntry = entry;
		fileContent = null;
		fileError = null;
		fileLoading = true;

		try {
			fileContent = await getWorkspaceFile(selectedQuestId, entry.path);
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

	function fileIcon(entry: WorkspaceEntry): string {
		if (entry.is_dir) return expanded.has(entry.path) ? 'v' : '>';
		const ext = fileExtension(entry.name);
		if (['go', 'py', 'js', 'ts', 'svelte', 'rs'].includes(ext)) return '#';
		if (['md', 'txt', 'log'].includes(ext)) return '=';
		if (['json', 'yaml', 'yml', 'toml'].includes(ext)) return '{';
		return '.';
	}

	function fileExtension(name: string): string {
		const dot = name.lastIndexOf('.');
		return dot > 0 ? name.substring(dot + 1).toLowerCase() : '';
	}

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

	// Resolve quest title/agent from worldStore for richer display
	function questTitle(q: WorkspaceQuest): string {
		if (q.title) return q.title;
		const quest = worldStore.quests.get(q.quest_id as any);
		return quest?.title ?? q.quest_id;
	}

	function questAgent(q: WorkspaceQuest): string {
		if (q.agent) {
			return worldStore.agentName(q.agent as any) || q.agent;
		}
		return '';
	}

	const selectedQuestTitle = $derived(() => {
		if (!selectedQuestId) return '';
		const q = quests.find((q) => q.quest_id === selectedQuestId);
		return q ? questTitle(q) : selectedQuestId;
	});

	// Group workspace quests into parent→children tree using worldStore quest data
	type QuestNode = { quest: WorkspaceQuest; children: WorkspaceQuest[] };

	const questTree = $derived.by((): QuestNode[] => {
		// Build a set of workspace quest instance IDs for fast lookup
		const workspaceIds = new Set(quests.map((q) => q.quest_id));

		// Identify parent quests and sub-quests
		const parentNodes = new Map<string, QuestNode>();
		const subQuests: WorkspaceQuest[] = [];

		for (const wq of quests) {
			// Try to find this quest in worldStore to check parent_quest
			const storeQuest = findStoreQuest(wq.quest_id);
			if (storeQuest?.parent_quest) {
				subQuests.push(wq);
			} else {
				parentNodes.set(wq.quest_id, { quest: wq, children: [] });
			}
		}

		// Attach sub-quests to their parents
		for (const sq of subQuests) {
			const storeQuest = findStoreQuest(sq.quest_id);
			if (!storeQuest?.parent_quest) continue;
			const parentInstanceId = extractInstance(storeQuest.parent_quest);
			const parent = parentNodes.get(parentInstanceId);
			if (parent) {
				parent.children.push(sq);
			} else {
				// Parent has no artifacts — show sub-quest as top-level
				parentNodes.set(sq.quest_id, { quest: sq, children: [] });
			}
		}

		return Array.from(parentNodes.values());
	});

	function findStoreQuest(instanceId: string) {
		// worldStore quests are keyed by full entity ID; match by instance suffix
		for (const [, q] of worldStore.quests) {
			if (extractInstance(q.id) === instanceId) return q;
		}
		return null;
	}

	// Track expanded parent quests in the quest selector
	let expandedParents = $state(new Set<string>());

	function toggleParent(questId: string) {
		const next = new Set(expandedParents);
		if (next.has(questId)) {
			next.delete(questId);
		} else {
			next.add(questId);
		}
		expandedParents = next;
	}
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
			{#if !selectedQuestId}
				<!-- Quest selector view -->
				<header class="workspace-header">
					<h1>Workspace</h1>
					{#if !questsLoading && !questsError}
						<span class="file-count">{questTree.length} quests</span>
					{/if}
					<button class="refresh-btn" onclick={loadQuests} aria-label="Refresh" disabled={questsLoading}>
						&#x21bb;
					</button>
				</header>

				{#if questsLoading}
					<div class="loading-state">
						<p>Loading workspace...</p>
					</div>
				{:else if questsError}
					<div class="error-state" role="alert">
						<p>{questsError}</p>
						<button class="retry-btn" onclick={loadQuests}>Retry</button>
					</div>
				{:else if quests.length === 0}
					<div class="empty-state" data-testid="workspace-empty">
						<div class="empty-icon">W</div>
						<h2>No Artifacts Yet</h2>
						<p>Completed quests will have their workspace artifacts snapshotted here for browsing.</p>
					</div>
				{:else}
					<div class="quest-list" data-testid="workspace-quest-list">
						{#each questTree as node (node.quest.quest_id)}
							<div class="quest-tree-node">
								<!-- svelte-ignore a11y_click_events_have_key_events -->
								<!-- svelte-ignore a11y_no_static_element_interactions -->
								<div
									class="quest-card"
									class:has-children={node.children.length > 0}
									onclick={() => selectQuest(node.quest.quest_id)}
									data-testid="workspace-quest-{node.quest.quest_id}"
								>
									{#if node.children.length > 0}
										<button
											class="expand-toggle"
											onclick={(e: MouseEvent) => { e.stopPropagation(); toggleParent(node.quest.quest_id); }}
											aria-label={expandedParents.has(node.quest.quest_id) ? 'Collapse' : 'Expand'}
											aria-expanded={expandedParents.has(node.quest.quest_id)}
										>
											{expandedParents.has(node.quest.quest_id) ? '\u25BE' : '\u25B8'}
										</button>
									{/if}
									<div class="quest-card-body">
										<div class="quest-card-header">
											<span class="quest-card-title">{questTitle(node.quest)}</span>
											{#if node.quest.status}
												<span class="status-badge" data-status={node.quest.status}>{node.quest.status}</span>
											{/if}
										</div>
										<div class="quest-card-meta">
											{#if questAgent(node.quest)}
												<span>{questAgent(node.quest)}</span>
											{/if}
											<span>{node.quest.file_count} files</span>
											{#if node.children.length > 0}
												<span>{node.children.length} sub-quests</span>
											{/if}
										</div>
									</div>
								</div>
								{#if node.children.length > 0 && expandedParents.has(node.quest.quest_id)}
									<div class="sub-quest-list">
										{#each node.children as sub (sub.quest_id)}
											<button
												class="quest-card sub-quest"
												onclick={() => selectQuest(sub.quest_id)}
												data-testid="workspace-quest-{sub.quest_id}"
											>
												<div class="quest-card-body">
													<div class="quest-card-header">
														<span class="quest-card-title">{questTitle(sub)}</span>
														{#if sub.status}
															<span class="status-badge" data-status={sub.status}>{sub.status}</span>
														{/if}
													</div>
													<div class="quest-card-meta">
														{#if questAgent(sub)}
															<span>{questAgent(sub)}</span>
														{/if}
														<span>{sub.file_count} files</span>
													</div>
												</div>
											</button>
										{/each}
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			{:else}
				<!-- File browser view -->
				<header class="workspace-header">
					<button class="back-btn" onclick={backToQuests} aria-label="Back to quest list">
						&larr;
					</button>
					<h1>{selectedQuestTitle()}</h1>
					{#if !treeLoading && !treeError}
						<span class="file-count">{totalFiles} files</span>
					{/if}
					<a
						class="download-btn"
						href={getArtifactsDownloadUrl(selectedQuestId)}
						download
						aria-label="Download all artifacts as ZIP"
					>
						ZIP
					</a>
					<button class="refresh-btn" onclick={() => selectQuest(selectedQuestId!)} aria-label="Refresh" disabled={treeLoading}>
						&#x21bb;
					</button>
				</header>

				{#if treeLoading}
					<div class="loading-state">
						<p>Loading artifacts...</p>
					</div>
				{:else if treeError}
					<div class="error-state" role="alert">
						<p>{treeError}</p>
						<button class="retry-btn" onclick={() => selectQuest(selectedQuestId!)}>Retry</button>
					</div>
				{:else}
					<div class="workspace-content">
						<div class="file-tree" role="tree" aria-label="Artifact file tree" data-testid="workspace-tree">
							{#if tree.length === 0}
								<div class="empty-state">
									<p>No artifacts found for this quest.</p>
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
											{#if !entry.is_dir && entry.size}
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
									<span class="preview-path">{selectedEntry.path}<CopyButton text={selectedEntry.path} variant="inline" label="Copy file path" /></span>
								</header>
								<div class="copyable">
									<CopyButton text={fileContent} label="Copy file content" />
									<pre class="preview-content"><code>{fileContent}</code></pre>
								</div>
							{/if}
						</div>
					</div>
				{/if}
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
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.file-count {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		background: var(--ui-surface-tertiary);
		padding: 2px 8px;
		border-radius: var(--radius-full);
		flex-shrink: 0;
	}

	.back-btn {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		cursor: pointer;
		flex-shrink: 0;
	}

	.back-btn:hover {
		border-color: var(--ui-border-interactive);
	}

	.refresh-btn,
	.download-btn {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-sm);
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
		font-size: 0.625rem;
		font-weight: 600;
		cursor: pointer;
		transition: border-color 150ms ease;
		text-decoration: none;
		flex-shrink: 0;
	}

	.refresh-btn:hover:not(:disabled),
	.download-btn:hover {
		border-color: var(--ui-border-interactive);
	}

	.refresh-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* Quest list */
	.quest-list {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
	}

	.quest-tree-node {
		display: flex;
		flex-direction: column;
	}

	.quest-card {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		padding: var(--spacing-md);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		background: var(--ui-surface-secondary);
		text-align: left;
		cursor: pointer;
		transition: border-color 150ms ease;
		width: 100%;
		font-family: inherit;
		color: var(--ui-text-primary);
	}

	.quest-card:hover {
		border-color: var(--ui-border-interactive);
	}

	.quest-card.sub-quest {
		padding-left: calc(var(--spacing-md) + 20px);
		border-radius: 0;
		border-top: none;
	}

	.quest-card.sub-quest:last-child {
		border-radius: 0 0 var(--radius-md) var(--radius-md);
	}

	.quest-card-body {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: var(--spacing-xs);
		overflow: hidden;
	}

	.expand-toggle {
		width: 20px;
		height: 20px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: none;
		background: none;
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
		cursor: pointer;
		flex-shrink: 0;
		padding: 0;
		border-radius: var(--radius-sm);
	}

	.expand-toggle:hover {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-primary);
	}

	.sub-quest-list {
		display: flex;
		flex-direction: column;
		border-left: 2px solid var(--ui-border-subtle);
		margin-left: var(--spacing-lg);
	}

	.quest-card-header {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
	}

	.quest-card-title {
		flex: 1;
		font-weight: 600;
		font-size: 0.875rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.quest-card-meta {
		display: flex;
		gap: var(--spacing-md);
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.status-badge {
		padding: 2px 8px;
		border-radius: var(--radius-full);
		font-size: 0.6875rem;
		font-weight: 500;
		flex-shrink: 0;
	}

	.status-badge[data-status='completed'] {
		background: var(--quest-completed-container);
		color: var(--quest-completed);
	}
	.status-badge[data-status='failed'] {
		background: var(--quest-failed-container);
		color: var(--quest-failed);
	}
	.status-badge[data-status='in_review'] {
		background: var(--quest-in-review-container);
		color: var(--quest-in-review);
	}
	.status-badge[data-status='in_progress'] {
		background: var(--quest-in-progress-container);
		color: var(--quest-in-progress);
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
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}

	.copyable {
		position: relative;
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
