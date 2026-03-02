import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/**
 * Quest Lifecycle - Happy Path
 *
 * Exercises the full quest state machine via the backend API and verifies
 * that each transition produces the correct status in GET /quests/{id}.
 *
 * Each test recruits its own fresh agent so parallel runs never race
 * over the same idle agent.
 *
 * State machine:
 *   posted -> claimed -> in_progress -> completed   (no review)
 *   posted -> claimed -> in_progress -> in_review   (review required)
 *   posted -> claimed -> posted                     (abandon)
 *   posted -> claimed -> in_progress -> posted      (fail, retries remain)
 */
test.describe('Quest Lifecycle - Happy Path', () => {
	test('create -> claim -> start -> submit -> complete (no review)', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Create an easy quest with no review requirement
		const quest = await lifecycleApi.createQuest('E2E no-review lifecycle quest', 1);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		// 2. Recruit a fresh agent (starts idle, apprentice tier — fine for easy quests)
		const agent = await lifecycleApi.recruitAgent('lifecycle-norev-agent');
		const agentInstance = extractInstance(agent.id);

		// 3. Claim the quest
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		// 4. Start the quest
		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// 5. Submit the quest (no review -> should transition directly to completed)
		const submitRes = await lifecycleApi.submitQuest(questInstance, 'E2E output: all good');
		expect(submitRes.ok, `submit failed: ${submitRes.status}`).toBeTruthy();

		// 6. Verify the final state
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'completed') {
					throw new Error(`Expected completed, got ${q.status}`);
				}
				return q;
			},
			{ timeout: 8000, interval: 500, message: 'Quest did not reach completed status' }
		);

		expect(finalQuest.status).toBe('completed');
		// completed_at should be set when the quest finishes
		expect(finalQuest.completed_at).toBeTruthy();
	});

	test('create -> claim -> start -> submit -> in_review (with review)', async ({
		lifecycleApi
	}) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Create a quest that requires human review (difficulty=easy so any agent qualifies)
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E review-required lifecycle quest',
			1
		);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		// 2. Recruit a fresh agent
		const agent = await lifecycleApi.recruitAgent('lifecycle-review-agent');
		const agentInstance = extractInstance(agent.id);

		// 3. Claim -> start -> submit
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		const submitRes = await lifecycleApi.submitQuest(questInstance, 'E2E review output');
		expect(submitRes.ok, `submit failed: ${submitRes.status}`).toBeTruthy();

		// 4. With review required, the submit response should show in_review.
		//    The boss battle processor auto-evaluates quickly, so the status
		//    is transient — by the time we poll it may already be failed/completed.
		//    Check the submit response directly for the review path.
		const submitData = await submitRes.json();
		expect(submitData.status).toBe('in_review');
		expect(submitData.constraints?.require_review).toBe(true);
	});

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
		expect(afterAbandon.agent_id ?? '').toBe('');
	});

	test('fail with retries remaining reposts quest', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Create a quest and recruit a fresh agent
		const quest = await lifecycleApi.createQuest('E2E fail-repost lifecycle quest', 1);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('lifecycle-fail-agent');
		const agentInstance = extractInstance(agent.id);

		// 2. Claim -> start -> fail
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		const failRes = await lifecycleApi.failQuest(questInstance, 'E2E intentional failure');
		expect(failRes.ok, `fail failed: ${failRes.status}`).toBeTruthy();

		// 3. Quest should return to posted with attempt count incremented
		const afterFail = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'posted') {
					throw new Error(`Expected posted, got ${q.status}`);
				}
				return q;
			},
			{ timeout: 8000, interval: 500, message: 'Quest did not return to posted after fail' }
		);

		expect(afterFail.status).toBe('posted');
		// The backend increments attempts on each failure
		const attempts = (afterFail.attempts as number) ?? 0;
		expect(attempts).toBeGreaterThanOrEqual(1);
	});
});
