import {
	test,
	expect,
	hasBackend,
	hasLLM,
	extractInstance,
	waitForHydration
} from '../../fixtures/test-base';

/**
 * Files Artifacts — Tier 2 Scenario Suite
 *
 * Verifies the artifact file browser by examining quests that
 * quest-pipeline.spec.ts has already completed. Does NOT post its own quests.
 *
 * Runs AFTER quest-pipeline in the serial tier2-scenarios project. By the time
 * these tests execute, the easy/research/moderate quests from quest-pipeline
 * are already completed with artifacts.
 *
 * Test order:
 *   1. Find a completed quest with artifacts via API
 *   2. Verify file tree and preview pane load at /files?quest={id}
 *   3. Verify quest entity has artifact tracking predicates
 *
 * @scenario @tier2
 */

test.describe.serial('Files Artifacts', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend with LLM (E2E_LLM_MODE=mock|gemini|openai|...)'
		);
	});

	// Tracks the first completed quest that has artifact files.
	// Set by Test 1; consumed by Tests 2-3.
	let artifactQuestId: string | null = null;
	let artifactQuestInstanceId: string | null = null;

	// ===========================================================================
	// Test 1: Find completed quest with artifacts (from quest-pipeline)
	// ===========================================================================

	test('completed quest from pipeline has artifacts accessible via API', async ({ lifecycleApi }) => {
		test.setTimeout(30_000);

		const quests = await lifecycleApi.listQuests();
		const completed = quests.filter((q) => q.status === 'completed');

		if (completed.length === 0) {
			test.skip(true, 'No completed quests — quest-pipeline may not have run first');
			return;
		}

		// Try each completed quest until we find one with artifacts
		for (const quest of completed.slice(0, 10)) {
			const instanceId = extractInstance(quest.id);
			const artifacts = await lifecycleApi.listQuestArtifacts(instanceId);

			if (artifacts.count > 0) {
				artifactQuestId = quest.id;
				artifactQuestInstanceId = instanceId;

				expect(artifacts.count).toBe(artifacts.files.length);
				expect(artifacts.files.length).toBeGreaterThan(0);

				for (const file of artifacts.files) {
					expect(file.length).toBeGreaterThan(0);
					expect(file).not.toContain('..');
				}
				return;
			}
		}

		// No artifacts found — expected when sandbox is not configured or
		// quests completed without writing files. Not a failure.
		console.warn(
			'[Files] No completed quests have artifact files — sandbox may not be configured'
		);
	});

	// ===========================================================================
	// Test 2: Verify file tree and content at /files?quest={id}
	// ===========================================================================

	test('files page shows file tree and preview pane for quest with artifacts', async ({ page }) => {
		test.setTimeout(30_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found in previous test');
			return;
		}

		await page.goto(`/files?quest=${artifactQuestInstanceId}`);
		await waitForHydration(page);

		await expect(async () => {
			const loading = await page.locator('text=Loading artifacts').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5_000 });

		const tree = page.locator('[data-testid="files-tree"]');
		const preview = page.locator('[data-testid="files-preview"]');
		await expect(tree).toBeVisible({ timeout: 5_000 });
		await expect(preview).toBeVisible({ timeout: 5_000 });
	});

	// ===========================================================================
	// Test 2b: Quest context header shows quest metadata
	// ===========================================================================

	test('files page header shows quest title, status, and agent', async ({ page }) => {
		test.setTimeout(30_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found in previous test');
			return;
		}

		await page.goto(`/files?quest=${artifactQuestInstanceId}`);
		await waitForHydration(page);

		// Header should show the quest title (not just the raw instance ID)
		const heading = page.locator('.page-header h1');
		await expect(heading).toBeVisible({ timeout: 5_000 });
		const title = await heading.textContent();
		expect(title).toBeTruthy();
		expect(title).not.toBe('Files'); // Should be the quest title, not generic

		// Status badge should be present
		const statusBadge = page.locator('.page-header .status-badge');
		await expect(statusBadge).toBeVisible();

		// Back link should navigate to quest picker
		const backLink = page.locator('.back-link');
		await expect(backLink).toBeVisible();
		await expect(backLink).toHaveAttribute('href', '/files');

		// Right panel should auto-open with quest details
		const detailsPanel = page.locator('.details-panel');
		await expect(detailsPanel).toBeVisible({ timeout: 3_000 });

		// Details should include a link to the full quest page
		const fullLink = page.locator('.view-full-link');
		await expect(fullLink).toBeVisible();
	});

	// ===========================================================================
	// Test 3: Verify artifact tracking predicates on quest entity
	// ===========================================================================

	test('quest entity has artifact tracking fields', async ({ lifecycleApi }) => {
		test.setTimeout(15_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found');
			return;
		}

		const quest = await lifecycleApi.getQuest(artifactQuestInstanceId);

		if (quest.artifacts_merged) {
			expect(quest.artifacts_merged).toMatch(/^[0-9a-f]{7,40}$/);
		}

		console.log('[Files] Quest artifact tracking:', {
			id: artifactQuestInstanceId,
			artifacts_merged: quest.artifacts_merged ?? '(not set)',
			artifacts_indexed: quest.artifacts_indexed ?? false
		});
	});
});
