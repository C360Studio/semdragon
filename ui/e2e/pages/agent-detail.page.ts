import { type Page, type Locator, expect } from '@playwright/test';
import { BasePage } from './base.page';

/**
 * Page object for the Agent Detail page.
 * Located at '/agents/[id]'.
 *
 * The page shows the full agent profile with tabbed history sections:
 * quests, parties, boss battles, and collaborators.
 */
export class AgentDetailPage extends BasePage {
	// Core page elements
	readonly pageContainer: Locator;
	readonly agentName: Locator;
	readonly levelCard: Locator;
	readonly tierBadge: Locator;
	readonly agentStatus: Locator;
	readonly backLink: Locator;
	readonly notFound: Locator;

	// Level and XP
	readonly xpBar: Locator;

	// Detail cards section
	readonly detailCards: Locator;
	readonly statsGrid: Locator;

	// History section
	readonly historySection: Locator;
	readonly tabBar: Locator;
	readonly tabButtons: Locator;
	readonly activeTabPanel: Locator;

	// History sub-components
	readonly questHistoryList: Locator;
	readonly questSummaryRow: Locator;
	readonly partyHistoryList: Locator;
	readonly battleHistoryList: Locator;
	readonly battleSummaryRow: Locator;
	readonly collaboratorList: Locator;
	readonly emptyState: Locator;

	constructor(page: Page) {
		super(page);

		this.pageContainer = page.locator('[data-testid="agent-detail-page"]');
		this.agentName = page.locator('[data-testid="agent-name"]');
		this.levelCard = page.locator('[data-testid="agent-level"]');
		this.tierBadge = page.locator('.tier-badge').first();
		this.agentStatus = page.locator('.agent-status');
		this.backLink = page.locator('.back-link');
		this.notFound = page.locator('[data-testid="agent-not-found"]');

		this.xpBar = page.locator('[data-testid="agent-level"] .xp-bar');

		this.detailCards = page.locator('.detail-card');
		this.statsGrid = page.locator('.stats-grid');

		this.historySection = page.locator('.history-section');
		this.tabBar = page.locator('.tab-bar[role="tablist"]');
		this.tabButtons = page.locator('.tab-bar [role="tab"]');
		this.activeTabPanel = page.locator('[role="tabpanel"]');

		this.questHistoryList = page.locator('.quest-history .history-list');
		this.questSummaryRow = page.locator('.quest-history .summary-row');
		this.partyHistoryList = page.locator('.party-history .history-list');
		this.battleHistoryList = page.locator('.battle-history .history-list');
		this.battleSummaryRow = page.locator('.battle-history .summary-row');
		this.collaboratorList = page.locator('.collaborators .collaborator-list');
		this.emptyState = page.locator('[role="tabpanel"] .empty-state');
	}

	/**
	 * Navigate to the agents listing page (satisfies BasePage abstract).
	 */
	async goto(): Promise<void> {
		await this.page.goto('/agents');
		await this.waitForLoad();
	}

	/**
	 * Navigate to a specific agent's detail page by short instance ID.
	 */
	async gotoAgent(agentId: string): Promise<void> {
		await this.page.goto(`/agents/${agentId}`);
		await this.waitForLoad();
	}

	/**
	 * Check if the agent detail page container is visible.
	 */
	async isLoaded(): Promise<boolean> {
		return (await this.pageContainer.count()) > 0;
	}

	/**
	 * Check if the not-found state is visible (unknown agent ID).
	 */
	async isNotFound(): Promise<boolean> {
		return (await this.notFound.count()) > 0;
	}

	/**
	 * Get the agent name text from the h1.
	 */
	async getAgentName(): Promise<string> {
		return (await this.agentName.textContent()) ?? '';
	}

	/**
	 * Get the numeric level value from the level card.
	 *
	 * The level card shows a large `.level-value` span containing just the number.
	 */
	async getLevel(): Promise<number> {
		const text = await this.levelCard.locator('.level-value').textContent();
		return parseInt(text?.trim() ?? '0', 10);
	}

	/**
	 * Get the tier badge text (e.g. "Apprentice", "Journeyman").
	 */
	async getTierName(): Promise<string> {
		return (await this.tierBadge.textContent())?.trim() ?? '';
	}

	/**
	 * Get the status badge text.
	 *
	 * The backend uses snake_case (e.g. "on_quest"); the UI replaces underscores
	 * with spaces so callers should expect "on quest".
	 */
	async getStatus(): Promise<string> {
		return (await this.agentStatus.textContent())?.trim() ?? '';
	}

	/**
	 * Click a history tab by its label text (e.g. "Quests", "Boss Battles").
	 */
	async switchTab(tabLabel: string): Promise<void> {
		const tab = this.tabButtons.filter({ hasText: tabLabel });
		await tab.click();
	}

	/**
	 * Get the label text of the currently active tab.
	 */
	async getActiveTabLabel(): Promise<string> {
		const active = this.tabButtons.filter({ has: this.page.locator('[aria-selected="true"]') });
		const text = await active.textContent();
		return text?.trim() ?? '';
	}

	/**
	 * Get the numeric count badge displayed on a named tab.
	 *
	 * Returns 0 if the tab has no count badge (e.g. Collaborators tab).
	 */
	async getTabCount(tabLabel: string): Promise<number> {
		const tab = this.tabButtons.filter({ hasText: tabLabel });
		const countEl = tab.locator('.tab-count');
		if ((await countEl.count()) === 0) {
			return 0;
		}
		const text = await countEl.textContent();
		return parseInt(text?.trim() ?? '0', 10);
	}

	/**
	 * Count the history items in the currently visible tab panel.
	 *
	 * Works for quest, party, battle, and collaborator history — all use
	 * a `<ul class="history-list">` or `<ul class="collaborator-list">` pattern.
	 */
	async getHistoryItemCount(): Promise<number> {
		const panel = this.activeTabPanel;
		const historyList = panel.locator('.history-list, .collaborator-list').first();
		if ((await historyList.count()) === 0) {
			return 0;
		}
		return await historyList.locator('li').count();
	}

	/**
	 * Get all summary chip texts from the quest history summary row.
	 *
	 * Returns strings like ["3 completed", "1 failed"].
	 */
	async getQuestSummary(): Promise<string[]> {
		const chips = this.questSummaryRow.locator('.summary-chip');
		const texts: string[] = [];
		const count = await chips.count();
		for (let i = 0; i < count; i++) {
			const text = await chips.nth(i).textContent();
			if (text) texts.push(text.trim());
		}
		return texts;
	}

	/**
	 * Get all summary chip texts from the battle history summary row.
	 *
	 * Returns strings like ["2W", "1L", "67% win rate"].
	 */
	async getBattleSummary(): Promise<string[]> {
		const chips = this.battleSummaryRow.locator('.summary-chip');
		const texts: string[] = [];
		const count = await chips.count();
		for (let i = 0; i < count; i++) {
			const text = await chips.nth(i).textContent();
			if (text) texts.push(text.trim());
		}
		return texts;
	}

	/**
	 * Check if the current tab panel displays an empty state message.
	 */
	async hasEmptyState(): Promise<boolean> {
		return (await this.emptyState.count()) > 0;
	}
}
