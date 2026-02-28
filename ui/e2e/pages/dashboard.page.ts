import { type Page, type Locator, expect } from '@playwright/test';
import { BasePage } from './base.page';

/**
 * Page object for the Dashboard (DM Command Center).
 * Located at the root path '/'.
 */
export class DashboardPage extends BasePage {
	// Header
	readonly heading: Locator;
	readonly connectionStatusBadge: Locator;

	// Stats grid
	readonly statsGrid: Locator;
	readonly statActiveAgents: Locator;
	readonly statIdleAgents: Locator;
	readonly statOpenQuests: Locator;
	readonly statActiveQuests: Locator;
	readonly statCompletionRate: Locator;
	readonly statAvgQuality: Locator;
	readonly statTotalXP: Locator;
	readonly statActiveParties: Locator;
	readonly statGuilds: Locator;
	readonly statBattleWinRate: Locator;

	// Sections
	readonly tierDistribution: Locator;
	readonly tierBars: Locator;
	readonly activeQuestsSection: Locator;
	readonly activeBattlesSection: Locator;
	readonly guildActivitySection: Locator;
	readonly activePartiesSection: Locator;

	// Event feed (in left panel)
	readonly eventFeed: Locator;
	readonly eventFilter: Locator;
	readonly eventList: Locator;
	readonly eventItems: Locator;

	// Details panel (right)
	readonly detailsPanel: Locator;

	constructor(page: Page) {
		super(page);

		// Header
		this.heading = page.locator('.dashboard-header h1');
		this.connectionStatusBadge = page.locator('.connection-status');

		// Stats grid - locate by label text
		this.statsGrid = page.locator('.stats-grid');
		this.statActiveAgents = this.getStatCard('Active Agents');
		this.statIdleAgents = this.getStatCard('Idle Agents');
		this.statOpenQuests = this.getStatCard('Open Quests');
		this.statActiveQuests = this.getStatCard('Active Quests');
		this.statCompletionRate = this.getStatCard('Completion Rate');
		this.statAvgQuality = this.getStatCard('Avg Quality');
		this.statTotalXP = this.getStatCard('Total XP Earned');
		this.statActiveParties = this.getStatCard('Active Parties');
		this.statGuilds = this.getStatCard('Guilds');
		this.statBattleWinRate = this.getStatCard('Battle Win Rate');

		// Sections
		this.tierDistribution = page.locator('.dashboard-section').filter({
			has: page.locator('#tier-heading')
		});
		this.tierBars = page.locator('.tier-bars .tier-row');
		this.activeQuestsSection = page.locator('.dashboard-section').filter({
			has: page.locator('#quests-heading')
		});
		this.activeBattlesSection = page.locator('.dashboard-section').filter({
			has: page.locator('#battles-heading')
		});
		this.guildActivitySection = page.locator('.dashboard-section').filter({
			has: page.locator('#guilds-heading')
		});
		this.activePartiesSection = page.locator('.dashboard-section').filter({
			has: page.locator('#parties-heading')
		});

		// Event feed
		this.eventFeed = page.locator('.event-feed');
		this.eventFilter = page.locator('.event-filter');
		this.eventList = page.locator('.event-list');
		this.eventItems = page.locator('.event-item');

		// Details panel
		this.detailsPanel = page.locator('.details-panel');
	}

	/**
	 * Get a stat card by its label.
	 */
	private getStatCard(label: string): Locator {
		return this.page.locator('.stat-card').filter({
			has: this.page.locator('.stat-label', { hasText: label })
		});
	}

	/**
	 * Navigate to the dashboard.
	 */
	async goto(): Promise<void> {
		await this.page.goto('/');
		await this.waitForLoad();
	}

	/**
	 * Check if the dashboard is loaded.
	 */
	async isLoaded(): Promise<boolean> {
		const heading = await this.heading.textContent();
		return heading?.includes("The DM's Scrying Pool") ?? false;
	}

	/**
	 * Get the value of a stat card.
	 */
	async getStatValue(label: string): Promise<string> {
		const card = this.getStatCard(label);
		const value = await card.locator('.stat-value').textContent();
		return value ?? '';
	}

	/**
	 * Filter events by category.
	 */
	async filterEvents(category: 'all' | 'quest' | 'agent' | 'battle' | 'guild'): Promise<void> {
		await this.eventFilter.selectOption(category);
	}

	/**
	 * Get the number of events in the feed.
	 */
	async getEventCount(): Promise<number> {
		return await this.eventItems.count();
	}

	/**
	 * Get tier distribution data.
	 */
	async getTierDistribution(): Promise<Array<{ name: string; count: string }>> {
		const rows = await this.tierBars.all();
		const data: Array<{ name: string; count: string }> = [];

		for (const row of rows) {
			const name = await row.locator('.tier-name').textContent();
			const count = await row.locator('.tier-count').textContent();
			if (name && count) {
				data.push({ name: name.trim(), count: count.trim() });
			}
		}

		return data;
	}

	/**
	 * Click on an active quest to select it.
	 */
	async selectQuest(index: number): Promise<void> {
		const questRows = this.activeQuestsSection.locator('.quest-row');
		await questRows.nth(index).click();
	}

	/**
	 * Check if the details panel shows quest information.
	 */
	async hasQuestDetails(): Promise<boolean> {
		const text = await this.detailsPanel.textContent();
		return text?.includes('Selected Quest') ?? false;
	}

	/**
	 * Verify the dashboard displays expected sections.
	 */
	async verifyAllSectionsVisible(): Promise<void> {
		await expect(this.heading).toBeVisible();
		await expect(this.statsGrid).toBeVisible();
		await expect(this.tierDistribution).toBeVisible();
		await expect(this.activeQuestsSection).toBeVisible();
		await expect(this.activeBattlesSection).toBeVisible();
		await expect(this.guildActivitySection).toBeVisible();
		await expect(this.activePartiesSection).toBeVisible();
	}

	/**
	 * Verify connection status is displayed.
	 */
	async verifyConnectionStatus(connected: boolean): Promise<void> {
		if (connected) {
			await expect(this.connectionStatusBadge).toHaveClass(/connected/);
			await expect(this.connectionStatusBadge).toContainText('Connected');
		} else {
			await expect(this.connectionStatusBadge).not.toHaveClass(/connected/);
			await expect(this.connectionStatusBadge).toContainText('Disconnected');
		}
	}
}
