import { test, expect } from '../fixtures/test-base';

test.describe('Dashboard', () => {
	test.beforeEach(async ({ dashboardPage }) => {
		await dashboardPage.goto();
	});

	test('displays page title', async ({ dashboardPage }) => {
		await expect(dashboardPage.heading).toContainText("The DM's Scrying Pool");
	});

	test('displays connection status indicator', async ({ dashboardPage }) => {
		// Should show either connected or disconnected
		await expect(dashboardPage.connectionStatusBadge).toBeVisible();
	});

	test('displays stats grid with all metrics', async ({ dashboardPage }) => {
		await expect(dashboardPage.statsGrid).toBeVisible();

		// Check that stat cards exist
		await expect(dashboardPage.statActiveAgents).toBeVisible();
		await expect(dashboardPage.statIdleAgents).toBeVisible();
		await expect(dashboardPage.statOpenQuests).toBeVisible();
		await expect(dashboardPage.statActiveQuests).toBeVisible();
		await expect(dashboardPage.statCompletionRate).toBeVisible();
	});

	test('displays tier distribution section', async ({ dashboardPage }) => {
		await expect(dashboardPage.tierDistribution).toBeVisible();
		await expect(dashboardPage.tierBars.first()).toBeVisible();
	});

	test('displays all major sections', async ({ dashboardPage }) => {
		await dashboardPage.verifyAllSectionsVisible();
	});

	test('event feed shows recent events', async ({ dashboardPage }) => {
		await expect(dashboardPage.eventFeed).toBeVisible();
		await expect(dashboardPage.eventList).toBeVisible();
	});

	test('event filter changes displayed events', async ({ dashboardPage }) => {
		// Filter to quest events
		await dashboardPage.filterEvents('quest');

		// Verify filter is set
		await expect(dashboardPage.eventFilter).toHaveValue('quest');
	});

	test('tier distribution shows five tiers', async ({ dashboardPage }) => {
		const tiers = await dashboardPage.getTierDistribution();

		// Should have exactly 5 tiers
		expect(tiers.length).toBe(5);

		// Verify tier names
		const tierNames = tiers.map((t) => t.name.toLowerCase());
		expect(tierNames).toContain('apprentice');
		expect(tierNames).toContain('journeyman');
		expect(tierNames).toContain('expert');
		expect(tierNames).toContain('master');
		expect(tierNames).toContain('grandmaster');
	});
});

test.describe('Dashboard - Navigation', () => {
	test('navigates to quest board from explorer', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();
		await dashboardPage.goToQuests();

		await expect(page).toHaveURL(/.*\/quests/);
	});

	test('navigates to agent roster from explorer', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();
		await dashboardPage.goToAgents();

		await expect(page).toHaveURL(/.*\/agents/);
	});

	test('navigates to guilds from explorer', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();
		await dashboardPage.goToGuilds();

		await expect(page).toHaveURL(/.*\/guilds/);
	});
});

test.describe('Dashboard - Stats Values', () => {
	test('stat values are numeric or percentage', async ({ dashboardPage }) => {
		await dashboardPage.goto();

		const activeAgents = await dashboardPage.getStatValue('Active Agents');
		expect(activeAgents).toMatch(/^\d+$/);

		const completionRate = await dashboardPage.getStatValue('Completion Rate');
		expect(completionRate).toMatch(/^\d+\.?\d*%$/);

		const avgQuality = await dashboardPage.getStatValue('Avg Quality');
		expect(avgQuality).toMatch(/^\d+\.?\d*%$/);
	});
});
