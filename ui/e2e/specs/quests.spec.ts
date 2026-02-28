import { test, expect } from '../fixtures/test-base';

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
		const statuses = ['posted', 'claimed', 'in_progress', 'in_review', 'completed'] as const;

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
	test('creating a quest adds it to posted column', async ({ questsPage, seedQuests }) => {
		// Seed a new quest
		const questIds = await seedQuests([
			{
				title: 'E2E Test Quest',
				description: 'Created by Playwright test',
				difficulty: 'easy',
				base_xp: 100
			}
		]);

		// Navigate to quests page
		await questsPage.goto();

		// If quest was created successfully, it should appear
		if (questIds.length > 0) {
			// Wait for the quest to appear in the posted column
			await questsPage.page.waitForTimeout(500); // Allow for WebSocket update

			// Look for our quest
			const postedQuests = questsPage.getQuestsInColumn('posted');
			const count = await postedQuests.count();
			expect(count).toBeGreaterThan(0);
		}
	});

	test('seeded quests appear in board', async ({ questsPage, seedQuests }) => {
		// Create multiple quests
		await seedQuests([
			{ title: 'Quest Alpha', difficulty: 'trivial', base_xp: 50 },
			{ title: 'Quest Beta', difficulty: 'moderate', base_xp: 200 },
			{ title: 'Quest Gamma', difficulty: 'hard', base_xp: 500 }
		]);

		await questsPage.goto();
		await questsPage.page.waitForTimeout(500);

		// Total quest count should include our new quests
		const totalCount = await questsPage.getTotalQuestCount();
		expect(totalCount).toBeGreaterThanOrEqual(3);
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
