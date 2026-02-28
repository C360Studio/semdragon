import { test, expect } from '../fixtures/test-base';

test.describe('Navigation', () => {
	test('can navigate from dashboard to quests', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();

		await dashboardPage.navQuests.click();
		await expect(page).toHaveURL(/.*\/quests/);
		await expect(page.locator('h1')).toContainText('Quest Board');
	});

	test('can navigate from dashboard to agents', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();

		await dashboardPage.navAgents.click();
		await expect(page).toHaveURL(/.*\/agents/);
		await expect(page.locator('h1')).toContainText('Agent Roster');
	});

	test('can navigate from dashboard to battles', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();

		await dashboardPage.navBattles.click();
		await expect(page).toHaveURL(/.*\/battles/);
	});

	test('can navigate from dashboard to guilds', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();

		await dashboardPage.navGuilds.click();
		await expect(page).toHaveURL(/.*\/guilds/);
	});

	test('can navigate from dashboard to trajectories', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();

		await dashboardPage.navTrajectories.click();
		await expect(page).toHaveURL(/.*\/trajectories/);
	});

	test('can navigate from dashboard to store', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();

		await dashboardPage.navStore.click();
		await expect(page).toHaveURL(/.*\/store/);
	});
});

test.describe('Navigation - Cross-Page', () => {
	test('can navigate from quests to agents', async ({ questsPage, page }) => {
		await questsPage.goto();

		// Use a direct link or navigate via URL
		await page.goto('/agents');
		await expect(page).toHaveURL(/.*\/agents/);
	});

	test('can navigate from agents to quests', async ({ agentsPage, page }) => {
		await agentsPage.goto();

		await page.goto('/quests');
		await expect(page).toHaveURL(/.*\/quests/);
	});

	test('can navigate back to dashboard', async ({ questsPage, page }) => {
		await questsPage.goto();

		await page.goto('/');
		await expect(page.locator('.dashboard-header h1')).toContainText("The DM's Scrying Pool");
	});
});

test.describe('Navigation - Direct URLs', () => {
	test('direct navigation to /quests works', async ({ page }) => {
		await page.goto('/quests');
		await expect(page.locator('h1')).toContainText('Quest Board');
	});

	test('direct navigation to /agents works', async ({ page }) => {
		await page.goto('/agents');
		await expect(page.locator('h1')).toContainText('Agent Roster');
	});

	test('direct navigation to /guilds works', async ({ page }) => {
		await page.goto('/guilds');
		await expect(page).toHaveURL(/.*\/guilds/);
	});

	test('direct navigation to /battles works', async ({ page }) => {
		await page.goto('/battles');
		await expect(page).toHaveURL(/.*\/battles/);
	});

	test('direct navigation to /store works', async ({ page }) => {
		await page.goto('/store');
		await expect(page).toHaveURL(/.*\/store/);
	});

	test('direct navigation to /trajectories works', async ({ page }) => {
		await page.goto('/trajectories');
		await expect(page).toHaveURL(/.*\/trajectories/);
	});
});

test.describe('Navigation - Browser History', () => {
	test('back button works after navigation', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();
		await dashboardPage.navQuests.click();
		await expect(page).toHaveURL(/.*\/quests/);

		await page.goBack();
		await expect(page).toHaveURL('/');
	});

	test('forward button works after going back', async ({ dashboardPage, page }) => {
		await dashboardPage.goto();
		await dashboardPage.navQuests.click();
		await page.goBack();

		await page.goForward();
		await expect(page).toHaveURL(/.*\/quests/);
	});
});

test.describe('Navigation - Page Titles', () => {
	test('dashboard has correct title', async ({ dashboardPage }) => {
		await dashboardPage.goto();
		const title = await dashboardPage.getTitle();
		expect(title).toContain('Semdragons');
	});

	test('quests page has correct title', async ({ questsPage }) => {
		await questsPage.goto();
		const title = await questsPage.getTitle();
		expect(title).toContain('Quest Board');
	});

	test('agents page has correct title', async ({ agentsPage }) => {
		await agentsPage.goto();
		const title = await agentsPage.getTitle();
		expect(title).toContain('Agent Roster');
	});
});

test.describe('Navigation - Link Appearance', () => {
	test('nav items have icons', async ({ dashboardPage }) => {
		await dashboardPage.goto();

		const questNavItem = dashboardPage.navQuests;
		const icon = questNavItem.locator('.nav-icon');
		await expect(icon).toBeVisible();
	});

	test('nav items have counts', async ({ dashboardPage }) => {
		await dashboardPage.goto();

		// Quest nav should show count
		const questNavItem = dashboardPage.navQuests;
		const count = questNavItem.locator('.nav-count');
		await expect(count).toBeVisible();
	});
});
