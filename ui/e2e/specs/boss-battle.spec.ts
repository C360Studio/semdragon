import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/** Whether an LLM backend (mock or real) is available for the agentic pipeline. */
function hasLLM(): boolean {
	return process.env.E2E_MOCK_LLM === 'true' || process.env.E2E_REAL_LLM === 'true';
}

/**
 * Boss Battle - Auto Trigger
 *
 * When a quest requiring review is submitted, the backend should automatically
 * create a boss battle record. These tests verify that the battle appears in
 * GET /battles and that quests without review skip that path entirely.
 *
 * Each test recruits its own fresh agent to avoid race conditions with
 * parallel tests competing for shared seeded agents.
 */
test.describe('Boss Battle - Auto Trigger', () => {
	test('submitting quest with review triggers battle', async ({ lifecycleApi }) => {
		test.skip(!hasBackend() || !hasLLM(), 'Requires running backend and LLM');
		test.setTimeout(120_000);

		// 1. Create a quest that requires human review (difficulty=easy so fresh agent qualifies)
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E boss battle trigger quest',
			1
		);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		// 2. Recruit a fresh agent and run through the lifecycle
		const agent = await lifecycleApi.recruitAgent('battle-trigger-agent');
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// 3. The agentic loop processes the quest and questbridge transitions it
		//    to in_review. Poll GET /battles until a battle matching this quest appears.
		//    Depending on processing speed, the battle may be active or already
		//    resolved to victory — both are acceptable.
		const battle = await retry(
			async () => {
				const battles = await lifecycleApi.listBattles();
				const match = battles.find(
					(b) =>
						b.quest_id === quest.id ||
						extractInstance(b.quest_id ?? '') === questInstance
				);
				if (!match) {
					throw new Error('Battle for quest not found yet');
				}
				return match;
			},
			{ timeout: 90000, interval: 1000, message: 'No battle was created for the reviewed quest' }
		);

		expect(battle).toBeTruthy();
		// The battle status may be 'active', 'victory', or similar — just verify it exists
		expect(battle.id).toBeTruthy();
	});

	test('no battle created for quest without review', async ({ lifecycleApi }) => {
		test.skip(!hasBackend() || !hasLLM(), 'Requires running backend and LLM');
		test.setTimeout(120_000);

		// 1. Capture existing battles before the test
		const battlesBefore = await lifecycleApi.listBattles();
		const battleIdsBefore = new Set(battlesBefore.map((b) => b.id));

		// 2. Create a quest without review
		const quest = await lifecycleApi.createQuest('E2E no-battle quest', 1);
		const questInstance = extractInstance(quest.id);

		// 3. Recruit a fresh agent and complete the full lifecycle
		const agent = await lifecycleApi.recruitAgent('battle-nobattle-agent');
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// 4. The agentic loop processes the quest and auto-completes (no review).
		//    Wait for the quest to reach completed.
		await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'completed') {
					throw new Error(`Expected completed, got ${q.status}`);
				}
			},
			{ timeout: 90000, interval: 1000, message: 'Quest did not complete without review' }
		);

		// 5. Give the system a moment to process any async events, then check battles.
		//    No new battle should reference this quest.
		await new Promise((resolve) => setTimeout(resolve, 1000));

		const battlesAfter = await lifecycleApi.listBattles();
		const newBattles = battlesAfter.filter((b) => !battleIdsBefore.has(b.id));
		const relatedBattle = newBattles.find(
			(b) =>
				b.quest_id === quest.id ||
				extractInstance(b.quest_id ?? '') === questInstance
		);

		expect(
			relatedBattle,
			'A battle was unexpectedly created for a quest without review'
		).toBeUndefined();
	});
});
