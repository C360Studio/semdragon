import { test, expect, hasBackend, hasLLM, isMockLLM, extractInstance, retry } from '../../fixtures/test-base';

/**
 * Dependency Chain — Tier 2 Scenario
 *
 * Verifies that when a quest with dependencies completes, autonomy automatically
 * claims and completes the dependent quest without manual intervention.
 *
 * This tests the full pipeline: post chain → autonomy claims A → agentic loop
 * completes A → notifyDependents fires → boid engine publishes suggestions →
 * autonomy immediately evaluates and claims B → agentic loop completes B.
 *
 * Bug context: autonomy backed off its heartbeat timer (up to 60s) and never
 * re-evaluated when dependencies were satisfied. Fix: handleBoidSuggestion
 * triggers evaluateAutonomy immediately via goroutine.
 *
 * @scenario @tier2
 */
test.describe.serial('Dependency Chain', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend with LLM (E2E_LLM_MODE=mock|gemini|...)'
		);
	});

	test('quest chain: A completes, B is autonomously claimed and completed', async ({
		lifecycleApi,
		request
	}) => {
		// Mock LLM completes quests in ~1-2s; real LLMs take longer.
		// The key assertion: B must be claimed AFTER A completes, not manually.
		test.setTimeout(isMockLLM() ? 60_000 : 300_000);

		const baseURL = `http://localhost:${process.env.BACKEND_PORT || '8081'}`;

		// 1. Post a chain: Quest A (no deps) → Quest B (depends on A)
		const chainResp = await request.post(`${baseURL}/game/quests/chain`, {
			data: {
				quests: [
					{
						title: 'Chain Quest A - Independent Research',
						goal: 'Research and summarize a topic. Write findings to a file.',
						difficulty: 1,
						skills: ['research']
					},
					{
						title: 'Chain Quest B - Depends on A',
						goal: 'Build on the research from Quest A. Produce a synthesis document.',
						difficulty: 1,
						skills: ['research'],
						depends_on: [0]
					}
				]
			}
		});
		expect(chainResp.ok(), `chain post failed: ${chainResp.status()}`).toBeTruthy();
		const quests = (await chainResp.json()) as Array<{
			id: string;
			title: string;
			depends_on?: string[];
		}>;
		expect(quests.length).toBe(2);

		const questA = quests.find((q) => q.title.includes('Quest A'))!;
		const questB = quests.find((q) => q.title.includes('Quest B'))!;
		expect(questA, 'Quest A not found in chain response').toBeTruthy();
		expect(questB, 'Quest B not found in chain response').toBeTruthy();

		const instanceA = extractInstance(questA.id);
		const instanceB = extractInstance(questB.id);

		console.log(`[Chain] Quest A: ${questA.id} (${instanceA})`);
		console.log(`[Chain] Quest B: ${questB.id} (${instanceB}), depends_on: ${questB.depends_on}`);

		// 2. Wait for Quest A to be autonomously claimed and completed.
		// Autonomy + boid engine should pick it up without intervention.
		const questACompleted = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(instanceA);
				if (q.status === 'completed') return q;
				throw new Error(`Quest A status: ${q.status}`);
			},
			{
				retries: isMockLLM() ? 30 : 120,
				delay: 1000
			}
		);
		expect(questACompleted.status).toBe('completed');
		console.log(`[Chain] Quest A completed (claimed_by: ${questACompleted.claimed_by})`);

		// 3. Quest B should now be eligible (dependency satisfied).
		// The critical assertion: B must be autonomously claimed and completed
		// WITHOUT manual intervention. This is the code path that was broken.
		//
		// Flow: A completes → notifyDependents → boid engine recomputes →
		// publishes suggestion → autonomy evaluateAutonomy (immediate) → claims B
		const questBCompleted = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(instanceB);
				if (q.status === 'completed') return q;
				// Log intermediate state for debugging
				if (q.status !== 'posted') {
					console.log(`[Chain] Quest B status: ${q.status}, claimed_by: ${q.claimed_by}`);
				}
				throw new Error(`Quest B status: ${q.status}`);
			},
			{
				retries: isMockLLM() ? 30 : 120,
				delay: 1000
			}
		);
		expect(questBCompleted.status).toBe('completed');
		expect(questBCompleted.claimed_by, 'Quest B should have been claimed by an agent').toBeTruthy();
		console.log(
			`[Chain] Quest B completed (claimed_by: ${questBCompleted.claimed_by}) — ` +
				`dependency chain pipeline PASSED`
		);
	});
});
