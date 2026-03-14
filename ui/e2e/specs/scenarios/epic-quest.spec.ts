import {
	test,
	expect,
	hasBackend,
	hasLLM,
	isMockLLM,
	retry
} from '../../fixtures/test-base';
import { postQuestViaDMChat } from '../../fixtures/scenario-helpers';

/**
 * Epic Quest — Tier 3 Scenario
 *
 * Exercises the full party quest DAG pipeline with a real-world prompt that
 * triggers multi-quest decomposition, party formation, sub-quest execution,
 * lead review, and rollup. Requires a real LLM — skipped for mock.
 *
 * The Meshtastic/OSH prompt reliably produces:
 * - Quest chain with depends_on relationships
 * - Party quests requiring DAG decomposition
 * - Multiple sub-quests with workspace tool usage
 *
 * @scenario @tier3
 */

test.describe.serial('Epic Quest Pipeline', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend with LLM'
		);
		test.skip(isMockLLM(), 'Epic tier requires real LLM — skip for mock');
	});

	test('epic quest: Meshtastic driver completes without escalation', async ({
		page,
		lifecycleApi
	}) => {
		test.setTimeout(900_000); // 15 minutes

		const startTime = Date.now();

		await test.step('post quest via DM chat', async () => {
			await page.goto('/');
			await page.waitForLoadState('domcontentloaded');

			await postQuestViaDMChat(
				page,
				'Create a Meshtastic driver for OpenSensorHub (OSH) so that clients ' +
					'can use the Connected Systems API of OSH to interact with the Meshtastic network.',
				{ timeout: 240_000 }
			);
		});

		await test.step('verify quests appear on board', async () => {
			await page.goto('/quests');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="quest-card"]').count(),
					{ timeout: 60_000, message: 'No quest cards on board after posting epic quest' }
				)
				.toBeGreaterThan(0);
		});

		await test.step('wait for all quests to reach terminal state', async () => {
			await retry(
				async () => {
					const quests = await lifecycleApi.listQuests();
					const active = quests.filter((q) =>
						['posted', 'claimed', 'in_progress', 'in_review'].includes(q.status)
					);

					if (active.length > 0) {
						const summary = active
							.map((q) => `${q.title?.slice(0, 40)}:${q.status}`)
							.join(', ');
						throw new Error(
							`${active.length} quests still active: ${summary}`
						);
					}

					return quests;
				},
				{
					timeout: 840_000,
					interval: 10_000,
					message: 'Epic quest pipeline did not complete within timeout'
				}
			);
		});

		await test.step('verify no escalations', async () => {
			const quests = await lifecycleApi.listQuests();
			const escalated = quests.filter((q) => q.status === 'escalated');
			const completed = quests.filter((q) => q.status === 'completed');
			const failed = quests.filter((q) => q.status === 'failed');
			const partyQuests = quests.filter((q) => q.party_required);
			const soloQuests = quests.filter((q) => !q.party_required);
			const elapsed = Math.round((Date.now() - startTime) / 1000);

			console.log(
				`[Epic] ${quests.length} quests in ${elapsed}s: ` +
					`${completed.length} completed, ${failed.length} failed, ` +
					`${escalated.length} escalated | ` +
					`${soloQuests.length} solo, ${partyQuests.length} party`
			);

			expect(escalated.length, 'No quests should escalate').toBe(0);
		});
	});
});
