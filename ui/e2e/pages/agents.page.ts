import { type Page, type Locator, expect } from '@playwright/test';
import { BasePage } from './base.page';

/**
 * Agent status types matching the backend.
 */
type AgentStatus = 'idle' | 'on_quest' | 'in_battle' | 'cooldown' | 'retired';

/**
 * Page object for the Agent Roster page.
 * Located at '/agents'.
 */
export class AgentsPage extends BasePage {
	// Header
	readonly heading: Locator;
	readonly agentCount: Locator;

	// Filters panel
	readonly filtersPanel: Locator;
	readonly statusFilter: Locator;

	// Agent grid
	readonly agentGrid: Locator;
	readonly agentCards: Locator;

	// Details panel
	readonly detailsPanel: Locator;
	readonly detailsName: Locator;
	readonly detailsTierBadge: Locator;
	readonly detailsLevel: Locator;
	readonly detailsXPBar: Locator;
	readonly viewProfileLink: Locator;

	// Loading state
	readonly loadingState: Locator;

	constructor(page: Page) {
		super(page);

		// Header
		this.heading = page.locator('.roster-header h1');
		this.agentCount = page.locator('.agent-count');

		// Filters
		this.filtersPanel = page.locator('.filters-panel');
		this.statusFilter = page.locator('#agent-status-filter');

		// Agent grid
		this.agentGrid = page.locator('.agent-grid');
		this.agentCards = page.locator('.agent-card');

		// Details panel
		this.detailsPanel = page.locator('.details-panel');
		this.detailsName = page.locator('.agent-profile h3');
		this.detailsTierBadge = page.locator('.agent-profile .tier-badge');
		this.detailsLevel = page.locator('.level-number');
		this.detailsXPBar = page.locator('.level-display .xp-bar');
		this.viewProfileLink = page.locator('.view-full-link');

		// Loading state
		this.loadingState = page.locator('.loading-state');
	}

	/**
	 * Navigate to the agents page.
	 */
	async goto(): Promise<void> {
		await this.page.goto('/agents');
		await this.waitForLoad();
	}

	/**
	 * Check if the agents page is loaded.
	 */
	async isLoaded(): Promise<boolean> {
		const heading = await this.heading.textContent();
		return heading?.includes('Agent Roster') ?? false;
	}

	/**
	 * Get total agent count from the header.
	 */
	async getTotalAgentCount(): Promise<number> {
		const text = await this.agentCount.textContent();
		const match = text?.match(/(\d+)/);
		return match ? parseInt(match[1], 10) : 0;
	}

	/**
	 * Get the number of visible agent cards.
	 */
	async getVisibleAgentCount(): Promise<number> {
		return await this.agentCards.count();
	}

	/**
	 * Filter agents by status.
	 */
	async filterByStatus(status: AgentStatus | 'all'): Promise<void> {
		await this.statusFilter.selectOption(status);
		// Wait for the UI to update
		await this.page.waitForTimeout(100);
	}

	/**
	 * Get an agent card by index.
	 */
	getAgentCard(index: number): Locator {
		return this.agentCards.nth(index);
	}

	/**
	 * Get an agent card by name.
	 */
	getAgentCardByName(name: string): Locator {
		return this.page.locator('.agent-card').filter({
			has: this.page.locator('.agent-name', { hasText: name })
		});
	}

	/**
	 * Select an agent card by clicking on it.
	 */
	async selectAgent(index: number): Promise<void> {
		await this.agentCards.nth(index).click();
	}

	/**
	 * Select an agent by name.
	 */
	async selectAgentByName(name: string): Promise<void> {
		const card = this.getAgentCardByName(name);
		await card.click();
	}

	/**
	 * Get the name of the selected agent from the details panel.
	 */
	async getSelectedAgentName(): Promise<string> {
		const name = await this.detailsName.textContent();
		return name ?? '';
	}

	/**
	 * Check if an agent is selected (has details visible).
	 */
	async hasAgentSelected(): Promise<boolean> {
		const text = await this.detailsPanel.textContent();
		return !text?.includes('Select an agent to view details');
	}

	/**
	 * Get details of an agent card.
	 */
	async getAgentCardDetails(index: number): Promise<{
		name: string;
		level: string;
		tier: string;
		status: string;
	}> {
		const card = this.agentCards.nth(index);
		const name = (await card.locator('.agent-name').textContent()) ?? '';
		const level = (await card.locator('.agent-level').textContent()) ?? '';
		const tier = (await card.locator('.tier-badge').textContent()) ?? '';
		const status = (await card.locator('.agent-status').textContent()) ?? '';

		return {
			name: name.trim(),
			level: level.trim(),
			tier: tier.trim(),
			status: status.trim()
		};
	}

	/**
	 * Get all agent names from the visible cards.
	 */
	async getAllAgentNames(): Promise<string[]> {
		const names: string[] = [];
		const count = await this.agentCards.count();

		for (let i = 0; i < count; i++) {
			const name = await this.agentCards.nth(i).locator('.agent-name').textContent();
			if (name) {
				names.push(name.trim());
			}
		}

		return names;
	}

	/**
	 * Verify that agent cards display XP bars.
	 */
	async verifyXPBarsVisible(): Promise<void> {
		const cards = await this.agentCards.all();
		for (const card of cards.slice(0, 5)) {
			// Check first 5 for efficiency
			await expect(card.locator('.xp-bar')).toBeVisible();
		}
	}

	/**
	 * Verify the agent grid is visible.
	 */
	async verifyGridVisible(): Promise<void> {
		await expect(this.agentGrid).toBeVisible();
	}

	/**
	 * Check if the roster is in loading state.
	 */
	async isLoading(): Promise<boolean> {
		return await this.loadingState.isVisible();
	}

	/**
	 * Navigate to an agent's full profile page.
	 */
	async goToAgentProfile(): Promise<void> {
		await this.viewProfileLink.click();
		await this.page.waitForURL('**/agents/**');
	}

	/**
	 * Get agents filtered by tier.
	 * Note: UI currently only has status filter, this gets tier from card data.
	 */
	async getAgentsByTier(tierName: string): Promise<Locator[]> {
		const cards = await this.agentCards.all();
		const result: Locator[] = [];

		for (const card of cards) {
			const tier = await card.locator('.tier-badge').textContent();
			if (tier?.toLowerCase().includes(tierName.toLowerCase())) {
				result.push(card);
			}
		}

		return result;
	}

	/**
	 * Verify the empty state message.
	 */
	async verifyEmptyState(): Promise<void> {
		const emptyMessage = this.page.locator('.empty-state');
		await expect(emptyMessage).toContainText('No agents found');
	}
}
