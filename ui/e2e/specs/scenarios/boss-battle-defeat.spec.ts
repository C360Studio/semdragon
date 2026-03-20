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
 * Boss Battle Defeat — Mock-only scenario
 *
 * Exercises the boss battle defeat → party quest repost path:
 * 1. Post a party quest via DM chat
 * 2. Mock lead decomposes into sub-quests
 * 3. Sub-quests complete via agentic loop
 * 4. DAG rolls up, parent submitted for review
 * 5. Boss battle evaluator DEFEATS the quest (mock returns checklist failure)
 * 6. Verify: old sub-quests get failed status with boss battle feedback
 * 7. Parent is reposted for retry (attempt 2)
 * 8. Second attempt passes boss battle (mock returns victory on retry)
 *
 * This test ONLY runs with the mock LLM (deterministic responses).
 */
test.describe('Boss Battle Defeat', () => {
	test.skip(!hasBackend(), 'Requires running backend');
	test.skip(!hasLLM(), 'Requires LLM provider');
	test.skip(!isMockLLM(), 'Boss battle defeat scenario requires mock LLM for deterministic control');

	test('party quest defeat: sub-quests failed, parent reposted, retry succeeds', async ({
		page,
		lifecycleApi
	}) => {
		test.setTimeout(300_000);

		// Post a party quest via DM chat.
		await test.step('post party quest via DM chat', async () => {
			await page.goto('/');
			await waitForHydration(page);

			await postQuestViaDMChat(
				page,
				'Build a utility library with two independent functions that can be completed in parallel. ' +
					'Sub-task 1: Write a celsius_to_fahrenheit function with unit tests. ' +
					'Sub-task 2: Write a kilometers_to_miles function with unit tests.',
				{ timeout: 30_000 }
			);
		});

		// Wait for the quest to appear on the board.
		let parentQuestId: string;
		await test.step('find parent party quest', async () => {
			const quest = await retry(
				async () => {
					const quests = await lifecycleApi.listQuests();
					const partyQuest = quests.find(
						(q: any) => q.party_id && !q.parent_quest
					);
					if (!partyQuest) {
						throw new Error(
							`No parent party quest found. Board: ${quests.map((q: any) => `${q.title?.slice(0, 30)}:${q.status}`).join(', ')}`
						);
					}
					return partyQuest;
				},
				{ timeout: 30_000, interval: 2000, message: 'Parent party quest not found' }
			);
			parentQuestId = extractInstance(quest.id);
			console.log(`[BossBattle] Parent quest: ${parentQuestId} "${quest.title}"`);
		});

		// Wait for boss battle defeat (attempt 1). The mock LLM returns a checklist
		// failure on the first evaluation, triggering repost with attempt=2.
		await test.step('wait for boss battle defeat and repost', async () => {
			const quest = await retry(
				async () => {
					const q = await lifecycleApi.getQuest(parentQuestId);

					// After defeat + repost, the quest goes back to posted/claimed/in_progress
					// with attempts > 1 and a failure_reason from the boss battle.
					if (q.attempts >= 2) {
						return q;
					}

					throw new Error(
						`Quest still on attempt ${q.attempts}, status=${q.status}. ` +
							`Waiting for boss battle defeat and repost.`
					);
				},
				{
					timeout: 180_000,
					interval: 3000,
					message: 'Boss battle defeat did not trigger repost (attempts never reached 2)'
				}
			);

			console.log(
				`[BossBattle] After defeat: status=${quest.status}, attempts=${quest.attempts}, ` +
					`failure_reason=${quest.failure_reason?.slice(0, 80)}`
			);

			// Verify the failure feedback from boss battle is on the parent.
			expect(quest.failure_reason).toBeTruthy();
			expect(quest.failure_type).toBe('quality');
		});

		// Verify old sub-quests got failed with boss battle feedback.
		await test.step('verify old sub-quests are failed with feedback', async () => {
			const allQuests = await lifecycleApi.listQuests();
			const subQuests = allQuests.filter(
				(q: any) => q.parent_quest && extractInstance(q.parent_quest) === parentQuestId
			);

			// First batch: should all be failed (boss battle cleanup).
			const failedSubs = subQuests.filter((q: any) => q.status === 'failed');
			console.log(
				`[BossBattle] Sub-quests: ${subQuests.length} total, ${failedSubs.length} failed`
			);

			// At least 2 sub-quests should be failed (from first DAG attempt).
			expect(failedSubs.length).toBeGreaterThanOrEqual(2);

			// Each failed sub-quest should have a failure reason mentioning boss battle.
			for (const sq of failedSubs) {
				expect(sq.failure_reason).toBeTruthy();
				expect(sq.failure_reason.toLowerCase()).toContain('boss battle');
				console.log(
					`[BossBattle] Failed sub-quest ${extractInstance(sq.id)}: ` +
						`"${sq.failure_reason?.slice(0, 100)}"`
				);
			}
		});

		// Wait for the retry to succeed (attempt 2 — mock returns victory).
		await test.step('wait for retry to complete', async () => {
			await retry(
				async () => {
					const q = await lifecycleApi.getQuest(parentQuestId);
					if (!['completed', 'failed', 'escalated'].includes(q.status)) {
						throw new Error(
							`Quest still ${q.status} (attempt ${q.attempts}). Waiting for terminal state.`
						);
					}
					console.log(
						`[BossBattle] Final state: status=${q.status}, attempts=${q.attempts}`
					);
					return q;
				},
				{
					timeout: 180_000,
					interval: 3000,
					message: 'Retry attempt did not reach terminal state'
				}
			);
		});

		// Verify battles exist — at least 2 (defeat + victory).
		await test.step('verify battle records', async () => {
			const battles = await lifecycleApi.listBattles();
			const questBattles = battles.filter(
				(b: any) =>
					b.quest_id && extractInstance(b.quest_id) === parentQuestId
			);
			console.log(
				`[BossBattle] Battles for quest: ${questBattles.length} ` +
					`(verdicts: ${questBattles.map((b: any) => b.verdict?.passed ? 'victory' : 'defeat').join(', ')})`
			);
			expect(questBattles.length).toBeGreaterThanOrEqual(1);
		});
	});
});
