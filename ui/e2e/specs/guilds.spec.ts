import { test, expect } from '../fixtures/test-base';

test.describe('Guild Registry', () => {
	test.beforeEach(async ({ guildsPage }) => {
		await guildsPage.goto();
	});

	test('displays page title', async ({ guildsPage }) => {
		const loaded = await guildsPage.isLoaded();
		expect(loaded).toBe(true);
	});

	test('displays guild count in header', async ({ guildsPage }) => {
		await expect(guildsPage.guildCount).toBeVisible();
		const count = await guildsPage.getGuildCount();
		expect(typeof count).toBe('number');
	});

	test('displays guild grid', async ({ guildsPage }) => {
		await expect(guildsPage.guildsGrid).toBeVisible();
	});

	test('guild cards show name and status', async ({ guildsPage }) => {
		const cardCount = await guildsPage.getVisibleGuildCount();
		if (cardCount > 0) {
			const details = await guildsPage.getGuildCardDetails(0);
			expect(details.name).toBeTruthy();
			expect(details.status).toMatch(/active|inactive/);
		}
	});

	test('guild cards have description element', async ({ guildsPage }) => {
		const cardCount = await guildsPage.getVisibleGuildCount();
		if (cardCount > 0) {
			const descEl = guildsPage.guildCards.first().locator('.guild-description');
			await expect(descEl).toBeAttached();
		}
	});

	test('guild cards are keyboard focusable', async ({ guildsPage }) => {
		const cardCount = await guildsPage.getVisibleGuildCount();
		if (cardCount > 0) {
			const firstCard = guildsPage.guildCards.first();
			await firstCard.focus();
			await expect(firstCard).toBeFocused();
		}
	});

	test('guild card links to detail page', async ({ guildsPage, page }) => {
		const cardCount = await guildsPage.getVisibleGuildCount();
		if (cardCount > 0) {
			await guildsPage.clickGuild(0);
			await expect(page).toHaveURL(/.*\/guilds\/.+/);
		}
	});

	test('has correct page title', async ({ guildsPage }) => {
		const title = await guildsPage.getTitle();
		expect(title).toContain('Guilds');
	});
});

test.describe('Guild Detail Page', () => {
	test('shows not-found for invalid guild id', async ({ guildDetailPage }) => {
		await guildDetailPage.gotoGuild('nonexistent-guild-id');
		const notFound = await guildDetailPage.isNotFound();
		expect(notFound).toBe(true);
	});

	test('back link navigates to guild registry', async ({ guildDetailPage, page }) => {
		await guildDetailPage.gotoGuild('nonexistent-guild-id');
		await guildDetailPage.backLink.click();
		await expect(page).toHaveURL(/.*\/guilds$/);
	});

	test('displays guild detail when navigated from list', async ({ guildsPage, page }) => {
		await guildsPage.goto();
		const cardCount = await guildsPage.getVisibleGuildCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		const details = await guildsPage.getGuildCardDetails(0);
		await guildsPage.clickGuild(0);

		// Guild name should match card name
		const nameEl = page.locator('[data-testid="guild-name"]');
		await expect(nameEl).toContainText(details.name);
	});

	test('displays stats bar', async ({ guildsPage, page }) => {
		await guildsPage.goto();
		const cardCount = await guildsPage.getVisibleGuildCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		await guildsPage.clickGuild(0);
		const statsBar = page.locator('.stats-bar');
		await expect(statsBar).toBeVisible();
	});

	test('member links are keyboard focusable', async ({ guildsPage, page }) => {
		await guildsPage.goto();
		const cardCount = await guildsPage.getVisibleGuildCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		await guildsPage.clickGuild(0);

		// Check guildmaster highlight or member rows
		const memberLinks = page.locator('.member-highlight, .member-row');
		const linkCount = await memberLinks.count();
		if (linkCount > 0) {
			const firstLink = memberLinks.first();
			await firstLink.focus();
			await expect(firstLink).toBeFocused();

			// Should have aria-label for accessibility
			const ariaLabel = await firstLink.getAttribute('aria-label');
			expect(ariaLabel).toBeTruthy();
		}
	});

	test('has correct page title', async ({ guildsPage, page }) => {
		await guildsPage.goto();
		const cardCount = await guildsPage.getVisibleGuildCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		await guildsPage.clickGuild(0);
		const title = await page.title();
		expect(title).toContain('Semdragons');
	});
});
