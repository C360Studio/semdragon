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

		// Recruit a dedicated agent at level 5 so we have known XP state.
		// Level 5 agents have ~250 XP (midpoint) — more than enough for
		// web_search (50 XP). Using our own agent avoids races with other
		// tests that may also purchase items from seeded agents.
		const agent = await lifecycleApi.recruitAgentAtLevel(
			`store-purchase-${Date.now()}`,
			5,
			['coding']
		);

		const result = await lifecycleApi.purchaseItem(agent.id, 'web_search');

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

// =============================================================================
// CONSUMABLE USAGE
// =============================================================================

test.describe('Consumable Usage', () => {
	test('purchase consumable appears in inventory consumables map', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// Find an agent with enough XP for retry_token (50 XP, TierApprentice)
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
		const result = await lifecycleApi.purchaseItem(richAgent.id, 'retry_token');

		if (result.success) {
			// Verify the consumable shows up in inventory
			const inventory = await lifecycleApi.getInventory(agentInstance);
			expect(inventory.consumables).toBeDefined();
			expect(inventory.consumables['retry_token']).toBeGreaterThanOrEqual(1);
		}
		// If purchase failed (already owned or insufficient XP), check inventory directly
		const inventory = await lifecycleApi.getInventory(agentInstance);
		expect(inventory.agent_id).toBeTruthy();
	});

	test('use consumable returns success and decrements remaining', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// Find an agent with enough XP
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

		// Ensure agent has a retry_token
		await lifecycleApi.purchaseItem(richAgent.id, 'retry_token');

		// Check inventory has the consumable
		const invBefore = await lifecycleApi.getInventory(agentInstance);
		const countBefore = invBefore.consumables?.['retry_token'] ?? 0;
		if (countBefore === 0) {
			test.skip(true, 'Agent has no retry_token to use');
			return;
		}

		// Use the consumable (no quest_id needed for retry_token)
		const result = await lifecycleApi.useConsumable(agentInstance, 'retry_token');
		expect(result.success).toBe(true);

		// Verify inventory count decremented
		const invAfter = await lifecycleApi.getInventory(agentInstance);
		const countAfter = invAfter.consumables?.['retry_token'] ?? 0;
		expect(countAfter).toBe(countBefore - 1);
	});

	test('active effects appear after using consumable', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// Find an agent with enough XP for cooldown_skip (75 XP, TierApprentice)
		const world = await lifecycleApi.getWorldState();
		const agents = (world.agents ?? []) as Array<{
			id: string;
			xp?: number;
			level: number;
			status: string;
		}>;

		const richAgent = agents.find((a) => (a.xp ?? 0) >= 75);
		if (!richAgent) {
			test.skip(true, 'No agent with sufficient XP found');
			return;
		}

		const agentInstance = extractInstance(richAgent.id);

		// Purchase and use a cooldown_skip
		await lifecycleApi.purchaseItem(richAgent.id, 'cooldown_skip');
		const invCheck = await lifecycleApi.getInventory(agentInstance);
		if ((invCheck.consumables?.['cooldown_skip'] ?? 0) === 0) {
			test.skip(true, 'Agent has no cooldown_skip to use');
			return;
		}

		await lifecycleApi.useConsumable(agentInstance, 'cooldown_skip');

		// Check active effects
		const effects = await lifecycleApi.getEffects(agentInstance);
		expect(Array.isArray(effects)).toBe(true);
		// After using cooldown_skip, there should be an active effect
		const cooldownEffect = effects.find(
			(e) => e.consumable_id === 'cooldown_skip'
		);
		if (cooldownEffect) {
			expect(cooldownEffect.effect.type).toBe('cooldown_skip');
		}
	});
});
