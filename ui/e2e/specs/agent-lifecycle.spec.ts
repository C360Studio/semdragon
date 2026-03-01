import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/**
 * Agent Lifecycle
 *
 * Tests agent creation, retirement, and status transitions that occur
 * as a result of quest lifecycle events. These tests use the API directly
 * and do not depend on any specific seeded agent — they create their own.
 */
test.describe('Agent Lifecycle', () => {
	test('recruit new agent via API', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Recruit a new agent
		const agent = await lifecycleApi.recruitAgent('e2e-recruit-test', ['code_review']);
		expect(agent.id).toBeTruthy();

		const agentInstance = extractInstance(agent.id);

		// 2. Verify properties on the created agent
		const fetched = await lifecycleApi.getAgent(agentInstance);

		expect(fetched.id).toBeTruthy();
		expect(fetched.name).toBe('e2e-recruit-test');
		// Newly recruited agents start at level 1 and tier 0 (Apprentice)
		expect(fetched.level).toBe(1);
		expect(fetched.tier).toBe(0);
	});

	test('retire agent via API', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Recruit a new agent so we have a known entity to retire
		const agent = await lifecycleApi.recruitAgent('e2e-retire-test');
		const agentInstance = extractInstance(agent.id);

		// 2. Retire the agent
		// The retire endpoint is POST /game/agents/{id}/retire
		const retireRes = await fetch(
			`${process.env.API_URL ?? 'http://localhost:8080'}/game/agents/${agentInstance}/retire`,
			{ method: 'POST', headers: { 'Content-Type': 'application/json' } }
		);
		expect(retireRes.ok, `retire failed: ${retireRes.status}`).toBeTruthy();

		// 3. Verify the agent is now retired
		const retired = await retry(
			async () => {
				const a = await lifecycleApi.getAgent(agentInstance);
				if (a.status !== 'retired') {
					throw new Error(`Expected retired, got ${a.status}`);
				}
				return a;
			},
			{ timeout: 8000, interval: 500, message: 'Agent did not reach retired status' }
		);

		expect(retired.status).toBe('retired');
	});

	test('agent status updates during quest lifecycle', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Recruit a fresh agent so we have full control over its state
		const agent = await lifecycleApi.recruitAgent('e2e-status-tracking-agent');
		const agentInstance = extractInstance(agent.id);

		// Confirm initial idle status
		const initial = await lifecycleApi.getAgent(agentInstance);
		expect(initial.status).toBe('idle');

		// 2. Create an easy quest and claim it with our agent
		const quest = await lifecycleApi.createQuest('E2E agent status quest', 1);
		const questInstance = extractInstance(quest.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		// After claiming, the agent should transition to on_quest
		const onQuest = await retry(
			async () => {
				const a = await lifecycleApi.getAgent(agentInstance);
				if (a.status !== 'on_quest') {
					throw new Error(`Expected on_quest, got ${a.status}`);
				}
				return a;
			},
			{ timeout: 8000, interval: 500, message: 'Agent did not reach on_quest status after claim' }
		);
		expect(onQuest.status).toBe('on_quest');

		// 3. Start and submit the quest (no review -> completes immediately)
		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		const submitRes = await lifecycleApi.submitQuest(questInstance, 'E2E status tracking output');
		expect(submitRes.ok, `submit failed: ${submitRes.status}`).toBeTruthy();

		// After completion the agent should return to idle
		const backToIdle = await retry(
			async () => {
				const a = await lifecycleApi.getAgent(agentInstance);
				if (a.status !== 'idle') {
					throw new Error(`Expected idle, got ${a.status}`);
				}
				return a;
			},
			{
				timeout: 8000,
				interval: 500,
				message: 'Agent did not return to idle after quest completion'
			}
		);
		expect(backToIdle.status).toBe('idle');
	});
});
