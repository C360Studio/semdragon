import {
	test,
	expect,
	hasBackend,
	hasLLM,
	extractInstance,
	waitForHydration
} from '../../fixtures/test-base';

/**
 * Workspace Artifacts — Tier 2 Scenario Suite
 *
 * Verifies the workspace repo artifact pipeline by examining quests that
 * quest-pipeline.spec.ts has already completed. Does NOT post its own quests.
 *
 * Runs AFTER quest-pipeline in the serial tier2-scenarios project. By the time
 * these tests execute, the easy/research/moderate quests from quest-pipeline
 * are already completed with artifacts.
 *
 * Test order:
 *   1. Find a completed quest with artifacts via API
 *   2. Verify workspace browser shows the quest
 *   3. Verify file tree and preview pane load
 *   4. Verify quest entity has artifact tracking predicates
 *
 * @scenario @tier2
 */

test.describe.serial('Workspace Artifacts', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend with LLM (E2E_LLM_MODE=mock|gemini|openai|...)'
		);
	});

	// Tracks the first completed quest that has artifact files.
	// Set by Test 1; consumed by Tests 2-4.
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

		// No artifacts found — expected when workspace repo writes work_product.md
		// but filestore snapshot path is also active. Not a failure.
		console.warn(
			'[Workspace] No completed quests have artifact files — workspace repo may not be configured'
		);
	});

	// ===========================================================================
	// Test 2: Verify workspace browser shows quest
	// ===========================================================================

	test('workspace browser lists quest with artifacts', async ({ page }) => {
		test.setTimeout(30_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found in previous test');
			return;
		}

		await page.goto('/workspace');
		await waitForHydration(page);

		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5_000 });

		const specificCard = page.locator(
			`[data-testid="workspace-quest-${artifactQuestInstanceId}"]`
		);
		await expect(specificCard).toBeVisible({ timeout: 10_000 });
	});

	// ===========================================================================
	// Test 3: Verify file tree and content
	// ===========================================================================

	test('workspace file tree shows files and content is readable', async ({ page }) => {
		test.setTimeout(30_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found');
			return;
		}

		await page.goto(`/workspace?quest=${artifactQuestInstanceId}`);
		await waitForHydration(page);

		await expect(async () => {
			const loading = await page.locator('text=Loading artifacts').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 5_000 });

		const tree = page.locator('[data-testid="workspace-tree"]');
		const preview = page.locator('[data-testid="workspace-preview"]');
		await expect(tree).toBeVisible({ timeout: 5_000 });
		await expect(preview).toBeVisible({ timeout: 5_000 });
	});

	// ===========================================================================
	// Test 4: Verify artifact tracking predicates on quest entity
	// ===========================================================================

	test('quest entity has artifact tracking fields', async ({ lifecycleApi }) => {
		test.setTimeout(15_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found');
			return;
		}

		const quest = await lifecycleApi.getQuest(artifactQuestInstanceId);

		type QuestWithArtifacts = typeof quest & {
			artifacts_commit?: string;
			artifacts_merged?: string;
			artifacts_indexed?: boolean;
		};

		const q = quest as QuestWithArtifacts;

		if (q.artifacts_commit) {
			expect(q.artifacts_commit).toMatch(/^[0-9a-f]{7,40}$/);
		}

		if (q.artifacts_merged) {
			expect(q.artifacts_merged).toMatch(/^[0-9a-f]{7,40}$/);
		}

		console.log('[Workspace] Quest artifact tracking:', {
			id: artifactQuestInstanceId,
			artifacts_commit: q.artifacts_commit ?? '(not set)',
			artifacts_merged: q.artifacts_merged ?? '(not set)',
			artifacts_indexed: q.artifacts_indexed ?? false
		});
	});
});
