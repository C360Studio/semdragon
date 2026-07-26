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
test.describe('Quest Lifecycle - Metrics Fields', () => {
	test('quest response includes metrics fields', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const quest = await lifecycleApi.createQuest('E2E metrics schema quest', 1);
		const questInstance = extractInstance(quest.id);
		const q = await lifecycleApi.getQuest(questInstance);

		// Metrics fields exist on the response (may be zero/absent for unexecuted quests)
		expect(q).toHaveProperty('status');
		expect(typeof (q.turns_used ?? 0)).toBe('number');
		expect(typeof (q.tokens_prompt ?? 0)).toBe('number');
		expect(typeof (q.tokens_completion ?? 0)).toBe('number');
	});
});

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

test.describe('Quest Lifecycle - Framework Operator Gateway', () => {
	test('operator transition is completed by the mechanics-stub rule', async ({ request }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const authHeaders: Record<string, string> = {};
		if (process.env.SEMDRAGONS_API_KEY) {
			authHeaders['X-API-Key'] = process.env.SEMDRAGONS_API_KEY;
		}

		// Use the UI-origin request context for both product commands and the
		// operator gateway. This exercises the Vite same-origin proxy and keeps
		// the setup runnable when SEMDRAGONS_API_KEY is configured.
		const createQuest = await request.post('/game/quests', {
			headers: authHeaders,
			data: {
				objective: 'E2E lifecycle gateway quest',
				hints: {
					suggested_difficulty: 1,
					suggested_skills: [],
					require_human_review: false,
					budget: 100
				}
			}
		});
		expect(createQuest.ok(), `create quest failed: ${await createQuest.text()}`).toBeTruthy();
		const quest = (await createQuest.json()) as { id: string };
		const questInstance = extractInstance(quest.id);

		const recruitAgent = await request.post('/game/agents', {
			headers: authHeaders,
			data: { name: 'lifecycle-gateway-agent', skills: [], is_npc: false }
		});
		expect(recruitAgent.ok(), `recruit agent failed: ${await recruitAgent.text()}`).toBeTruthy();
		const agent = (await recruitAgent.json()) as { id: string };
		const agentInstance = extractInstance(agent.id);

		const claimRes = await request.post(`/game/quests/${questInstance}/claim`, {
			headers: authHeaders,
			data: { agent_id: agentInstance }
		});
		expect(claimRes.ok(), `claim failed: ${claimRes.status()}`).toBeTruthy();
		const startRes = await request.post(`/game/quests/${questInstance}/start`, {
			headers: authHeaders,
			data: {}
		});
		expect(startRes.ok(), `start failed: ${startRes.status()}`).toBeTruthy();

		await retry(
			async () => {
				const response = await request.get(`/game/quests/${questInstance}`);
				expect(response.ok()).toBeTruthy();
				const current = (await response.json()) as { status?: string };
				if (current.status !== 'in_progress') {
					throw new Error(`Expected in_progress, got ${current.status}`);
				}
			},
			{ timeout: 8000, interval: 250, message: 'Quest did not enter in_progress' }
		);

		const gatewayRoot = '/lifecycle-gateway/workflows';
		const operatorNote = 'tier-1 lifecycle gateway verification';
		const transition = await request.post(`${gatewayRoot}/quest/${quest.id}/transition`, {
			headers: authHeaders,
			data: { phase: 'in_review', note: operatorNote }
		});
		expect(
			transition.ok(),
			`gateway transition failed: ${transition.status()} ${await transition.text()}`
		).toBeTruthy();

		const terminal = await retry(
			async () => {
				const response = await request.get(`${gatewayRoot}/quest/${quest.id}`, {
					headers: authHeaders
				});
				expect(response.ok()).toBeTruthy();
				const state = (await response.json()) as { phase?: string; verdict?: string };
				if (state.phase !== 'completed' || state.verdict !== 'review-skipped-mechanics-disabled') {
					throw new Error(`Expected completed review stub, got ${state.phase}/${state.verdict}`);
				}
				return state;
			},
			{ timeout: 10000, interval: 250, message: 'Mechanics-stub rule did not complete quest' }
		);
		expect(terminal.verdict).toBe('review-skipped-mechanics-disabled');

		const historyResponse = await request.get(`${gatewayRoot}/quest/${quest.id}/history`, {
			headers: authHeaders
		});
		expect(historyResponse.ok()).toBeTruthy();
		const history = (await historyResponse.json()) as Array<{
			from: string;
			to: string;
			triggered: string;
			note?: string;
		}>;
		expect(history).toEqual(
			expect.arrayContaining([
				expect.objectContaining({
					from: 'in_progress',
					to: 'in_review',
					triggered: 'operator',
					note: operatorNote
				}),
				expect.objectContaining({
					from: 'in_review',
					to: 'completed',
					triggered: 'rule',
					note: 'review skipped (mechanics disabled)'
				})
			])
		);
	});
});
