import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/**
 * Boss Battle - Auto Trigger
 *
 * When a quest requiring review is submitted, the backend should automatically
 * create a boss battle record. These tests verify that the battle appears in
 * GET /battles and that quests without review skip that path entirely.
 */
test.describe('Boss Battle - Auto Trigger', () => {
	test('submitting quest with review triggers battle', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Create a quest that requires human review
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E boss battle trigger quest',
			1
		);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		// 2. Find an idle agent and run through the lifecycle
		const world = await lifecycleApi.getWorldState();
		const allAgents = (world.agents ?? []) as Array<{ id: string; status: string }>;
		const idleAgent = allAgents.find((a) => a.status === 'idle');
		expect(idleAgent, 'No idle agent available').toBeTruthy();
		const agentInstance = extractInstance(idleAgent!.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		const submitRes = await lifecycleApi.submitQuest(questInstance, 'E2E boss battle output');
		expect(submitRes.ok, `submit failed: ${submitRes.status}`).toBeTruthy();

		// 3. Poll GET /battles until a battle matching this quest appears.
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
			{ timeout: 15000, interval: 1000, message: 'No battle was created for the reviewed quest' }
		);

		expect(battle).toBeTruthy();
		// The battle status may be 'active', 'victory', or similar — just verify it exists
		expect(battle.id).toBeTruthy();
	});

	test('no battle created for quest without review', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Capture existing battles before the test
		const battlesBefore = await lifecycleApi.listBattles();
		const battleIdsBefore = new Set(battlesBefore.map((b) => b.id));

		// 2. Create a quest without review
		const quest = await lifecycleApi.createQuest('E2E no-battle quest', 1);
		const questInstance = extractInstance(quest.id);

		// 3. Find an idle agent and complete the full lifecycle
		const world = await lifecycleApi.getWorldState();
		const allAgents = (world.agents ?? []) as Array<{ id: string; status: string }>;
		const idleAgent = allAgents.find((a) => a.status === 'idle');
		expect(idleAgent, 'No idle agent available').toBeTruthy();
		const agentInstance = extractInstance(idleAgent!.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		const submitRes = await lifecycleApi.submitQuest(questInstance, 'E2E no-review output');
		expect(submitRes.ok, `submit failed: ${submitRes.status}`).toBeTruthy();

		// 4. Wait for the quest to reach completed (confirming no review path taken)
		await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'completed') {
					throw new Error(`Expected completed, got ${q.status}`);
				}
			},
			{ timeout: 8000, interval: 500, message: 'Quest did not complete without review' }
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
