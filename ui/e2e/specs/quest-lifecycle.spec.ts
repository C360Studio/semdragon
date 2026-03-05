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
	test('create -> claim -> start -> complete (no review)', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(120_000);

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

		// 4. Start the quest — the agentic loop (questbridge + mockLLM) processes
		//    it immediately, then bossbattle auto-passes (no review required).
		//    Calling submitQuest manually would race the pipeline, so we verify
		//    the quest reaches completed via the pipeline instead.
		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// 5. Verify the quest reaches completed
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'completed') {
					throw new Error(`Expected completed, got ${q.status}`);
				}
				return q;
			},
			{ timeout: 90000, interval: 1000, message: 'Quest did not reach completed status' }
		);

		expect(finalQuest.status).toBe('completed');
		// completed_at should be set when the quest finishes
		expect(finalQuest.completed_at).toBeTruthy();
	});

	test('quest with review required reaches in_review after processing', async ({
		lifecycleApi
	}) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(120_000);

		// Use ReviewHuman (level 3) so the quest parks at in_review.
		// The agentic loop (questbridge + mockLLM) processes started quests
		// near-instantly, so calling submitQuest manually races with the pipeline.
		// Instead, let the pipeline submit and verify the quest reaches in_review.
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E review-required lifecycle quest',
			3
		);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('lifecycle-review-agent');
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// The agentic loop processes the quest and questbridge transitions it to
		// in_review. Human review (level 3) keeps it parked there.
		const reviewQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'in_review') {
					throw new Error(`Expected in_review, got ${q.status}`);
				}
				return q;
			},
			{ timeout: 90000, interval: 1000, message: 'Quest did not reach in_review' }
		);

		expect(reviewQuest.status).toBe('in_review');
		expect(reviewQuest.constraints?.require_review).toBe(true);
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
		test.setTimeout(120_000);

		// Use a quest that requires human review (ReviewHuman = level 3).
		// This ensures the quest parks at in_review after the agentic loop completes,
		// because bossbattle returns Pending for human judges. Without this, the
		// mockLLM pipeline completes the quest in ~1ms, racing our failQuest call.
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E fail-repost lifecycle quest',
			3
		);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('lifecycle-fail-agent');
		const agentInstance = extractInstance(agent.id);

		// Claim and start — questbridge dispatches to the agentic loop
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// Wait for the quest to reach in_review (agentic loop completes, bossbattle
		// parks it because human judge is required). This is deterministic.
		await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'in_review') {
					throw new Error(`Expected in_review, got ${q.status}`);
				}
				return q;
			},
			{ timeout: 90000, interval: 1000, message: 'Quest did not reach in_review' }
		);

		// Fail from in_review — handler accepts both in_progress and in_review.
		// MaxAttempts defaults to 3, so with attempts=0 this reposts the quest.
		const failRes = await lifecycleApi.failQuest(questInstance, 'E2E intentional failure');
		expect(failRes.ok, `fail failed: ${failRes.status}`).toBeTruthy();

		// Quest should return to posted with attempt count incremented
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

	test('direct complete from in_review succeeds', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(120_000);

		// Use human review so the quest parks at in_review after the agentic loop.
		// This avoids the race where the mockLLM pipeline auto-completes the quest
		// before our completeQuest call arrives.
		const quest = await lifecycleApi.createQuestWithReview('E2E direct complete quest', 3);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('direct-complete-agent');
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// Wait for quest to reach in_review (agentic loop completes, bossbattle
		// parks it because human judge is required)
		await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status !== 'in_review') {
					throw new Error(`Expected in_review, got ${q.status}`);
				}
				return q;
			},
			{ timeout: 90000, interval: 1000, message: 'Quest did not reach in_review' }
		);

		// Direct complete — handler accepts both in_progress and in_review
		const completeRes = await lifecycleApi.completeQuest(questInstance);
		expect(completeRes.ok, `complete failed: ${completeRes.status}`).toBeTruthy();

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
		expect(finalQuest.completed_at).toBeTruthy();
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
