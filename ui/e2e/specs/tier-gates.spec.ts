import { test, expect, hasBackend, extractInstance } from '../fixtures/test-base';

/**
 * Tier Gates
 *
 * Verifies that the backend enforces capability checks before allowing
 * agents to claim quests. Newly recruited agents start at level 1 /
 * tier 0 (Apprentice) and must be rejected from quests that require
 * higher trust tiers. Also verifies that an agent already on a quest
 * cannot claim a second one concurrently.
 *
 * Difficulty -> minimum tier mapping (from semdragons domain):
 *   0 trivial   -> Apprentice (tier 0)
 *   1 easy      -> Apprentice (tier 0)
 *   2 moderate  -> Journeyman (tier 1)
 *   3 hard      -> Expert     (tier 2)
 *   4 epic      -> Master     (tier 3)
 *   5 legendary -> Grandmaster (tier 4)
 */
test.describe('Tier Gates', () => {
	test('apprentice cannot claim hard quest', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Create a hard quest (difficulty 3 requires Expert tier)
		const quest = await lifecycleApi.createQuest('E2E tier-gate hard quest', 3);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		// 2. Recruit a brand-new agent — always starts at level 1, tier 0 (Apprentice)
		const agent = await lifecycleApi.recruitAgent('e2e-apprentice-gate-test');
		const agentInstance = extractInstance(agent.id);

		// Confirm the agent is at apprentice level
		const fetchedAgent = await lifecycleApi.getAgent(agentInstance);
		expect(fetchedAgent.tier).toBe(0); // Apprentice

		// 3. Attempt to claim — backend should reject with 403 Forbidden
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);

		expect(
			claimRes.status,
			'Expected 403 when apprentice tries to claim a hard quest'
		).toBe(403);
	});

	test('agent cannot claim while already on a quest', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Recruit a dedicated agent so we control its state entirely
		const agent = await lifecycleApi.recruitAgent('e2e-double-claim-agent');
		const agentInstance = extractInstance(agent.id);

		// 2. Create two separate easy quests
		const questA = await lifecycleApi.createQuest('E2E double-claim quest A', 1);
		const questAInstance = extractInstance(questA.id);

		const questB = await lifecycleApi.createQuest('E2E double-claim quest B', 1);
		const questBInstance = extractInstance(questB.id);

		// 3. Successfully claim the first quest
		const firstClaim = await lifecycleApi.claimQuest(questAInstance, agentInstance);
		expect(
			firstClaim.ok,
			`First claim should succeed but got ${firstClaim.status}`
		).toBeTruthy();

		// 4. Attempting to claim a second quest should be rejected.
		//    The agent is no longer idle, so the backend must return 409 Conflict.
		const secondClaim = await lifecycleApi.claimQuest(questBInstance, agentInstance);

		expect(
			secondClaim.status,
			'Expected 409 when agent already on a quest tries to claim another'
		).toBe(409);
	});
});
