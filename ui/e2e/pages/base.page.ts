import { type Page, type Locator, expect } from '@playwright/test';

/**
 * Wait for SvelteKit hydration to complete.
 *
 * CRITICAL: Hydration must complete before Svelte 5 reactivity ($state, $derived) works.
 * Without this, tests may interact with DOM before reactive stores are initialized.
 */
async function waitForHydration(page: Page, timeout = 10000): Promise<void> {
	// Try to wait for hydrated class if the app uses it
	try {
		await page.locator('body.hydrated').waitFor({ state: 'attached', timeout: 2000 });
		return;
	} catch {
		// App doesn't use hydrated class, fall back to network idle
	}

	// Fallback: wait for network to be idle
	await page.waitForLoadState('networkidle', { timeout });
}

/**
 * Base page object with common layout elements.
 * All page objects extend this class.
 */
export abstract class BasePage {
	protected readonly page: Page;

	// Common layout selectors
	readonly connectionStatus: Locator;
	readonly leftPanel: Locator;
	readonly rightPanel: Locator;
	readonly centerPanel: Locator;

	// Navigation elements from the dashboard explorer
	readonly navQuests: Locator;
	readonly navAgents: Locator;
	readonly navBattles: Locator;
	readonly navGuilds: Locator;
	readonly navTrajectories: Locator;
	readonly navStore: Locator;

	constructor(page: Page) {
		this.page = page;

		// Layout
		this.connectionStatus = page.locator('.connection-status');
		this.leftPanel = page.locator('[class*="left-panel"], .explorer-panel, .filters-panel');
		this.rightPanel = page.locator('[class*="right-panel"], .details-panel');
		this.centerPanel = page.locator('[class*="center-panel"], main, .dashboard, .quest-board, .agent-roster');

		// Navigation (available from dashboard)
		this.navQuests = page.locator('.nav-item', { hasText: 'Quest Board' });
		this.navAgents = page.locator('.nav-item', { hasText: 'Agent Roster' });
		this.navBattles = page.locator('.nav-item', { hasText: 'Boss Battles' });
		this.navGuilds = page.locator('.nav-item', { hasText: 'Guilds' });
		this.navTrajectories = page.locator('.nav-item', { hasText: 'Trajectories' });
		this.navStore = page.locator('.nav-item', { hasText: 'Store' });
	}

	/**
	 * Navigate to the page (implemented by subclasses).
	 */
	abstract goto(): Promise<void>;

	/**
	 * Check if the page is loaded (implemented by subclasses).
	 */
	abstract isLoaded(): Promise<boolean>;

	/**
	 * Wait for the page to finish loading.
	 *
	 * This waits for:
	 * 1. SvelteKit hydration to complete (Svelte 5 reactivity requires this)
	 * 2. Loading states to disappear
	 */
	async waitForLoad(): Promise<void> {
		// CRITICAL: Wait for hydration before interacting with reactive state
		await waitForHydration(this.page);

		// Wait for loading states to disappear
		await this.page.waitForSelector('.loading', { state: 'hidden', timeout: 10000 }).catch(() => {
			// Loading state may not exist, that's fine
		});
		await this.page.waitForSelector('.loading-state', { state: 'hidden', timeout: 10000 }).catch(() => {
			// Loading state may not exist, that's fine
		});
	}

	/**
	 * Check WebSocket connection status.
	 */
	async isConnected(): Promise<boolean> {
		const classes = await this.connectionStatus.getAttribute('class');
		return classes?.includes('connected') ?? false;
	}

	/**
	 * Wait for WebSocket connection.
	 */
	async waitForConnection(timeout = 10000): Promise<void> {
		await this.page.waitForSelector('.connection-status.connected', { timeout });
	}

	/**
	 * Navigate to quests page.
	 */
	async goToQuests(): Promise<void> {
		await this.navQuests.click();
		await this.page.waitForURL('**/quests');
	}

	/**
	 * Navigate to agents page.
	 */
	async goToAgents(): Promise<void> {
		await this.navAgents.click();
		await this.page.waitForURL('**/agents');
	}

	/**
	 * Navigate to guilds page.
	 */
	async goToGuilds(): Promise<void> {
		await this.navGuilds.click();
		await this.page.waitForURL('**/guilds');
	}

	/**
	 * Get the page title.
	 */
	async getTitle(): Promise<string> {
		return await this.page.title();
	}
}
