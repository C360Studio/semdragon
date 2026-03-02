import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';
import type { StoreItemResponse } from '../fixtures/test-base';

/**
 * Store Lifecycle
 *
 * Exercises the agent store API: catalog listing, item lookup, purchase flow,
 * inventory inspection, and tier gating.
 *
 * Requires the backend to have the agentstore component running with the
 * default catalog seeded (10 items: 5 tools + 5 consumables).
 */
test.describe('Store Lifecycle', () => {
	test('list store returns seeded catalog', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const items = await lifecycleApi.listStore();
		expect(items.length).toBeGreaterThanOrEqual(10);

		// Verify structure of first item
		const item = items[0];
		expect(item.id).toBeTruthy();
		expect(item.name).toBeTruthy();
		expect(item.xp_cost).toBeGreaterThan(0);
		expect(item.item_type).toMatch(/^(tool|consumable)$/);
	});

	test('get individual store item', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const item = await lifecycleApi.getStoreItem('web_search');
		expect(item.id).toBe('web_search');
		expect(item.name).toBe('Web Search');
		expect(item.item_type).toBe('tool');
		expect(item.xp_cost).toBe(50);
	});

	test('purchase tool with sufficient XP', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// Use a seeded high-level agent with XP
		const world = await lifecycleApi.getWorldState();
		const agents = (world.agents ?? []) as Array<{
			id: string;
			xp?: number;
			level: number;
			status: string;
		}>;

		// Find an agent with enough XP for web_search (50 XP)
		const richAgent = agents.find((a) => (a.xp ?? 0) >= 50);
		if (!richAgent) {
			test.skip(true, 'No agent with sufficient XP found');
			return;
		}

		const agentInstance = extractInstance(richAgent.id);
		const result = await lifecycleApi.purchaseItem(richAgent.id, 'web_search');

		expect(result.success).toBe(true);
		expect(result.xp_spent).toBe(50);
		expect(result.inventory).toBeTruthy();
	});

	test('tier gate blocks apprentice from expert item', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// Recruit a fresh level-1 apprentice
		const agent = await lifecycleApi.recruitAgent('E2E Store Tier Test');
		expect(agent.tier).toBe(0); // TierApprentice = 0

		// deploy_access requires TierExpert — should be blocked
		const result = await lifecycleApi.purchaseItem(agent.id, 'deploy_access');
		expect(result.success).toBe(false);
		expect(result.error).toBeTruthy();
	});

	test('view agent inventory after purchase', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// Find or create agent with enough XP
		const world = await lifecycleApi.getWorldState();
		const agents = (world.agents ?? []) as Array<{
			id: string;
			xp?: number;
			level: number;
			status: string;
		}>;

		const richAgent = agents.find((a) => (a.xp ?? 0) >= 50);
		if (!richAgent) {
			test.skip(true, 'No agent with sufficient XP found');
			return;
		}

		const agentInstance = extractInstance(richAgent.id);

		// Purchase an item first
		const purchaseResult = await lifecycleApi.purchaseItem(richAgent.id, 'web_search');
		// May fail if already purchased — that's OK for this test
		if (!purchaseResult.success) {
			// Agent may already own it; verify via inventory
		}

		// Check inventory
		const inventory = await lifecycleApi.getInventory(agentInstance);
		expect(inventory.agent_id).toBeTruthy();
		expect(inventory.owned_tools).toBeDefined();
	});

	test('get agent effects returns array', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const agent = await lifecycleApi.recruitAgent('E2E Effects Test');
		const agentInstance = extractInstance(agent.id);

		const effects = await lifecycleApi.getEffects(agentInstance);
		expect(Array.isArray(effects)).toBe(true);
	});
});
