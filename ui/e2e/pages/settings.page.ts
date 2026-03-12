import { type Page, type Locator, expect } from '@playwright/test';
import { BasePage } from './base.page';

/**
 * Page object for the Settings page.
 *
 * Settings sections auto-generate data-testid via SettingsSection component:
 * data-testid="settings-section-{title.toLowerCase().replace(/\s+/g, '-')}"
 */
export class SettingsPage extends BasePage {
	// Page-level locators
	readonly heading: Locator;
	readonly refreshBtn: Locator;
	readonly errorBanner: Locator;
	readonly loadingState: Locator;
	readonly unavailableState: Locator;
	readonly sectionsContainer: Locator;

	// Section locators (auto-generated testids from SettingsSection)
	readonly sectionGettingStarted: Locator;
	readonly sectionConnection: Locator;
	readonly sectionPlatformIdentity: Locator;
	readonly sectionLLMProviders: Locator;
	readonly sectionCapabilityRouting: Locator;
	readonly sectionComponents: Locator;
	readonly sectionWebSearch: Locator;
	readonly sectionWebsocketInput: Locator;
	readonly sectionWorkspace: Locator;
	readonly sectionTokenBudget: Locator;

	// Token budget controls
	readonly budgetDisplay: Locator;
	readonly budgetInput: Locator;
	readonly budgetSave: Locator;
	readonly budgetCancel: Locator;

	// WebSocket controls
	readonly wsToggleBtn: Locator;
	readonly wsUrlDisplay: Locator;
	readonly wsUrlInput: Locator;
	readonly wsUrlSave: Locator;
	readonly wsUrlCancel: Locator;

	// Help panel
	readonly helpPanel: Locator;
	readonly helpTitle: Locator;
	readonly helpPlaceholder: Locator;

	constructor(page: Page) {
		super(page);

		// Page-level
		this.heading = page.locator('[data-testid="settings-heading"]');
		this.refreshBtn = page.locator('[data-testid="settings-refresh-btn"]');
		this.errorBanner = page.locator('[data-testid="settings-error-banner"]');
		this.loadingState = page.locator('[data-testid="settings-loading"]');
		this.unavailableState = page.locator('[data-testid="settings-unavailable"]');
		this.sectionsContainer = page.locator('[data-testid="settings-sections"]');

		// Sections
		this.sectionGettingStarted = page.locator(
			'[data-testid="settings-section-getting-started"]'
		);
		this.sectionConnection = page.locator('[data-testid="settings-section-connection"]');
		this.sectionPlatformIdentity = page.locator(
			'[data-testid="settings-section-platform-identity"]'
		);
		this.sectionLLMProviders = page.locator('[data-testid="settings-section-llm-providers"]');
		this.sectionCapabilityRouting = page.locator(
			'[data-testid="settings-section-capability-routing"]'
		);
		this.sectionComponents = page.locator('[data-testid="settings-section-components"]');
		this.sectionWebSearch = page.locator('[data-testid="settings-section-web-search"]');
		this.sectionWebsocketInput = page.locator(
			'[data-testid="settings-section-websocket-input"]'
		);
		this.sectionWorkspace = page.locator('[data-testid="settings-section-workspace"]');
		this.sectionTokenBudget = page.locator('[data-testid="settings-section-token-budget"]');

		// Token budget controls
		this.budgetDisplay = page.locator('[data-testid="budget-display"]');
		this.budgetInput = page.locator('[data-testid="budget-input"]');
		this.budgetSave = page.locator('[data-testid="budget-save"]');
		this.budgetCancel = page.locator('[data-testid="budget-cancel"]');

		// WebSocket controls
		this.wsToggleBtn = page.locator('[data-testid="ws-toggle-btn"]');
		this.wsUrlDisplay = page.locator('[data-testid="ws-url-display"]');
		this.wsUrlInput = page.locator('[data-testid="ws-url-input"]');
		this.wsUrlSave = page.locator('[data-testid="ws-url-save"]');
		this.wsUrlCancel = page.locator('[data-testid="ws-url-cancel"]');

		// Help panel
		this.helpPanel = page.locator('.help-panel');
		this.helpTitle = page.locator('.help-body h3');
		this.helpPlaceholder = page.locator('.help-placeholder');
	}

	async goto(): Promise<void> {
		await this.page.goto('/settings');
		await this.waitForLoad();
	}

	/**
	 * Override base waitForLoad — the settings page has SSE connections that
	 * prevent networkidle from ever resolving. Wait for the heading or
	 * unavailable state instead, which proves hydration + data fetch completed.
	 */
	async waitForLoad(): Promise<void> {
		await this.page.waitForLoadState('domcontentloaded');
		// Settings page renders either the heading (backend available)
		// or the unavailable state (backend down). Either means loaded.
		await Promise.race([
			this.heading.waitFor({ state: 'visible', timeout: 10000 }),
			this.unavailableState.waitFor({ state: 'visible', timeout: 10000 })
		]);
	}

	async isLoaded(): Promise<boolean> {
		try {
			await expect(this.heading).toBeVisible({ timeout: 5000 });
			return true;
		} catch {
			return false;
		}
	}

	/**
	 * Get the badge locator for a section by its data-testid.
	 */
	getSectionBadge(sectionTestId: string): Locator {
		return this.page.locator(`[data-testid="${sectionTestId}"] .section-badge`);
	}

	/**
	 * Click a section's toggle button to expand/collapse it.
	 */
	async toggleSection(sectionTestId: string): Promise<void> {
		await this.page.locator(`[data-testid="${sectionTestId}"] .section-toggle`).click();
	}

	/**
	 * Check if a section is currently expanded via aria-expanded.
	 */
	async isSectionExpanded(sectionTestId: string): Promise<boolean> {
		const toggle = this.page.locator(`[data-testid="${sectionTestId}"] .section-toggle`);
		return (await toggle.getAttribute('aria-expanded')) === 'true';
	}

	/**
	 * Hover over a section's toggle to trigger the help panel update.
	 */
	async hoverSection(sectionTestId: string): Promise<void> {
		await this.page.locator(`[data-testid="${sectionTestId}"] .section-toggle`).hover();
	}

	/**
	 * Get the current help panel title text.
	 */
	async getHelpTitle(): Promise<string> {
		return (await this.helpTitle.textContent()) ?? '';
	}
}
