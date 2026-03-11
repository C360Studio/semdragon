import { type Page, expect } from '@playwright/test';

/**
 * Scenario helpers for Tier 2 E2E tests.
 *
 * These helpers interact with the UI the way a human would — navigating pages,
 * clicking buttons, and waiting for reactive updates via SSE. They use
 * Playwright's built-in `expect.poll()` and `expect.toPass()` for waiting,
 * avoiding custom retry loops.
 */

// =============================================================================
// TIMEOUT DEFAULTS
// =============================================================================

const DEFAULT_WAIT_TIMEOUT = 60_000;
const LONG_WAIT_TIMEOUT = 120_000;

// =============================================================================
// DM CHAT HELPERS
// =============================================================================

/**
 * Post a quest via the DM chat panel.
 *
 * Opens the chat panel if closed, types `/quest <description>`, sends it,
 * waits for the quest preview card, then clicks "Post Quest". Returns the
 * quest title extracted from the preview card.
 */
export async function postQuestViaDMChat(
	page: Page,
	description: string,
	opts?: { timeout?: number }
): Promise<string> {
	const timeout = opts?.timeout ?? DEFAULT_WAIT_TIMEOUT;

	// Open chat panel if not visible
	const input = page.getByTestId('chat-input');
	if (!(await input.isVisible().catch(() => false))) {
		await page.getByTestId('chat-toggle').click();
	}
	await expect(input).toBeVisible({ timeout: 5_000 });

	// Type the quest command and send
	await input.fill(`/quest ${description}`);
	await page.getByTestId('chat-send').click();

	// Wait for either a single quest preview or a quest chain preview.
	// The mock LLM returns quest_brief for single quests and quest_chain
	// when the prompt matches chain patterns.
	const singlePreview = page.getByTestId('quest-preview');
	const chainPreview = page.getByTestId('quest-chain-preview');

	await expect(singlePreview.or(chainPreview)).toBeVisible({ timeout });

	let titleText: string | null;
	if (await singlePreview.isVisible()) {
		titleText = await singlePreview.textContent();
		await page.getByTestId('post-quest-button').click();
	} else {
		titleText = await chainPreview.textContent();
		await page.getByTestId('post-chain-button').click();
	}

	// Wait for the confirmation message
	await expect(
		page.getByTestId('chat-message').filter({ hasText: /posted/i })
	).toBeVisible({ timeout: 10_000 });

	return titleText?.trim() ?? description;
}

// =============================================================================
// QUEST BOARD HELPERS
// =============================================================================

/**
 * Wait for a quest card matching `titleMatch` to appear in the given kanban column.
 *
 * Uses `expect.poll()` for Playwright-native waiting.
 */
export async function waitForQuestInColumn(
	page: Page,
	column: 'posted' | 'claimed' | 'in_progress' | 'in_review' | 'completed',
	titleMatch: string | RegExp,
	opts?: { timeout?: number }
): Promise<void> {
	const timeout = opts?.timeout ?? DEFAULT_WAIT_TIMEOUT;
	const columnLocator = page.locator(`[data-testid="quest-column-${column}"]`);
	const pattern = typeof titleMatch === 'string' ? new RegExp(titleMatch, 'i') : titleMatch;

	await expect
		.poll(
			async () => {
				const cards = columnLocator.locator('[data-testid="quest-card"]');
				const count = await cards.count();
				for (let i = 0; i < count; i++) {
					const text = await cards.nth(i).textContent();
					if (text && pattern.test(text)) return true;
				}
				return false;
			},
			{ timeout, message: `Quest matching "${titleMatch}" not found in ${column} column` }
		)
		.toBe(true);
}

/**
 * Wait for any quest to appear in a kanban column (regardless of title).
 * Returns the count of quests in that column.
 */
export async function waitForAnyQuestInColumn(
	page: Page,
	column: 'posted' | 'claimed' | 'in_progress' | 'in_review' | 'completed',
	opts?: { timeout?: number; minCount?: number }
): Promise<number> {
	const timeout = opts?.timeout ?? DEFAULT_WAIT_TIMEOUT;
	const minCount = opts?.minCount ?? 1;
	const columnLocator = page.locator(`[data-testid="quest-column-${column}"]`);

	let finalCount = 0;
	await expect
		.poll(
			async () => {
				finalCount = await columnLocator.locator('[data-testid="quest-card"]').count();
				return finalCount;
			},
			{ timeout, message: `Expected at least ${minCount} quest(s) in ${column} column` }
		)
		.toBeGreaterThanOrEqual(minCount);

	return finalCount;
}

// =============================================================================
// AGENT HELPERS
// =============================================================================

/**
 * Wait for an agent card on the agents page matching `nameMatch` to show `status`.
 */
export async function waitForAgentStatus(
	page: Page,
	nameMatch: string | RegExp,
	status: string,
	opts?: { timeout?: number }
): Promise<void> {
	const timeout = opts?.timeout ?? DEFAULT_WAIT_TIMEOUT;
	const pattern = typeof nameMatch === 'string' ? new RegExp(nameMatch, 'i') : nameMatch;

	await expect
		.poll(
			async () => {
				const cards = page.locator('[data-testid="agent-card"]');
				const count = await cards.count();
				for (let i = 0; i < count; i++) {
					const card = cards.nth(i);
					const name = await card.locator('[data-testid="agent-name"]').textContent();
					if (name && pattern.test(name)) {
						const statusText = await card.locator('.agent-status').textContent();
						return statusText?.trim().toLowerCase().replace(/_/g, ' ') === status.toLowerCase().replace(/_/g, ' ');
					}
				}
				return false;
			},
			{ timeout, message: `Agent "${nameMatch}" did not reach status "${status}"` }
		)
		.toBe(true);
}

// =============================================================================
// BATTLE HELPERS
// =============================================================================

/**
 * Wait for a battle card to appear with a verdict (victory or defeat).
 */
export async function waitForBattleVerdict(
	page: Page,
	opts?: { timeout?: number }
): Promise<'victory' | 'defeat'> {
	const timeout = opts?.timeout ?? LONG_WAIT_TIMEOUT;
	let verdict: 'victory' | 'defeat' = 'victory';

	await expect
		.poll(
			async () => {
				// Look for battle cards with a verdict status
				const cards = page.locator('[data-testid="battle-card"]');
				const count = await cards.count();
				for (let i = 0; i < count; i++) {
					const text = (await cards.nth(i).textContent()) ?? '';
					if (/victory/i.test(text)) {
						verdict = 'victory';
						return true;
					}
					if (/defeat/i.test(text)) {
						verdict = 'defeat';
						return true;
					}
				}
				return false;
			},
			{ timeout, message: 'No battle verdict (victory/defeat) appeared' }
		)
		.toBe(true);

	return verdict;
}

// =============================================================================
// STAT HELPERS
// =============================================================================

/**
 * Read a numeric stat value from a dashboard stat card by its data-testid.
 */
export async function getStatValue(page: Page, testId: string): Promise<number> {
	const card = page.locator(`[data-testid="${testId}"]`);
	const text = await card.locator('.stat-value').textContent();
	// Handle percentage values like "85%"
	const cleaned = (text ?? '0').replace(/[^0-9.]/g, '');
	return parseFloat(cleaned) || 0;
}

/**
 * Wait for a stat value to change from its baseline.
 */
export async function waitForStatChange(
	page: Page,
	testId: string,
	baseline: number,
	direction: 'increase' | 'decrease',
	opts?: { timeout?: number }
): Promise<number> {
	const timeout = opts?.timeout ?? LONG_WAIT_TIMEOUT;
	let finalValue = baseline;

	await expect
		.poll(
			async () => {
				finalValue = await getStatValue(page, testId);
				return direction === 'increase' ? finalValue > baseline : finalValue < baseline;
			},
			{ timeout, message: `Stat "${testId}" did not ${direction} from ${baseline}` }
		)
		.toBe(true);

	return finalValue;
}

// =============================================================================
// EVENT FEED HELPERS
// =============================================================================

/**
 * Wait for an event matching a pattern to appear in the event feed.
 */
export async function waitForEventInFeed(
	page: Page,
	pattern: string | RegExp,
	opts?: { timeout?: number }
): Promise<void> {
	const timeout = opts?.timeout ?? DEFAULT_WAIT_TIMEOUT;
	const re = typeof pattern === 'string' ? new RegExp(pattern, 'i') : pattern;

	await expect
		.poll(
			async () => {
				const items = page.locator('[data-testid="event-item"]');
				const count = await items.count();
				for (let i = 0; i < count; i++) {
					const text = await items.nth(i).textContent();
					if (text && re.test(text)) return true;
				}
				return false;
			},
			{ timeout, message: `Event matching "${pattern}" not found in feed` }
		)
		.toBe(true);
}
