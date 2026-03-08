import { test, expect, hasBackend, hasLLM, extractInstance, retry } from '../fixtures/test-base';

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

	test('quest created without explicit review still triggers battle', async ({ lifecycleApi }) => {
		test.skip(!hasBackend() || !hasLLM(), 'Requires running backend and LLM');
		test.setTimeout(120_000);

		// All quests now go through review (PostQuest forces RequireReview=true).
		// Verify that even a quest created without explicit review_level gets a battle.
		const quest = await lifecycleApi.createQuest('E2E implicit-review quest', 1);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('battle-implicit-agent');
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// Poll for a battle matching this quest — should appear since all quests
		// now require review.
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
			{ timeout: 90000, interval: 1000, message: 'No battle was created for the implicit-review quest' }
		);

		expect(battle).toBeTruthy();
		expect(battle.id).toBeTruthy();
	});
});
