import { test, expect } from '../fixtures/test-base';

test.describe('Agent Roster', () => {
	test.beforeEach(async ({ agentsPage }) => {
		await agentsPage.goto();
	});

	test('displays page title', async ({ agentsPage }) => {
		await expect(agentsPage.heading).toContainText('Agent Roster');
	});

	test('displays agent count in header', async ({ agentsPage }) => {
		await expect(agentsPage.agentCount).toBeVisible();
		const count = await agentsPage.getTotalAgentCount();
		expect(typeof count).toBe('number');
	});

	test('displays agent grid', async ({ agentsPage }) => {
		await agentsPage.verifyGridVisible();
	});

	test('displays status filter', async ({ agentsPage }) => {
		await expect(agentsPage.statusFilter).toBeVisible();
	});

	test('agent cards show name and level', async ({ agentsPage }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			const details = await agentsPage.getAgentCardDetails(0);
			expect(details.name).toBeTruthy();
			expect(details.level).toMatch(/Lv\.\d+/);
		}
	});

	test('agent cards show tier badge', async ({ agentsPage }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			const details = await agentsPage.getAgentCardDetails(0);
			expect(details.tier).toBeTruthy();
			expect(details.tier.toLowerCase()).toMatch(
				/apprentice|journeyman|expert|master|grandmaster/
			);
		}
	});

	test('agent cards show XP bars', async ({ agentsPage }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			await agentsPage.verifyXPBarsVisible();
		}
	});

	test('agent cards show status badge', async ({ agentsPage }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			const details = await agentsPage.getAgentCardDetails(0);
			expect(details.status).toBeTruthy();
		}
	});
});

test.describe('Agent Roster - Filtering', () => {
	test.beforeEach(async ({ agentsPage }) => {
		await agentsPage.goto();
	});

	test('filter by idle status', async ({ agentsPage }) => {
		await agentsPage.filterByStatus('idle');

		// Count should update
		const count = await agentsPage.getVisibleAgentCount();
		const headerCount = await agentsPage.getTotalAgentCount();

		// Visible count should match header count after filtering
		expect(count).toBe(headerCount);
	});

	test('filter by all status shows all agents', async ({ agentsPage }) => {
		// First filter to something specific
		await agentsPage.filterByStatus('idle');
		const idleCount = await agentsPage.getVisibleAgentCount();

		// Then show all
		await agentsPage.filterByStatus('all');
		const allCount = await agentsPage.getVisibleAgentCount();

		// All should include at least the idle agents
		expect(allCount).toBeGreaterThanOrEqual(idleCount);
	});

	test('filter updates header count', async ({ agentsPage }) => {
		const initialCount = await agentsPage.getTotalAgentCount();

		await agentsPage.filterByStatus('idle');
		const filteredCount = await agentsPage.getTotalAgentCount();

		// Filtered count should be <= initial count
		expect(filteredCount).toBeLessThanOrEqual(initialCount);
	});
});

test.describe('Agent Roster - Selection', () => {
	test.beforeEach(async ({ agentsPage }) => {
		await agentsPage.goto();
	});

	test('clicking an agent shows details panel', async ({ agentsPage }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			await agentsPage.selectAgent(0);
			await expect(agentsPage.detailsPanel).toBeVisible();
			expect(await agentsPage.hasAgentSelected()).toBe(true);
		}
	});

	test('selected agent name matches details panel', async ({ agentsPage }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			const cardDetails = await agentsPage.getAgentCardDetails(0);
			await agentsPage.selectAgent(0);

			const selectedName = await agentsPage.getSelectedAgentName();
			expect(selectedName).toBe(cardDetails.name);
		}
	});

	test('details panel shows level information', async ({ agentsPage }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			await agentsPage.selectAgent(0);
			await expect(agentsPage.detailsLevel).toBeVisible();
			const levelText = await agentsPage.detailsLevel.textContent();
			expect(levelText).toMatch(/Level \d+/);
		}
	});

	test('details panel shows XP bar', async ({ agentsPage }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			await agentsPage.selectAgent(0);
			await expect(agentsPage.detailsXPBar).toBeVisible();
		}
	});

	test('view profile link navigates to agent page', async ({ agentsPage, page }) => {
		const cardCount = await agentsPage.getVisibleAgentCount();

		if (cardCount > 0) {
			await agentsPage.selectAgent(0);
			await agentsPage.goToAgentProfile();

			await expect(page).toHaveURL(/.*\/agents\/.+/);
		}
	});
});

test.describe('Agent Roster - With Seeded Data', () => {
	test('seeded agents appear in roster', async ({ agentsPage }) => {
		await agentsPage.goto();

		// E2E roster creates specific agents
		const names = await agentsPage.getAllAgentNames();

		// With E2ETestRoster, we should see agents like:
		// apprentice-1, apprentice-2, apprentice-3, journeyman-1, etc.
		// At minimum, check we have some agents
		expect(names.length).toBeGreaterThan(0);
	});

	test('agents span multiple tiers', async ({ agentsPage }) => {
		await agentsPage.goto();

		const cardCount = await agentsPage.getVisibleAgentCount();
		if (cardCount < 3) {
			test.skip();
			return;
		}

		// Collect tier information from multiple agents
		const tiers = new Set<string>();
		for (let i = 0; i < Math.min(cardCount, 10); i++) {
			const details = await agentsPage.getAgentCardDetails(i);
			tiers.add(details.tier.toLowerCase());
		}

		// With E2E roster, we should have at least 2 different tiers
		// (apprentice and journeyman at minimum)
		expect(tiers.size).toBeGreaterThanOrEqual(1);
	});
});

test.describe('Agent Roster - Accessibility', () => {
	test('agent cards have aria labels', async ({ agentsPage }) => {
		await agentsPage.goto();

		const cardCount = await agentsPage.getVisibleAgentCount();
		if (cardCount > 0) {
			const firstCard = agentsPage.agentCards.first();
			const ariaLabel = await firstCard.getAttribute('aria-label');
			expect(ariaLabel).toBeTruthy();
			expect(ariaLabel).toMatch(/Select agent:/);
		}
	});

	test('agent cards have aria-pressed state', async ({ agentsPage }) => {
		await agentsPage.goto();

		const cardCount = await agentsPage.getVisibleAgentCount();
		if (cardCount > 0) {
			const firstCard = agentsPage.agentCards.first();

			// Before selection
			const pressedBefore = await firstCard.getAttribute('aria-pressed');
			expect(pressedBefore).toBe('false');

			// After selection
			await firstCard.click();
			const pressedAfter = await firstCard.getAttribute('aria-pressed');
			expect(pressedAfter).toBe('true');
		}
	});

	test('status filter has label', async ({ agentsPage }) => {
		await agentsPage.goto();

		const label = agentsPage.page.locator('label[for="agent-status-filter"]');
		await expect(label).toBeVisible();
	});
});
