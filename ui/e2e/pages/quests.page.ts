import { type Page, type Locator, expect } from '@playwright/test';
import { BasePage } from './base.page';

/**
 * Quest status types matching the backend.
 */
type QuestStatus = 'posted' | 'claimed' | 'in_progress' | 'in_review' | 'completed';

/**
 * Page object for the Quest Board page.
 * Located at '/quests'.
 */
export class QuestsPage extends BasePage {
	// Header
	readonly heading: Locator;
	readonly questCount: Locator;

	// Kanban board
	readonly kanbanBoard: Locator;
	readonly kanbanColumns: Locator;

	// Quest cards
	readonly questCards: Locator;

	// Details panel
	readonly detailsPanel: Locator;
	readonly detailsTitle: Locator;
	readonly detailsDescription: Locator;

	// Loading state
	readonly loadingState: Locator;

	constructor(page: Page) {
		super(page);

		// Header
		this.heading = page.locator('.board-header h1');
		this.questCount = page.locator('.quest-count');

		// Kanban board
		this.kanbanBoard = page.locator('.kanban-board');
		this.kanbanColumns = page.locator('.kanban-column');

		// Quest cards
		this.questCards = page.locator('.quest-card');

		// Details panel
		this.detailsPanel = page.locator('.details-panel');
		this.detailsTitle = page.locator('.detail-section h3');
		this.detailsDescription = page.locator('.quest-description');

		// Loading state
		this.loadingState = page.locator('.loading-state');
	}

	/**
	 * Navigate to the quests page.
	 */
	async goto(): Promise<void> {
		await this.page.goto('/quests');
		await this.waitForLoad();
	}

	/**
	 * Check if the quests page is loaded.
	 */
	async isLoaded(): Promise<boolean> {
		const heading = await this.heading.textContent();
		return heading?.includes('Quest Board') ?? false;
	}

	/**
	 * Get a kanban column by status.
	 */
	getColumn(status: QuestStatus): Locator {
		return this.page.locator(`.kanban-column[data-status="${status}"]`);
	}

	/**
	 * Get the column header for a status.
	 */
	getColumnHeader(status: QuestStatus): Locator {
		return this.getColumn(status).locator('.column-header h3');
	}

	/**
	 * Get the count badge for a column.
	 */
	getColumnCount(status: QuestStatus): Locator {
		return this.getColumn(status).locator('.column-count');
	}

	/**
	 * Get all quest cards in a column.
	 */
	getQuestsInColumn(status: QuestStatus): Locator {
		return this.getColumn(status).locator('.quest-card');
	}

	/**
	 * Get the number of quests in a column.
	 */
	async getQuestCountInColumn(status: QuestStatus): Promise<number> {
		return await this.getQuestsInColumn(status).count();
	}

	/**
	 * Get total quest count from the header.
	 */
	async getTotalQuestCount(): Promise<number> {
		const text = await this.questCount.textContent();
		const match = text?.match(/(\d+)/);
		return match ? parseInt(match[1], 10) : 0;
	}

	/**
	 * Select a quest card by clicking on it.
	 */
	async selectQuest(index: number): Promise<void> {
		await this.questCards.nth(index).click();
	}

	/**
	 * Select a quest by its title.
	 */
	async selectQuestByTitle(title: string): Promise<void> {
		const questCard = this.page.locator('.quest-card').filter({
			has: this.page.locator('.quest-title', { hasText: title })
		});
		await questCard.click();
	}

	/**
	 * Get the title of the selected quest from the details panel.
	 */
	async getSelectedQuestTitle(): Promise<string> {
		const title = await this.detailsTitle.textContent();
		return title ?? '';
	}

	/**
	 * Check if a quest is selected (has details visible).
	 */
	async hasQuestSelected(): Promise<boolean> {
		const text = await this.detailsPanel.textContent();
		return !text?.includes('Select a quest to view details');
	}

	/**
	 * Get details of a quest card.
	 */
	async getQuestCardDetails(index: number): Promise<{
		title: string;
		difficulty: string;
		xp: string;
	}> {
		const card = this.questCards.nth(index);
		const title = (await card.locator('.quest-title').textContent()) ?? '';
		const difficulty = (await card.locator('.difficulty-badge').textContent()) ?? '';
		const xp = (await card.locator('.xp-badge').textContent()) ?? '';

		return { title: title.trim(), difficulty: difficulty.trim(), xp: xp.trim() };
	}

	/**
	 * Verify all kanban columns are visible.
	 */
	async verifyAllColumnsVisible(): Promise<void> {
		const columns: QuestStatus[] = ['posted', 'claimed', 'in_progress', 'in_review', 'completed'];

		for (const status of columns) {
			await expect(this.getColumn(status)).toBeVisible();
		}
	}

	/**
	 * Wait for a specific number of quests in a column.
	 */
	async waitForQuestsInColumn(status: QuestStatus, count: number, timeout = 5000): Promise<void> {
		await expect(this.getQuestsInColumn(status)).toHaveCount(count, { timeout });
	}

	/**
	 * Check if the board is in loading state.
	 */
	async isLoading(): Promise<boolean> {
		return await this.loadingState.isVisible();
	}

	/**
	 * Verify the empty state message in a column.
	 */
	async verifyColumnEmpty(status: QuestStatus): Promise<void> {
		const emptyMessage = this.getColumn(status).locator('.empty-column');
		await expect(emptyMessage).toContainText('No quests');
	}
}
