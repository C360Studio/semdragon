import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/**
 * Dependency Chain — Tier 1 Tests
 *
 * Verifies that completing a quest notifies dependent quests, making them
 * visible to the boid engine and autonomy for claiming. Does NOT require
 * an LLM — uses direct API state transitions.
 *
 * Bug: quests with depends_on were stuck in "posted" forever because
 * completing a dependency didn't emit a KV event on the dependent quest.
 */
test.describe('Dependency Chain', () => {
	test.beforeEach(() => {
		test.skip(!hasBackend(), 'Requires running backend');
	});

	test('completing a dependency makes dependent quest available', async ({ lifecycleApi, request }) => {
		test.setTimeout(30_000);

		const baseURL = `http://localhost:${process.env.BACKEND_PORT || '8081'}`;

		// 1. Post a quest chain: A (index 0, no deps) → B (index 1, depends on index 0)
		const chainResp = await request.post(`${baseURL}/game/quests/chain`, {
			data: {
				quests: [
					{
						title: 'Quest A - Independent',
						goal: 'First quest with no dependencies',
						difficulty: 1,
						skills: ['research'],
					},
					{
						title: 'Quest B - Depends on A',
						goal: 'Second quest that depends on Quest A',
						difficulty: 1,
						skills: ['research'],
						depends_on: [0],
					},
				],
			},
		});
		expect(chainResp.ok(), `chain post failed: ${chainResp.status()}`).toBeTruthy();
		const quests = (await chainResp.json()) as Array<{ id: string; title: string; depends_on?: string[] }>;
		expect(quests.length).toBe(2);

		const questA = quests.find((q) => q.title.includes('Quest A'))!;
		const questB = quests.find((q) => q.title.includes('Quest B'))!;
		expect(questA).toBeTruthy();
		expect(questB).toBeTruthy();

		const instanceA = extractInstance(questA.id);
		const instanceB = extractInstance(questB.id);

		// 2. Quest B should have depends_on referencing Quest A
		const bDetail = await lifecycleApi.getQuest(instanceB);
		expect(bDetail.depends_on).toBeTruthy();
		expect(bDetail.depends_on!.length).toBeGreaterThan(0);

		// 3. Recruit an agent and complete Quest A through the full lifecycle
		const agent = await lifecycleApi.recruitAgent('dep-chain-agent', ['research']);
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(instanceA, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(instanceA);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		const submitRes = await lifecycleApi.submitQuest(instanceA, 'E2E test output for quest A');
		expect(submitRes.ok, `submit failed: ${submitRes.status}`).toBeTruthy();

		// Submit triggers boss battle which auto-completes. If the quest isn't
		// yet completed, try the explicit complete endpoint (DM/admin path).
		const aAfterSubmit = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(instanceA);
				if (q.status === 'completed') return q;
				// Try explicit complete if boss battle hasn't auto-completed
				await lifecycleApi.completeQuest(instanceA);
				return await lifecycleApi.getQuest(instanceA);
			},
			{ retries: 15, delay: 500 }
		);
		expect(aAfterSubmit.status).toBe('completed');

		// 4. Wait for Quest B to receive the dependency notification.
		// The notification is async (goroutine in CompleteQuest), so poll briefly.
		const bAfter = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(instanceB);
				return q;
			},
			{ retries: 10, delay: 500 }
		);
		expect(bAfter.status).toBe('posted');

		// 5. Verify Quest B is now claimable (dependency met)
		const claimB = await lifecycleApi.claimQuest(instanceB, agentInstance);
		expect(claimB.ok, `Quest B claim should succeed after A completed: ${claimB.status}`).toBeTruthy();

		// 6. Verify Quest A is completed
		const aFinal = await lifecycleApi.getQuest(instanceA);
		expect(aFinal.status).toBe('completed');
	});

	test('quest with unmet dependency cannot be claimed', async ({ lifecycleApi, request }) => {
		test.setTimeout(15_000);

		const baseURL = `http://localhost:${process.env.BACKEND_PORT || '8081'}`;

		// Post chain: C (index 0) → D (index 1, depends on index 0)
		const chainResp = await request.post(`${baseURL}/game/quests/chain`, {
			data: {
				quests: [
					{
						title: 'Quest C - Blocker',
						goal: 'Blocking quest',
						difficulty: 1,
						skills: ['research'],
					},
					{
						title: 'Quest D - Blocked',
						goal: 'Blocked by Quest C',
						difficulty: 1,
						skills: ['research'],
						depends_on: [0],
					},
				],
			},
		});
		expect(chainResp.ok()).toBeTruthy();
		const quests = (await chainResp.json()) as Array<{ id: string; title: string }>;

		const questD = quests.find((q) => q.title.includes('Quest D'))!;
		const instanceD = extractInstance(questD.id);

		// Recruit agent and try to claim D (should fail — C not completed)
		const agent = await lifecycleApi.recruitAgent('dep-blocked-agent', ['research']);
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(instanceD, agentInstance);
		expect(claimRes.ok).toBeFalsy();
	});
});
