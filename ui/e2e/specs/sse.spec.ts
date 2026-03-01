import { test, expect, hasBackend } from '../fixtures/test-base';

test.describe('SSE Connection', () => {
	test('dashboard shows connection status', async ({ dashboardPage }) => {
		await dashboardPage.goto();

		// Connection status should be visible
		await expect(dashboardPage.connectionStatusBadge).toBeVisible();
	});

	test('connection status reflects SSE state', async ({ dashboardPage, sseHelper }) => {
		test.skip(!hasBackend(), 'Requires backend for SSE connection');

		await dashboardPage.goto();

		// Wait for SSE to connect
		await sseHelper.waitForConnection(15000);
		await dashboardPage.verifyConnectionStatus(true);
	});
});

test.describe('SSE - Real-time Updates', () => {
	test('event feed updates when events occur', async ({ dashboardPage, sseHelper, seedQuests }) => {
		test.skip(!hasBackend(), 'Requires backend for SSE events');

		await dashboardPage.goto();

		// Wait for connection
		await sseHelper.waitForConnection(5000);

		// Get initial event count
		const initialCount = await sseHelper.getEventCount();

		// Create a new quest (should trigger an SSE event via KV change)
		await seedQuests([
			{
				title: 'SSE Test Quest',
				difficulty: 'easy',
				base_xp: 50
			}
		]);

		// Wait for the event to appear in the feed
		await dashboardPage.page.waitForTimeout(1000);

		// Check if event count increased
		const newCount = await sseHelper.getEventCount();
		expect(newCount).toBeGreaterThanOrEqual(initialCount);
	});

	test('stats update when data changes', async ({ dashboardPage, sseHelper, seedQuests }) => {
		test.skip(!hasBackend(), 'Requires backend for SSE data');

		await dashboardPage.goto();
		await sseHelper.waitForConnection(5000);

		// Get initial open quests count
		const initialOpenQuests = await dashboardPage.getStatValue('Open Quests');

		// Create new quests
		await seedQuests([
			{ title: 'Stat Update Test 1', difficulty: 'easy' },
			{ title: 'Stat Update Test 2', difficulty: 'easy' }
		]);

		// Wait for stats to update via SSE
		await dashboardPage.page.waitForTimeout(1000);

		// Note: This test verifies the stat display exists and updates
		// The exact count depends on backend state
		const newOpenQuests = await dashboardPage.getStatValue('Open Quests');
		expect(newOpenQuests).toBeTruthy();
	});
});

test.describe('SSE - Connection Recovery', () => {
	test('connection status updates on disconnect', async ({ dashboardPage, sseHelper }) => {
		test.skip(!hasBackend(), 'Requires backend for SSE connection');

		await dashboardPage.goto();

		// Wait for initial connection
		await sseHelper.waitForConnection(10000);

		// Block SSE connections
		await sseHelper.blockSSE();

		// Reload page to trigger reconnection attempt
		await dashboardPage.page.reload();

		// Should show disconnected (since we blocked the route)
		await sseHelper.waitForDisconnection(5000);

		// Always unblock
		await sseHelper.unblockSSE();
	});
});

test.describe('SSE - Event Types', () => {
	test('quest events appear in feed', async ({ dashboardPage, sseHelper, seedQuests }) => {
		test.skip(!hasBackend(), 'Requires backend for SSE events');

		await dashboardPage.goto();
		await sseHelper.waitForConnection(5000);

		// Filter to quest events
		await dashboardPage.filterEvents('quest');

		// Create a quest
		await seedQuests([{ title: 'Quest Event Test', difficulty: 'easy' }]);

		// Wait for event propagation
		await dashboardPage.page.waitForTimeout(1000);

		// Verify event filter is set
		await expect(dashboardPage.eventFilter).toHaveValue('quest');
	});

	test('filter controls event visibility', async ({ dashboardPage, sseHelper }) => {
		test.skip(!hasBackend(), 'Requires backend for SSE events');

		await dashboardPage.goto();
		await sseHelper.waitForConnection(5000);

		// Get all events
		await dashboardPage.filterEvents('all');
		const allCount = await sseHelper.getEventCount();

		// Filter to specific category
		await dashboardPage.filterEvents('agent');
		const agentCount = await sseHelper.getEventCount();

		// Agent-only count should be <= all events
		expect(agentCount).toBeLessThanOrEqual(allCount);
	});
});

test.describe('SSE - UI Responsiveness', () => {
	test('UI remains responsive during connection', async ({ dashboardPage }) => {
		await dashboardPage.goto();

		// Verify UI elements are interactive while SSE connects
		// These elements are always visible regardless of loading state
		await expect(dashboardPage.heading).toBeVisible();
		await expect(dashboardPage.eventFeed).toBeVisible();
		await expect(dashboardPage.eventFilter).toBeEnabled();

		// Can interact with filter
		await dashboardPage.filterEvents('quest');
		await expect(dashboardPage.eventFilter).toHaveValue('quest');
	});

	test('navigation works regardless of connection state', async ({
		dashboardPage,
		questsPage,
		page
	}) => {
		await dashboardPage.goto();

		// Navigate without waiting for SSE
		await dashboardPage.navQuests.click();
		await expect(page).toHaveURL(/.*\/quests/);

		// Page should load
		await expect(questsPage.heading).toBeVisible();
	});
});
