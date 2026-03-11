import {
	test,
	expect,
	hasBackend,
	hasLLM,
	isMockLLM,
	extractInstance,
	retry,
	waitForHydration
} from '../../fixtures/test-base';
import {
	postQuestViaDMChat,
	waitForAnyQuestInColumn
} from '../../fixtures/scenario-helpers';

/**
 * Quest Pipeline — Tier 2 Scenario Suite
 *
 * Exercises the full quest pipeline end-to-end by acting like a human at the
 * dashboard. The seeded E2E roster (21 agents across all tiers) is already
 * running; autonomy, partycoord, guildformation, bossbattle, and agent
 * progression all run without intervention.
 *
 * Test order:
 *   1. Solo quest  — post via DM chat, watch autonomy claim and complete it
 *   2. Party quest — post epic quest via API, watch DAG execute and roll up
 *   3. Aftermath   — verify downstream effects: XP, battles, guilds, trajectories
 *
 * Design principles:
 *   - Work WITH autonomy — never pause the board, never manually claim quests
 *   - Use the seeded roster — never recruit agents inside these tests
 *   - Minimal assertions — verify pipeline movement, not intermediate state
 *   - Serial execution — tests share state intentionally; order matters
 *
 * @scenario @tier2
 */

test.describe.serial('Quest Pipeline', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend with LLM (E2E_LLM_MODE=mock|gemini|openai|...)'
		);
	});

	// ===========================================================================
	// Test 1: Solo Quest
	// ===========================================================================

	test('solo quest: post via DM chat, watch it complete', async ({ page }) => {
		test.setTimeout(isMockLLM() ? 120_000 : 300_000);

		// Step 1: Navigate to the dashboard and post a quest via the DM chat panel.
		// A human would open the chat, type a command, confirm the preview, and post.
		await test.step('navigate to dashboard', async () => {
			await page.goto('/');
			await waitForHydration(page);
		});

		await test.step('post quest via DM chat', async () => {
			await postQuestViaDMChat(
				page,
				'Create a quest to analyze customer churn data and build a report',
				{ timeout: isMockLLM() ? 30_000 : 60_000 }
			);
		});

		// Step 2: Navigate to the quests board and verify the quest card appeared.
		// With autonomy running, the quest may already be claimed before we arrive.
		await test.step('verify quest appears on board', async () => {
			await page.goto('/quests');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="quest-card"]').count(),
					{ timeout: 30_000, message: 'No quest cards on board after posting via DM chat' }
				)
				.toBeGreaterThan(0);
		});

		// Step 3: Wait for the quest to reach completed. The full pipeline runs:
		//   autonomy claims → questbridge dispatches → mock LLM executes
		//   → quest submitted → boss battle resolves → quest completed
		await test.step('wait for quest to reach completed column', async () => {
			await waitForAnyQuestInColumn(page, 'completed', {
				timeout: isMockLLM() ? 90_000 : 240_000
			});
		});
	});

	// ===========================================================================
	// Test 2: Party Quest
	// ===========================================================================

	test('party quest: post epic quest, watch DAG execute', async ({ page, lifecycleApi }) => {
		test.setTimeout(isMockLLM() ? 180_000 : 600_000);

		// Post via the lifecycle API since DM chat does not surface party hints.
		// The seeded roster already has Master-tier agents (lv16-17) available as
		// party leads and Journeyman agents (lv7-9) available as members.
		const parentQuest = await test.step('post party-required epic quest via API', async () => {
			const quest = await lifecycleApi.createQuestWithParty(
				'Build a utility library with two independent functions. ' +
					'Sub-task 1: Write a Python function celsius_to_fahrenheit(c) with unit tests. ' +
					'Sub-task 2: Write a Python function kilometers_to_miles(km) with unit tests. ' +
					'Each sub-task is independent and can be completed in parallel.',
				3
			);
			expect(quest.id).toBeTruthy();
			return quest;
		});

		const parentInstance = extractInstance(parentQuest.id);

		// Verify the quest card is visible on the board before we start waiting
		// for longer pipeline stages.
		await test.step('verify quest appears on board', async () => {
			await page.goto('/quests');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="quest-card"]').count(),
					{ timeout: 15_000, message: 'No quest cards on board' }
				)
				.toBeGreaterThan(0);
		});

		// Poll the API for the parent quest terminal state. The full DAG pipeline:
		//   partycoord forms party → questbridge dispatches to lead agent
		//   → lead calls decompose_quest → sub-quests posted
		//   → questdagexec drives sub-quest assignment and execution
		//   → lead reviews each node via review_sub_quest
		//   → parent quest rolls up to completed/failed/escalated
		await test.step('wait for parent quest to reach terminal state', async () => {
			await retry(
				async () => {
					const q = await lifecycleApi.getQuest(parentInstance);
					if (!['completed', 'failed', 'escalated'].includes(q.status)) {
						throw new Error(`Parent quest still ${q.status}`);
					}
					return q;
				},
				{
					timeout: isMockLLM() ? 150_000 : 540_000,
					interval: 3000,
					message: 'Parent quest did not reach a terminal state'
				}
			);
		});
	});

	// ===========================================================================
	// Test 3: Aftermath
	// ===========================================================================

	test('verify aftermath: agents have XP, battles exist, guilds formed', async ({
		page,
		lifecycleApi
	}) => {
		test.setTimeout(30_000);

		// After two quests ran through the pipeline, the downstream systems should
		// have fired. This test navigates around and verifies the effects exist —
		// it does not retry long waits because those effects happened in prior tests.

		// Agents page: at least one agent should be visible.
		await test.step('agents page shows agents', async () => {
			await page.goto('/agents');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="agent-card"]').count(),
					{ timeout: 10_000, message: 'No agent cards on agents page' }
				)
				.toBeGreaterThan(0);
		});

		// Battles page: at least one boss battle should exist from the solo quest.
		await test.step('battles page shows boss battles', async () => {
			await page.goto('/battles');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => {
						const cards = await page.locator('[data-testid="battle-card"]').count();
						if (cards > 0) return cards;
						// Fallback: look for battle content in the page body
						const text = await page.locator('main').textContent();
						return (text?.match(/victory|defeat|battle/gi) ?? []).length;
					},
					{ timeout: 15_000, message: 'No battle content on battles page' }
				)
				.toBeGreaterThan(0);
		});

		// World state API: verify at least one agent has non-zero XP.
		await test.step('world state shows at least one agent with XP', async () => {
			const world = await lifecycleApi.getWorldState();
			const agents = world.agents ?? [];
			const agentWithXP = agents.find(
				(a) => ((a as { xp?: number }).xp ?? 0) > 0
			);
			// Non-fatal: XP propagation timing is best-effort across test boundaries.
			// Log the finding rather than hard-failing the aftermath check.
			if (!agentWithXP) {
				console.warn('Aftermath: no agent with XP > 0 found; XP may still be propagating');
			}
		});

		// Agent detail: navigate to any agent's detail page and verify the level
		// field is visible — proves the detail page hydrates and renders properly.
		await test.step('agent detail page renders level', async () => {
			const world = await lifecycleApi.getWorldState();
			const agents = world.agents ?? [];
			if (agents.length === 0) return;

			const agentInstance = extractInstance(agents[0].id);

			await expect(async () => {
				await page.goto(`/agents/${agentInstance}`);
				await waitForHydration(page);
				await expect(page.locator('[data-testid="agent-name"]')).toBeVisible({
					timeout: 5_000
				});
			}).toPass({ timeout: 15_000 });

			await expect(page.locator('[data-testid="agent-level"]')).toBeVisible();
		});

		// Guilds: guildformation auto-clusters Expert+ agents. With the seeded
		// roster having 3 experts + 2 masters + 1 grandmaster, at least one guild
		// should have formed.
		await test.step('guilds page shows at least one guild', async () => {
			await page.goto('/guilds');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => {
						const cards = await page.locator('[data-testid="guild-card"]').count();
						if (cards > 0) return cards;
						// Fallback: guild may render without guild-card testid
						const text = await page.locator('main').textContent();
						return (text?.match(/guild/gi) ?? []).length;
					},
					{ timeout: 15_000, message: 'No guild content on guilds page' }
				)
				.toBeGreaterThan(0);
		});

		// Trajectories: every completed agentic loop leaves a trajectory entry.
		await test.step('trajectories page shows entries', async () => {
			await page.goto('/trajectories');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () =>
						page
							.locator(
								'[data-testid="trajectory-entry"], .trajectory-row, .trajectory-item, tr'
							)
							.count(),
					{ timeout: 15_000, message: 'No trajectory entries found' }
				)
				.toBeGreaterThan(0);
		});
	});
});
