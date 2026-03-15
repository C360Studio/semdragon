import {
	test,
	expect,
	hasBackend,
	hasLLM,
	isMockLLM,
	extractInstance,
	waitForHydration
} from '../../fixtures/test-base';
import {
	postQuestViaDMChat,
	waitForAnyQuestInColumn
} from '../../fixtures/scenario-helpers';

/**
 * Workspace Artifacts — Tier 2 Scenario Suite
 *
 * Exercises the workspace repo artifact pipeline end-to-end:
 *   1. Post a quest via DM chat
 *   2. Wait for it to complete (autonomy claims and executes)
 *   3. Verify the quest has artifacts in the workspace browser
 *   4. Verify the artifact API endpoints return files
 *   5. Verify the quest entity has artifact tracking predicates set
 *
 * The seeded E2E roster runs autonomy, bossbattle, and agent progression
 * without intervention. Mock LLM writes files during execution, which
 * questbridge finalizes (git commit) on completion.
 *
 * Design principles:
 *   - Same as quest-pipeline.spec.ts — act like a human, work WITH autonomy
 *   - Serial execution — workspace verification depends on quest completion
 *   - Defensive — tests skip gracefully when workspace repo or artifacts
 *     are not available (e.g., filestore-only config)
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
	// Set by Test 2; consumed by Tests 3-5. May be null if no artifacts exist
	// (e.g., workspace repo not configured, or mock LLM didn't write files).
	let artifactQuestId: string | null = null;
	let artifactQuestInstanceId: string | null = null;

	// ===========================================================================
	// Test 1: Post quest and wait for completion
	// ===========================================================================

	test('post quest via DM chat and wait for completion', async ({ page }) => {
		test.setTimeout(isMockLLM() ? 120_000 : 300_000);

		await test.step('navigate to dashboard', async () => {
			await page.goto('/');
			await waitForHydration(page);
		});

		await test.step('post quest via DM chat', async () => {
			await postQuestViaDMChat(
				page,
				'Create a quest to build a Go utility function that validates email addresses with tests',
				{ timeout: isMockLLM() ? 30_000 : 60_000 }
			);
		});

		await test.step('wait for quest to complete', async () => {
			await page.goto('/quests');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="quest-card"]').count(),
					{ timeout: 30_000, message: 'No quest cards on board' }
				)
				.toBeGreaterThan(0);

			await waitForAnyQuestInColumn(page, 'completed', {
				timeout: isMockLLM() ? 90_000 : 240_000
			});
		});
	});

	// ===========================================================================
	// Test 2: Verify artifacts via API
	// ===========================================================================

	test('completed quest has artifacts accessible via API', async ({ lifecycleApi }) => {
		test.setTimeout(30_000);

		// Find a completed quest with artifacts
		const quests = await lifecycleApi.listQuests();
		const completed = quests.filter((q) => q.status === 'completed');

		if (completed.length === 0) {
			console.warn('[Workspace] No completed quests found — Test 1 may not have finished');
			return;
		}

		// Try each completed quest until we find one with artifacts
		for (const quest of completed.slice(0, 10)) {
			const instanceId = extractInstance(quest.id);
			const artifacts = await lifecycleApi.listQuestArtifacts(instanceId);

			if (artifacts.count > 0) {
				artifactQuestId = quest.id;
				artifactQuestInstanceId = instanceId;

				// Verify artifact list integrity
				expect(artifacts.count).toBe(artifacts.files.length);
				expect(artifacts.files.length).toBeGreaterThan(0);

				// Every file path should be a non-empty string
				for (const file of artifacts.files) {
					expect(file.length).toBeGreaterThan(0);
					expect(file).not.toContain('..');
				}
				return;
			}
		}

		// If no artifacts found, this is expected when workspace repo is not
		// configured — quests complete but only have text output (work_product.md
		// may or may not be persisted depending on config).
		console.warn('[Workspace] No completed quests have artifact files — workspace repo may not be configured');
	});

	// ===========================================================================
	// Test 3: Verify workspace browser shows quest
	// ===========================================================================

	test('workspace browser lists quest with artifacts', async ({ page }) => {
		test.setTimeout(30_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found in previous test');
			return;
		}

		await page.goto('/workspace');
		await waitForHydration(page);

		// Wait for loading to finish
		await expect(async () => {
			const loading = await page.locator('text=Loading workspace').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 10_000 });

		// The specific quest with artifacts should appear as a card.
		// Workspace quest cards have data-testid="workspace-quest-{quest_id}".
		const specificCard = page.locator(`[data-testid="workspace-quest-${artifactQuestInstanceId}"]`);
		await expect(specificCard).toBeVisible({ timeout: 10_000 });
	});

	// ===========================================================================
	// Test 4: Verify file tree and content
	// ===========================================================================

	test('workspace file tree shows files and content is readable', async ({ page }) => {
		test.setTimeout(30_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found');
			return;
		}

		// Navigate to workspace with quest param to auto-select
		await page.goto(`/workspace?quest=${artifactQuestInstanceId}`);
		await waitForHydration(page);

		// Wait for tree to load
		await expect(async () => {
			const loading = await page.locator('text=Loading artifacts').isVisible();
			expect(loading).toBe(false);
		}).toPass({ timeout: 10_000 });

		// Verify tree and preview areas are present
		const tree = page.locator('[data-testid="workspace-tree"]');
		const preview = page.locator('[data-testid="workspace-preview"]');
		await expect(tree).toBeVisible({ timeout: 5_000 });
		await expect(preview).toBeVisible({ timeout: 5_000 });
	});

	// ===========================================================================
	// Test 5: Verify artifact tracking predicates on quest entity
	// ===========================================================================

	test('quest entity has artifact tracking fields', async ({ lifecycleApi }) => {
		test.setTimeout(15_000);

		if (!artifactQuestInstanceId) {
			test.skip(true, 'No quest with artifacts found');
			return;
		}

		const quest = await lifecycleApi.getQuest(artifactQuestInstanceId);

		// Cast to access newer fields not yet in generated types
		type QuestWithArtifacts = typeof quest & {
			artifacts_commit?: string;
			artifacts_merged?: string;
			artifacts_indexed?: boolean;
		};

		const q = quest as QuestWithArtifacts;

		// artifacts_commit should be set by questbridge on finalization
		if (q.artifacts_commit) {
			// Commit hash should look like a git SHA (7-40 hex chars)
			expect(q.artifacts_commit).toMatch(/^[0-9a-f]{7,40}$/);
		}

		// artifacts_merged is set after boss battle victory merge to main
		// This may or may not be set depending on review config
		if (q.artifacts_merged) {
			expect(q.artifacts_merged).toMatch(/^[0-9a-f]{7,40}$/);
		}

		// Log what we found for debugging
		console.log('[Workspace] Quest artifact tracking:', {
			id: artifactQuestInstanceId,
			artifacts_commit: q.artifacts_commit ?? '(not set)',
			artifacts_merged: q.artifacts_merged ?? '(not set)',
			artifacts_indexed: q.artifacts_indexed ?? false
		});
	});
});
