import { type Page, type Locator, expect } from '@playwright/test';
import { BasePage } from './base.page';

/**
 * Page object for the Guild Registry page.
 * Located at '/guilds'.
 */
export class GuildsPage extends BasePage {
	// Header
	readonly heading: Locator;
	readonly guildCount: Locator;

	// Guild grid
	readonly guildsGrid: Locator;
	readonly guildCards: Locator;

	constructor(page: Page) {
		super(page);

		this.heading = page.locator('.page-header h1');
		this.guildCount = page.locator('.guild-count');
		this.guildsGrid = page.locator('.guilds-grid');
		this.guildCards = page.locator('[data-testid="guild-card"]');
	}

	async goto(): Promise<void> {
		await this.page.goto('/guilds');
		await this.waitForLoad();
	}

	async isLoaded(): Promise<boolean> {
		const heading = await this.heading.textContent();
		return heading?.includes('Guild Registry') ?? false;
	}

	async getGuildCount(): Promise<number> {
		const text = await this.guildCount.textContent();
		const match = text?.match(/(\d+)/);
		return match ? parseInt(match[1], 10) : 0;
	}

	async getVisibleGuildCount(): Promise<number> {
		return await this.guildCards.count();
	}

	async getGuildCardDetails(index: number): Promise<{
		name: string;
		status: string;
		description: string;
		guildmaster: string | null;
	}> {
		const card = this.guildCards.nth(index);
		const name = (await card.locator('.guild-header h2').textContent()) ?? '';
		const status = (await card.locator('.guild-status').textContent()) ?? '';
		const description = (await card.locator('.guild-description').textContent()) ?? '';

		let guildmaster: string | null = null;
		const gmEl = card.locator('.gm-name');
		if ((await gmEl.count()) > 0) {
			guildmaster = (await gmEl.textContent()) ?? null;
		}

		return {
			name: name.trim(),
			status: status.trim(),
			description: description.trim(),
			guildmaster
		};
	}

	async clickGuild(index: number): Promise<void> {
		await this.guildCards.nth(index).click();
		await this.page.waitForURL('**/guilds/**');
	}
}

/**
 * Page object for the Guild Detail page.
 * Located at '/guilds/[id]'.
 */
export class GuildDetailPage extends BasePage {
	readonly guildName: Locator;
	readonly guildStatus: Locator;
	readonly backLink: Locator;
	readonly statsBar: Locator;
	readonly guildmasterCard: Locator;
	readonly memberRows: Locator;
	readonly notFound: Locator;

	constructor(page: Page) {
		super(page);

		this.guildName = page.locator('[data-testid="guild-name"]');
		this.guildStatus = page.locator('.guild-status');
		this.backLink = page.locator('.back-link');
		this.statsBar = page.locator('.stats-bar');
		this.guildmasterCard = page.locator('.guildmaster-card');
		this.memberRows = page.locator('.member-row');
		this.notFound = page.locator('[data-testid="guild-not-found"]');
	}

	async goto(): Promise<void> {
		await this.page.goto('/guilds');
		await this.waitForLoad();
	}

	async gotoGuild(id: string): Promise<void> {
		await this.page.goto(`/guilds/${id}`);
		await this.waitForLoad();
	}

	async isLoaded(): Promise<boolean> {
		return (await this.guildName.count()) > 0;
	}

	async getGuildName(): Promise<string> {
		return (await this.guildName.textContent()) ?? '';
	}

	async hasGuildmaster(): Promise<boolean> {
		return (await this.guildmasterCard.count()) > 0;
	}

	async getGuildmasterName(): Promise<string> {
		const highlight = this.guildmasterCard.locator('.member-name');
		return (await highlight.textContent()) ?? '';
	}

	async getMemberCount(): Promise<number> {
		return await this.memberRows.count();
	}

	async isNotFound(): Promise<boolean> {
		return (await this.notFound.count()) > 0;
	}
}
