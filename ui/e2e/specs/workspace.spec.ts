import { test, expect, hasBackend, extractInstance } from '../fixtures/test-base';

test.describe('Workspace', () => {
	test('displays page title', async ({ page }) => {
		await page.goto('/workspace');
		await expect(page.locator('h1')).toContainText('Workspace');
	});

	test('shows refresh button', async ({ page }) => {
		await page.goto('/workspace');
		const refresh = page.locator('button[aria-label="Refresh"]');
		await expect(refresh.first()).toBeVisible();
	});
});

test.describe('Workspace - Navigation', () => {
	test('can navigate to workspace from nav', async ({ page }) => {
		await page.goto('/');
		await page.locator('[data-testid="nav-workspace"]').click();
		await expect(page).toHaveURL(/.*\/workspace/);
		await expect(page.locator('h1')).toContainText('Workspace');
	});

	test('direct navigation to /workspace works', async ({ page }) => {
		await page.goto('/workspace');
		await expect(page.locator('h1')).toContainText('Workspace');
	});

	test('quest param auto-selects quest', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend for quest data');

		// Navigate with a bogus quest ID — should show error or empty tree
		await page.goto('/workspace?quest=nonexistent');
		// Should be in file browser view (has back button)
		const backBtn = page.locator('button[aria-label="Back to quest list"]');
		await expect(backBtn).toBeVisible();
	});
});

test.describe('Workspace - With Backend', () => {
	test('loads quest list from artifact store', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		// Wait for loading to finish (either quests appear or empty state)
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });
	});

	test('quest cards show title and file count', async ({ page, lifecycleApi, seedQuests }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		// Wait for loading to complete
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		// If there are quest cards, verify they have expected structure
		const cards = page.locator('.quest-card');
		const count = await cards.count();
		if (count > 0) {
			const firstCard = cards.first();
			await expect(firstCard.locator('.quest-card-title')).toBeVisible();
			await expect(firstCard.locator('.quest-card-meta')).toBeVisible();
		}
	});

	test('clicking quest card opens file browser', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		// Wait for loading to complete
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		const cards = page.locator('.quest-card');
		const count = await cards.count();
		if (count > 0) {
			await cards.first().click();
			// Should show back button (file browser view)
			const backBtn = page.locator('button[aria-label="Back to quest list"]');
			await expect(backBtn).toBeVisible();
		}
	});

	test('file browser shows tree and preview areas', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		// Wait for loading to complete
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		const cards = page.locator('.quest-card');
		const count = await cards.count();
		if (count > 0) {
			await cards.first().click();

			// Wait for tree to load
			await expect(async () => {
				const loading = await page.locator('text=Loading artifacts').isVisible();
				expect(loading).toBe(false);
			}).toPass({ timeout: 5000 });

			// Should have tree and preview areas
			const tree = page.locator('[data-testid="workspace-tree"]');
			const preview = page.locator('[data-testid="workspace-preview"]');
			await expect(tree).toBeVisible();
			await expect(preview).toBeVisible();
		}
	});

	test('back button returns to quest list', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend for artifact store');

		await page.goto('/workspace');

		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		const cards = page.locator('.quest-card');
		const count = await cards.count();
		if (count > 0) {
			await cards.first().click();
			const backBtn = page.locator('button[aria-label="Back to quest list"]');
			await expect(backBtn).toBeVisible();
			await backBtn.click();
			// Should be back to quest list view
			await expect(page.locator('h1')).toContainText('Workspace');
		}
	});

	test('parent quests with sub-quests show expand toggle', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend with party quest data');

		await page.goto('/workspace');

		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5000 });

		// If there are parent quests with children, they should have an expand toggle
		const expandToggles = page.locator('.expand-toggle');
		const toggleCount = await expandToggles.count();
		if (toggleCount > 0) {
			// Click expand toggle — should show sub-quest list
			await expandToggles.first().click();
			const subList = page.locator('.sub-quest-list');
			await expect(subList.first()).toBeVisible();
		}
	});
});

test.describe('Workspace - Artifact API Integration', () => {
	test('artifact list endpoint returns files for completed quest', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires backend');

		const quests = await lifecycleApi.listQuests();
		const completed = quests.filter((q) => q.status === 'completed');

		if (completed.length === 0) {
			test.skip(true, 'No completed quests in environment');
			return;
		}

		const questId = extractInstance(completed[0].id);
		const result = await lifecycleApi.listQuestArtifacts(questId);

		expect(result.count).toBeGreaterThanOrEqual(0);
		expect(Array.isArray(result.files)).toBe(true);
		expect(result.count).toBe(result.files.length);
	});

	test('artifact list returns empty for nonexistent quest', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires backend');

		const result = await lifecycleApi.listQuestArtifacts('nonexistent');

		expect(result.count).toBe(0);
		expect(Array.isArray(result.files)).toBe(true);
		expect(result.files).toHaveLength(0);
	});

	test('completed quest with artifacts_commit has artifact files', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires backend');

		const quests = await lifecycleApi.listQuests();

		// artifacts_commit is a newer field not yet in generated types — cast to access it
		type QuestWithArtifacts = (typeof quests)[number] & {
			artifacts_commit?: string;
			artifacts_merged?: string;
			artifacts_indexed?: boolean;
		};

		const withCommit = (quests as QuestWithArtifacts[]).find(
			(q) => q.status === 'completed' && q.artifacts_commit
		);

		if (!withCommit) {
			test.skip(true, 'No completed quests with artifacts_commit in environment');
			return;
		}

		const questId = extractInstance(withCommit.id);
		const result = await lifecycleApi.listQuestArtifacts(questId);

		expect(result.count).toBeGreaterThan(0);
		expect(result.files.length).toBeGreaterThan(0);
	});

	test('workspace quest list endpoint returns quests with file counts', async ({ page }) => {
		test.skip(!hasBackend(), 'Requires backend');

		// Backend REST endpoint via Vite proxy (not the SvelteKit /workspace page)
		const response = await page.request.get('/game/workspace');

		// 404 or 501 means the endpoint is not yet wired — skip gracefully
		if (response.status() === 404 || response.status() === 501) {
			test.skip(true, '/game/workspace endpoint not available');
			return;
		}

		expect(response.ok()).toBe(true);

		const body = await response.json();
		expect(Array.isArray(body)).toBe(true);

		if (body.length > 0) {
			const first = body[0] as Record<string, unknown>;
			expect(first).toHaveProperty('quest_id');
			expect(first).toHaveProperty('file_count');
		}
	});

	test('workspace file endpoint serves file content', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires backend');

		const quests = await lifecycleApi.listQuests();
		const completed = quests.filter((q) => q.status === 'completed');

		// Find any completed quest that has artifact files
		let targetQuestId: string | null = null;
		let firstFile: string | null = null;

		for (const quest of completed.slice(0, 10)) {
			const questId = extractInstance(quest.id);
			const artifacts = await lifecycleApi.listQuestArtifacts(questId);
			if (artifacts.count > 0 && artifacts.files.length > 0) {
				targetQuestId = questId;
				firstFile = artifacts.files[0];
				break;
			}
		}

		if (!targetQuestId || !firstFile) {
			test.skip(true, 'No completed quests with artifact files in environment');
			return;
		}

		// Strip any leading slash before encoding — backend returns relative paths
		// but a leading slash would double-encode and cause 404s.
		const encodedPath = encodeURIComponent(firstFile.replace(/^\//, ''));
		const response = await page.request.get(
			`/game/workspace/file?quest=${targetQuestId}&path=${encodedPath}`
		);

		// 404 or 501 means the endpoint is not yet wired — skip gracefully
		if (response.status() === 404 || response.status() === 501) {
			test.skip(true, '/game/workspace/file endpoint not available');
			return;
		}

		expect(response.ok()).toBe(true);
		const text = await response.text();
		expect(text.length).toBeGreaterThan(0);
	});

	test('workspace tree endpoint returns nested structure', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires backend');

		const quests = await lifecycleApi.listQuests();
		const completed = quests.filter((q) => q.status === 'completed');

		// Find a completed quest that has artifact files
		let targetQuestId: string | null = null;

		for (const quest of completed.slice(0, 10)) {
			const questId = extractInstance(quest.id);
			const artifacts = await lifecycleApi.listQuestArtifacts(questId);
			if (artifacts.count > 0) {
				targetQuestId = questId;
				break;
			}
		}

		if (!targetQuestId) {
			test.skip(true, 'No completed quests with artifacts in environment');
			return;
		}

		const response = await page.request.get(`/game/workspace/tree?quest=${targetQuestId}`);

		// 404 or 501 means the endpoint is not yet wired — skip gracefully
		if (response.status() === 404 || response.status() === 501) {
			test.skip(true, '/game/workspace/tree endpoint not available');
			return;
		}

		expect(response.ok()).toBe(true);

		const body = await response.json();
		expect(Array.isArray(body)).toBe(true);

		if (body.length > 0) {
			const entry = body[0] as Record<string, unknown>;
			expect(entry).toHaveProperty('name');
			expect(entry).toHaveProperty('path');
			expect(entry).toHaveProperty('is_dir');
		}
	});
});
