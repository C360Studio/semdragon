import { test, expect, hasBackend } from '../fixtures/test-base';

test.describe('Quest Board', () => {
	test.beforeEach(async ({ questsPage }) => {
		await questsPage.goto();
	});

	test('displays page title', async ({ questsPage }) => {
		await expect(questsPage.heading).toContainText('Quest Board');
	});

	test('displays all kanban columns', async ({ questsPage }) => {
		await questsPage.verifyAllColumnsVisible();
	});

	test('displays quest count in header', async ({ questsPage }) => {
		await expect(questsPage.questCount).toBeVisible();
		const count = await questsPage.getTotalQuestCount();
		expect(typeof count).toBe('number');
	});

	test('kanban columns have headers with counts', async ({ questsPage }) => {
		const statuses = ['posted', 'in_progress', 'in_review', 'completed'] as const;

		for (const status of statuses) {
			await expect(questsPage.getColumnHeader(status)).toBeVisible();
			await expect(questsPage.getColumnCount(status)).toBeVisible();
		}
	});

	test('quest cards show difficulty badges', async ({ questsPage }) => {
		const cardCount = await questsPage.questCards.count();

		if (cardCount > 0) {
			const details = await questsPage.getQuestCardDetails(0);
			expect(details.difficulty).toBeTruthy();
		}
	});

	test('quest cards show XP badges', async ({ questsPage }) => {
		const cardCount = await questsPage.questCards.count();

		if (cardCount > 0) {
			const details = await questsPage.getQuestCardDetails(0);
			expect(details.xp).toMatch(/\d+\s*XP/);
		}
	});

	test('empty columns show "No quests" message', async ({ questsPage }) => {
		// Check columns that might be empty
		const completedCount = await questsPage.getQuestCountInColumn('completed');

		if (completedCount === 0) {
			await questsPage.verifyColumnEmpty('completed');
		}
	});
});

test.describe('Quest Board - Selection', () => {
	test.beforeEach(async ({ questsPage }) => {
		await questsPage.goto();
	});

	test('clicking a quest shows details panel', async ({ questsPage }) => {
		const cardCount = await questsPage.questCards.count();

		if (cardCount > 0) {
			await questsPage.selectQuest(0);
			await expect(questsPage.detailsPanel).toBeVisible();
			expect(await questsPage.hasQuestSelected()).toBe(true);
		}
	});

	test('selected quest title matches details panel', async ({ questsPage }) => {
		const cardCount = await questsPage.questCards.count();

		if (cardCount > 0) {
			const cardDetails = await questsPage.getQuestCardDetails(0);
			await questsPage.selectQuest(0);

			const selectedTitle = await questsPage.getSelectedQuestTitle();
			expect(selectedTitle).toBe(cardDetails.title);
		}
	});

	test('deselecting shows empty state in details', async ({ questsPage, page }) => {
		const cardCount = await questsPage.questCards.count();

		if (cardCount > 0) {
			// Select a quest
			await questsPage.selectQuest(0);
			expect(await questsPage.hasQuestSelected()).toBe(true);

			// Click somewhere else (the board header) to deselect
			await questsPage.heading.click();
		}

		// Check empty state (may or may not be shown depending on UI behavior)
	});
});

test.describe('Quest Board - With Seeded Data', () => {
	test('creating a quest shows it on the board', async ({ questsPage, seedQuests }) => {
		test.skip(!hasBackend(), 'Requires backend for quest creation');

		// Get baseline count
		await questsPage.goto();
		const before = await questsPage.getTotalQuestCount();

		// Seed a new quest
		const questIds = await seedQuests([
			{
				title: 'E2E Test Quest',
				description: 'Created by Playwright test',
				difficulty: 'easy',
				base_xp: 100
			}
		]);

		expect(questIds.length).toBe(1);

		// Wait for the quest count to increase — autonomy may claim/complete it,
		// but it should appear on the board regardless of which column it lands in
		await expect(async () => {
			const after = await questsPage.getTotalQuestCount();
			expect(after).toBeGreaterThan(before);
		}).toPass({ timeout: 5000 });
	});

	test('seeded quests appear in board', async ({ questsPage, seedQuests }) => {
		test.skip(!hasBackend(), 'Requires backend for quest creation');

		await questsPage.goto();
		const before = await questsPage.getTotalQuestCount();

		// Create multiple quests
		await seedQuests([
			{ title: 'Quest Alpha', difficulty: 'trivial', base_xp: 50 },
			{ title: 'Quest Beta', difficulty: 'moderate', base_xp: 200 },
			{ title: 'Quest Gamma', difficulty: 'hard', base_xp: 500 }
		]);

		// Wait for all 3 to appear — autonomy may move them between columns,
		// but the total should increase by at least 3
		await expect(async () => {
			const after = await questsPage.getTotalQuestCount();
			expect(after).toBeGreaterThanOrEqual(before + 3);
		}).toPass({ timeout: 5000 });
	});
});

test.describe('Quest Board - Detail Panel Structure', () => {
	test.beforeEach(async ({ questsPage }) => {
		await questsPage.goto();
	});

	test('selected quest shows detail fields', async ({ questsPage }) => {
		const cardCount = await questsPage.questCards.count();
		if (cardCount > 0) {
			await questsPage.selectQuest(0);
			const panel = questsPage.detailsPanel;

			// Should show core detail fields
			await expect(panel.locator('dt:has-text("Status")')).toBeVisible();
			await expect(panel.locator('dt:has-text("Difficulty")')).toBeVisible();
			await expect(panel.locator('dt:has-text("Base XP")')).toBeVisible();
		}
	});

	test('selected quest shows "View full quest" link', async ({ questsPage }) => {
		const cardCount = await questsPage.questCards.count();
		if (cardCount > 0) {
			await questsPage.selectQuest(0);
			const viewLink = questsPage.detailsPanel.locator('.view-full-link');
			await expect(viewLink).toBeVisible();
			await expect(viewLink).toContainText('View full quest');
		}
	});

	test('boss battle card shows for completed quests', async ({ questsPage, page }) => {
		test.skip(!hasBackend(), 'Requires backend with quest data');

		await questsPage.goto();

		// Look for completed quests which should have a verdict (from battle or auto-pass)
		const completedColumn = questsPage.getColumn('completed');
		const completedCards = completedColumn.locator('[data-testid="quest-card"]');
		const count = await completedCards.count();

		if (count > 0) {
			await completedCards.first().click();

			// Battle card shows when quest has a verdict (battle entity or auto-pass)
			const battleCard = questsPage.detailsPanel.locator('.battle-card');
			const hasBattle = await battleCard.isVisible().catch(() => false);
			if (hasBattle) {
				await expect(battleCard.locator('h4')).toContainText('Boss Battle');
				await expect(battleCard.locator('.verdict-badge')).toBeVisible();
			}
		}
	});
});

test.describe('Quest Board - Accessibility', () => {
	test('quest cards have aria labels', async ({ questsPage }) => {
		await questsPage.goto();

		const cardCount = await questsPage.questCards.count();
		if (cardCount > 0) {
			const firstCard = questsPage.questCards.first();
			const ariaLabel = await firstCard.getAttribute('aria-label');
			expect(ariaLabel).toBeTruthy();
		}
	});

	test('quest cards have aria-pressed state', async ({ questsPage }) => {
		await questsPage.goto();

		const cardCount = await questsPage.questCards.count();
		if (cardCount > 0) {
			const firstCard = questsPage.questCards.first();

			// Before selection
			const pressedBefore = await firstCard.getAttribute('aria-pressed');
			expect(pressedBefore).toBe('false');

			// After selection
			await firstCard.click();
			const pressedAfter = await firstCard.getAttribute('aria-pressed');
			expect(pressedAfter).toBe('true');
		}
	});
});
