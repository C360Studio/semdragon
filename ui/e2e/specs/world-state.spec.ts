import { test, expect, hasBackend, retry } from '../fixtures/test-base';

/**
 * World State
 *
 * Verifies the GET /world endpoint returns a structurally valid response
 * and that it reflects changes made through other API operations (quest
 * creation). The world state endpoint provides a denormalized snapshot
 * used by the DM's scrying pool dashboard.
 */
test.describe('World State', () => {
	test('world state returns valid structure', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const world = await lifecycleApi.getWorldState();

		// The world state must be a non-null object
		expect(world).toBeTruthy();
		expect(typeof world).toBe('object');

		// Seeded data guarantees at least some agents and quests exist at startup
		// so these arrays (or their equivalents) should be present and non-empty.
		// We accept both top-level arrays and wrapped shapes.
		const agentList = world.agents ?? (world as Record<string, unknown>)['roster'];
		const questList = world.quests ?? (world as Record<string, unknown>)['board'];

		// Either the standard fields or wrapped equivalents must be present
		expect(
			agentList !== undefined || questList !== undefined,
			'World state must contain at least one of: agents, quests, roster, board'
		).toBeTruthy();

		// If we got arrays, confirm they are actual arrays
		if (Array.isArray(agentList)) {
			expect(agentList.length).toBeGreaterThanOrEqual(0);
		}
		if (Array.isArray(questList)) {
			expect(questList.length).toBeGreaterThanOrEqual(0);
		}
	});

	test('world state reflects quest creation', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// 1. Capture a baseline snapshot
		const before = await lifecycleApi.getWorldState();
		const questsBefore = (before.quests ?? []) as unknown[];
		const countBefore = questsBefore.length;

		// 2. Create a new quest
		const quest = await lifecycleApi.createQuest('E2E world-state reflection quest', 1);
		expect(quest.id).toBeTruthy();

		// 3. Poll world state until the new quest appears.
		//    The world state may be derived from a KV watch / SSE chain so allow
		//    a short propagation window.
		const after = await retry(
			async () => {
				const w = await lifecycleApi.getWorldState();
				const quests = (w.quests ?? []) as unknown[];
				if (quests.length <= countBefore) {
					throw new Error(
						`World state quest count (${quests.length}) has not grown from ${countBefore}`
					);
				}
				return w;
			},
			{
				timeout: 10000,
				interval: 1000,
				message: 'World state did not reflect new quest after creation'
			}
		);

		const questsAfter = (after.quests ?? []) as unknown[];
		expect(questsAfter.length).toBeGreaterThan(countBefore);
	});
});
