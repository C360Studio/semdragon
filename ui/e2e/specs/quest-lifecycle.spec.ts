import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/**
 * Quest Lifecycle - State Machine Tests
 *
 * Exercises quest state transitions that do NOT require agentic loop execution.
 * Tests that require LLM/agentic pipeline are covered by the tier2 scenario
 * suite (quest-pipeline.spec.ts).
 *
 * State machine paths tested here:
 *   posted -> claimed -> posted                     (abandon)
 *   posted -> completeQuest -> 409 Conflict         (invalid transition)
 */
test.describe('Quest Lifecycle - State Transitions', () => {
	test('abandon returns quest to posted', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Create and claim a quest with a fresh agent
		const quest = await lifecycleApi.createQuest('E2E abandon lifecycle quest', 1);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('lifecycle-abandon-agent');
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		// 2. Abandon the quest
		const abandonRes = await lifecycleApi.abandonQuest(questInstance, 'E2E test abandonment');
		expect(abandonRes.ok, `abandon failed: ${abandonRes.status}`).toBeTruthy();

		// 3. Quest should revert to posted with no agent assigned
		const afterAbandon = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'posted') {
					throw new Error(`Expected posted, got ${q.status}`);
				}
				return q;
			},
			{ timeout: 8000, interval: 500, message: 'Quest did not return to posted after abandon' }
		);

		expect(afterAbandon.status).toBe('posted');
		// No agent should be holding the quest
		expect(afterAbandon.claimed_by ?? '').toBe('');
	});

	test('complete on posted quest returns conflict error', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// Create a quest but don't claim/start it — it stays posted
		const quest = await lifecycleApi.createQuest('E2E posted-complete quest', 1);
		const questInstance = extractInstance(quest.id);

		// Attempting to complete a posted quest should fail with 409 Conflict
		const completeRes = await lifecycleApi.completeQuest(questInstance);
		expect(completeRes.ok).toBe(false);
		expect(completeRes.status).toBe(409);
	});
});
