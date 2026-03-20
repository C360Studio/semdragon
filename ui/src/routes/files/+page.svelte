<script lang="ts">
	/**
	 * Files - Read-only artifact file browser for quest outputs
	 *
	 * Two modes:
	 * 1. No ?quest param → quest picker showing quests that have had work done
	 * 2. ?quest={id} → file tree + preview for that quest's artifacts
	 *
	 * Quest picker uses worldStore (reactive to SSE). File listing and content
	 * use the artifact API (on-demand fetch through sandbox).
	 */

	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import ThreePanelLayout from '$components/layout/ThreePanelLayout.svelte';
	import ExplorerNav from '$components/layout/ExplorerNav.svelte';
	import CopyButton from '$components/CopyButton.svelte';
	import { listQuestArtifacts, getArtifactFile, getArtifactsDownloadUrl } from '$services/api';
	import { worldStore } from '$stores/worldStore.svelte';
	import { QuestDifficultyNames, extractInstance } from '$types';

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

	// Resolve full quest entity from worldStore for context display
	const currentQuest = $derived(
		questId ? worldStore.questList.find((q) => extractInstance(q.id) === questId) ?? null : null
	);
	const currentAgentName = $derived(
		currentQuest?.claimed_by ? worldStore.agentName(currentQuest.claimed_by) : null
	);
	const currentParentQuest = $derived(
		currentQuest?.parent_quest
			? worldStore.questList.find((q) => q.id === currentQuest.parent_quest) ?? null
			: null
	);

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
	// Quest picker (when no ?quest param)
	// ---------------------------------------------------------------------------

	interface QuestPickerItem {
		instanceId: string;
		questId: string;
		title: string;
		status: string;
		agentName: string;
		children: QuestPickerItem[];
	}

	const workStatuses = ['completed', 'failed', 'in_review', 'in_progress', 'escalated'];

	// Group quests: top-level parents with sub-quests nested underneath
	const workQuests = $derived.by(() => {
		const all = worldStore.questList.filter((q) => workStatuses.includes(q.status));
		const toItem = (q: typeof all[0]): QuestPickerItem => ({
			instanceId: extractInstance(q.id),
			questId: q.id,
			title: q.title ?? extractInstance(q.id),
			status: q.status,
			agentName: q.claimed_by ? worldStore.agentName(q.claimed_by) : '',
			children: []
		});

		// Index sub-quests by parent
		const childMap = new Map<string, QuestPickerItem[]>();
		const subQuestIds = new Set<string>();
		for (const q of all) {
			if (q.parent_quest) {
				subQuestIds.add(q.id);
				const parentId = String(q.parent_quest);
				const siblings = childMap.get(parentId) ?? [];
				siblings.push(toItem(q));
				childMap.set(parentId, siblings);
			}
		}

		// Build top-level list with children attached
		const result: QuestPickerItem[] = [];
		for (const q of all) {
			if (subQuestIds.has(q.id)) continue;
			const item = toItem(q);
			item.children = childMap.get(q.id) ?? [];
			result.push(item);
		}
		return result;
	});

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

	// Auto-open right panel when viewing a quest's files
	$effect(() => {
		if (questId && currentQuest) {
			untrack(() => { rightPanelOpen = true; });
		} else {
			untrack(() => { rightPanelOpen = false; });
		}
	});
</script>

<svelte:head>
	<title>{currentQuest ? `${currentQuest.title} Files` : 'Files'} - Semdragons</title>
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
		<div class="details-panel">
			<header class="panel-header">
				<h2>Quest Details</h2>
			</header>
			<div class="details-content">
				{#if currentQuest}
					{@const quest = currentQuest}
					<section class="detail-section">
						<p class="quest-description">{quest.description}</p>
						<dl class="detail-list">
							<dt>Status</dt>
							<dd><span class="status-badge" data-status={quest.status}>{quest.status}</span></dd>
							<dt>Difficulty</dt>
							<dd>{QuestDifficultyNames[quest.difficulty]}</dd>
							<dt>Base XP</dt>
							<dd>{quest.base_xp}</dd>
							<dt>Required Skills</dt>
							<dd>{quest.required_skills.join(', ') || 'None'}</dd>
							{#if quest.claimed_by}
								<dt>Claimed By</dt>
								<dd><a href="/agents/{quest.claimed_by}">{worldStore.agentName(quest.claimed_by)}</a></dd>
							{/if}
							<dt>Attempts</dt>
							<dd>{quest.attempts} / {quest.max_attempts}</dd>
							{#if quest.artifacts_merged}
								<dt>Merged</dt>
								<dd><code class="merge-hash">{quest.artifacts_merged.slice(0, 8)}</code></dd>
							{/if}
							{#if quest.loop_id}
								<dt>Trajectory</dt>
								<dd><a href="/trajectories/{quest.loop_id}">View</a></dd>
							{/if}
						</dl>

						{#if quest.failure_reason}
							<div class="escalation-block">
								<h4>{quest.status === 'failed' ? 'Failure' : 'Escalation'}</h4>
								<p class="escalation-reason-text">{quest.failure_reason}</p>
							</div>
						{/if}

						<a href="/quests/{quest.id}" class="view-full-link">View full quest</a>
					</section>
				{:else if questId}
					<p class="empty-state">Quest not found in world state.</p>
				{:else}
					<p class="empty-state">Select a quest to view details.</p>
				{/if}
			</div>
		</div>
	{/snippet}

	{#snippet centerPanel()}
		<div class="files-page">
			<header class="page-header">
				{#if questId && currentQuest}
					<a href="/files" class="back-link">&larr; All quests</a>
					<div class="header-row">
						<h1>{currentQuest.title}</h1>
						<div class="header-badges">
							<span class="status-badge" data-status={currentQuest.status}>{currentQuest.status}</span>
							<span class="difficulty-badge" data-difficulty={currentQuest.difficulty}>{QuestDifficultyNames[currentQuest.difficulty]}</span>
						</div>
					</div>
					<div class="header-meta">
						{#if currentAgentName}
							<span>{currentAgentName}</span>
						{/if}
						{#if currentQuest.artifacts_merged}
							<span class="merge-pill merged">Merged</span>
						{:else if currentQuest.status === 'in_progress'}
							<span class="merge-pill worktree">Worktree</span>
						{:else if currentQuest.status === 'completed' || currentQuest.status === 'in_review'}
							<span class="merge-pill worktree">Worktree</span>
						{/if}
						{#if fileCount > 0}
							<span class="file-count">{fileCount} file{fileCount === 1 ? '' : 's'}</span>
							<a href={downloadUrl()} class="download-btn" download>ZIP</a>
						{/if}
					</div>
					{#if currentParentQuest}
						<div class="breadcrumb">
							Part of: <a href="/files?quest={extractInstance(currentParentQuest.id)}">{currentParentQuest.title}</a>
						</div>
					{/if}
				{:else if questId}
					<a href="/files" class="back-link">&larr; All quests</a>
					<div class="header-row">
						<h1>Files</h1>
					</div>
					<p class="quest-id-label">Quest: <code>{questId}</code></p>
				{:else}
					<div class="header-row">
						<h1>Files</h1>
					</div>
				{/if}
			</header>

			{#if !questId}
				{#if workQuests.length > 0}
					<div class="quest-picker" data-testid="files-quest-list">
						{#each workQuests as q (q.instanceId)}
							<div class="quest-group" class:has-children={q.children.length > 0}>
								<a href="/files?quest={q.instanceId}" class="quest-pick-card">
									<div class="pick-header">
										<span class="pick-title">{q.title}</span>
										<span class="status-badge" data-status={q.status}>{q.status}</span>
									</div>
									{#if q.agentName}
										<div class="pick-meta"><span>{q.agentName}</span></div>
									{/if}
								</a>
								{#if q.children.length > 0}
									<div class="sub-quests">
										{#each q.children as sub (sub.instanceId)}
											<a href="/files?quest={sub.instanceId}" class="quest-pick-card sub-quest">
												<div class="pick-header">
													<span class="pick-title">{sub.title}</span>
													<span class="status-badge" data-status={sub.status}>{sub.status}</span>
												</div>
												{#if sub.agentName}
													<div class="pick-meta"><span>{sub.agentName}</span></div>
												{/if}
											</a>
										{/each}
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{:else}
					<div class="empty-state" data-testid="files-empty">
						<p>No quests with work yet.</p>
					</div>
				{/if}

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

	.back-link {
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
		text-decoration: none;
	}

	.back-link:hover {
		color: var(--ui-text-primary);
	}

	.header-badges {
		display: flex;
		gap: var(--spacing-xs);
		align-items: center;
	}

	.header-meta {
		display: flex;
		align-items: center;
		gap: var(--spacing-sm);
		margin-top: var(--spacing-xs);
		font-size: 0.8125rem;
		color: var(--ui-text-tertiary);
	}

	.merge-pill {
		font-size: 0.6875rem;
		padding: 1px 6px;
		border-radius: var(--radius-sm);
		font-weight: 500;
	}

	.merge-pill.merged {
		background: color-mix(in srgb, var(--status-success) 20%, transparent);
		color: var(--status-success);
	}

	.merge-pill.worktree {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
	}

	.breadcrumb {
		margin-top: var(--spacing-xs);
		font-size: 0.8125rem;
		color: var(--ui-text-tertiary);
	}

	.breadcrumb a {
		color: var(--ui-interactive-primary);
	}

	.difficulty-badge {
		font-size: 0.625rem;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		text-transform: uppercase;
		font-weight: 600;
	}

	.difficulty-badge[data-difficulty='0'] { background: var(--difficulty-trivial-container); color: var(--difficulty-trivial); }
	.difficulty-badge[data-difficulty='1'] { background: var(--difficulty-easy-container); color: var(--difficulty-easy); }
	.difficulty-badge[data-difficulty='2'] { background: var(--difficulty-moderate-container); color: var(--difficulty-moderate); }
	.difficulty-badge[data-difficulty='3'] { background: var(--difficulty-hard-container); color: var(--difficulty-hard); }
	.difficulty-badge[data-difficulty='4'] { background: var(--difficulty-epic-container); color: var(--difficulty-epic); }
	.difficulty-badge[data-difficulty='5'] { background: var(--difficulty-legendary-container); color: var(--difficulty-legendary); }

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

	/* Quest picker */
	.quest-picker {
		display: flex;
		flex-direction: column;
		gap: var(--spacing-sm);
		padding: var(--spacing-lg);
		overflow-y: auto;
		flex: 1;
	}

	.quest-pick-card {
		display: block;
		padding: var(--spacing-sm) var(--spacing-md);
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		text-decoration: none;
		color: var(--ui-text-primary);
		transition: background 100ms ease, border-color 100ms ease;
	}

	.quest-pick-card:hover {
		background: var(--ui-surface-tertiary);
		border-color: var(--ui-border-default);
		text-decoration: none;
	}

	.pick-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--spacing-sm);
	}

	.pick-title {
		font-size: 0.875rem;
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.status-badge {
		font-size: 0.6875rem;
		padding: 1px 6px;
		border-radius: var(--radius-sm);
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
		white-space: nowrap;
		text-transform: lowercase;
	}

	.status-badge[data-status='completed'] { color: var(--status-success); }
	.status-badge[data-status='failed'] { color: var(--status-error); }
	.status-badge[data-status='in_progress'] { color: var(--status-active); }
	.status-badge[data-status='escalated'] { color: var(--status-warning); }

	.pick-meta {
		font-size: 0.8125rem;
		color: var(--ui-text-tertiary);
		margin-top: 2px;
	}

	.quest-group.has-children {
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		overflow: hidden;
	}

	.quest-group.has-children > .quest-pick-card {
		border: none;
		border-radius: 0;
	}

	.sub-quests {
		border-top: 1px solid var(--ui-border-subtle);
		padding-left: var(--spacing-md);
	}

	.sub-quests .quest-pick-card {
		border: none;
		border-radius: 0;
		border-top: 1px solid var(--ui-border-subtle);
	}

	.sub-quests .quest-pick-card:first-child {
		border-top: none;
	}

	.sub-quest .pick-title {
		font-size: 0.8125rem;
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

	/* Details panel */
	.details-panel {
		height: 100%;
		display: flex;
		flex-direction: column;
	}

	.panel-header {
		padding: var(--spacing-sm) var(--spacing-md);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.panel-header h2 {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
	}

	.details-content {
		flex: 1;
		overflow-y: auto;
		padding: var(--spacing-md);
	}

	.quest-description {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		margin-bottom: var(--spacing-md);
	}

	.detail-list {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--spacing-xs) var(--spacing-md);
	}

	.detail-list dt {
		color: var(--ui-text-tertiary);
		font-size: 0.75rem;
	}

	.detail-list dd {
		margin: 0;
		font-size: 0.875rem;
	}

	.merge-hash {
		font-family: var(--font-mono, monospace);
		font-size: 0.8rem;
		color: var(--status-success);
	}

	.escalation-block {
		margin-top: var(--spacing-md);
		padding: var(--spacing-sm) var(--spacing-md);
		background: var(--ui-surface-secondary);
		border: 1px solid var(--quest-escalated, #e67e22);
		border-radius: var(--radius-md);
	}

	.escalation-block h4 {
		margin: 0 0 var(--spacing-xs);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--quest-escalated, #e67e22);
	}

	.escalation-reason-text {
		margin: 0;
		font-size: 0.8125rem;
		line-height: 1.4;
		color: var(--ui-text-secondary);
		white-space: pre-wrap;
	}

	.view-full-link {
		display: block;
		text-align: center;
		padding: var(--spacing-sm);
		margin-top: var(--spacing-md);
		font-size: 0.8125rem;
		color: var(--ui-interactive-primary);
	}
</style>
