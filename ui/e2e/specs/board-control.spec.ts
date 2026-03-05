import { test, expect, hasBackend, waitForHydration } from '../fixtures/test-base';

/**
 * Board control E2E tests for the GameStatusBar component.
 *
 * The status bar is rendered globally in +layout.svelte and appears on every
 * page. Tests are split into two categories:
 *
 * - Layout tests: No backend required. Verify the DOM structure and ARIA
 *   attributes that must be present regardless of server state.
 *
 * - API tests: Require a live backend. Verify that the pause/resume REST
 *   endpoints behave correctly and that the UI responds to state changes.
 */

test.describe('GameStatusBar - Layout', () => {
	test('status bar is visible on the dashboard', async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);

		const statusBar = page.locator('[data-testid="game-status-bar"]');
		await expect(statusBar).toBeVisible();
	});

	test('status bar is visible on the quests page', async ({ page }) => {
		await page.goto('/quests');
		await waitForHydration(page);

		const statusBar = page.locator('[data-testid="game-status-bar"]');
		await expect(statusBar).toBeVisible();
	});

	test('status bar is visible on the agents page', async ({ page }) => {
		await page.goto('/agents');
		await waitForHydration(page);

		const statusBar = page.locator('[data-testid="game-status-bar"]');
		await expect(statusBar).toBeVisible();
	});

	test('status bar is visible on the guilds page', async ({ page }) => {
		await page.goto('/guilds');
		await waitForHydration(page);

		const statusBar = page.locator('[data-testid="game-status-bar"]');
		await expect(statusBar).toBeVisible();
	});

	test('status bar is visible on the battles page', async ({ page }) => {
		await page.goto('/battles');
		await waitForHydration(page);

		const statusBar = page.locator('[data-testid="game-status-bar"]');
		await expect(statusBar).toBeVisible();
	});

	test('shows connection indicator', async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);

		const connectionStatus = page.locator('[data-testid="connection-status"]');
		await expect(connectionStatus).toBeVisible();
		// Should display one of the two possible states
		await expect(connectionStatus).toContainText(/Connected|Disconnected/);
	});

	test('has proper role="status" ARIA attribute on the bar', async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);

		const statusBar = page.locator('[data-testid="game-status-bar"]');
		await expect(statusBar).toHaveAttribute('role', 'status');
	});

	test('has aria-live="polite" on the bar for screen-reader announcements', async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);

		const statusBar = page.locator('[data-testid="game-status-bar"]');
		await expect(statusBar).toHaveAttribute('aria-live', 'polite');
	});

	test('toggle button has descriptive aria-label', async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);

		const toggleButton = page.locator('[data-testid="board-toggle"]');
		await expect(toggleButton).toBeVisible();
		// The aria-label changes based on board state; it must always be present
		await expect(toggleButton).toHaveAttribute('aria-label', /.+/);
	});

	test('status dot is present and hidden from screen readers', async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);

		const statusDot = page.locator('[data-testid="status-dot"]');
		await expect(statusDot).toBeVisible();
		await expect(statusDot).toHaveAttribute('aria-hidden', 'true');
	});

	test('status label is present', async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);

		const statusLabel = page.locator('[data-testid="status-label"]');
		await expect(statusLabel).toBeVisible();
		// Must show one of the two board states
		await expect(statusLabel).toContainText(/Running|Paused/);
	});
});

// Board state is global — both API and UI tests mutate it. Wrap in an outer
// serial describe so the two inner blocks never run on different workers.
// Limit to chromium to prevent cross-browser contention over the shared backend.
test.describe('Board Control', () => {
	test.describe.configure({ mode: 'serial' });
	test.skip(({ browserName }) => browserName !== 'chromium', 'Board mutation tests run on chromium only to avoid cross-browser state contention');

	test.describe('Board Control - API', () => {
	test('GET /game/board/status returns board status', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		const status = await lifecycleApi.getBoardStatus();

		// paused field must be a boolean
		expect(typeof status.paused).toBe('boolean');
		// paused_at is null when running, a string when paused
		if (status.paused) {
			expect(typeof status.paused_at).toBe('string');
		} else {
			expect(status.paused_at).toBeNull();
		}
	});

	test('POST /game/board/pause transitions board to paused state', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		// Ensure we start from running state
		await lifecycleApi.resumeBoard();

		const result = await lifecycleApi.pauseBoard('e2e-test');

		expect(result.paused).toBe(true);
		expect(typeof result.paused_at).toBe('string');

		// Clean up: return board to running
		await lifecycleApi.resumeBoard();
	});

	test('POST /game/board/resume transitions board to running state', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		// Ensure we start from paused state
		await lifecycleApi.pauseBoard('e2e-test');

		const result = await lifecycleApi.resumeBoard();

		expect(result.paused).toBe(false);
		expect(result.paused_at).toBeNull();
	});

	test('pause is idempotent - double pause does not error', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();

		// First pause
		const first = await lifecycleApi.pauseBoard('e2e-test');
		expect(first.paused).toBe(true);

		// Second pause on already-paused board should not throw
		const second = await lifecycleApi.pauseBoard('e2e-test-2');
		expect(second.paused).toBe(true);

		// Clean up
		await lifecycleApi.resumeBoard();
	});

	test('resume is idempotent - double resume does not error', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.pauseBoard('e2e-test');

		// First resume
		const first = await lifecycleApi.resumeBoard();
		expect(first.paused).toBe(false);

		// Second resume on already-running board should not throw
		const second = await lifecycleApi.resumeBoard();
		expect(second.paused).toBe(false);
	});

	test('board status reflects pause after pause call', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();
		await lifecycleApi.pauseBoard('e2e-test');

		const status = await lifecycleApi.getBoardStatus();
		expect(status.paused).toBe(true);

		// Clean up
		await lifecycleApi.resumeBoard();
	});

	test('board status reflects running after resume call', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.pauseBoard('e2e-test');
		await lifecycleApi.resumeBoard();

		const status = await lifecycleApi.getBoardStatus();
		expect(status.paused).toBe(false);
		expect(status.paused_at).toBeNull();
	});
});

	test.describe('Board Control - UI Integration', () => {

	test('status bar shows "Running" as initial label when board is running', async ({
		page,
		lifecycleApi
	}) => {
		if (!hasBackend()) test.skip();

		// Ensure board is running before navigating
		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const statusLabel = page.locator('[data-testid="status-label"]');
		await expect(statusLabel).toContainText('Running');
	});

	test('clicking Pause changes status label to "Paused"', async ({ page, lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		// Ensure board starts running
		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const statusLabel = page.locator('[data-testid="status-label"]');
		await expect(statusLabel).toContainText('Running');

		const toggleButton = page.locator('[data-testid="board-toggle"]');
		await toggleButton.click();

		// Wait for the label to reflect the new state
		await expect(statusLabel).toContainText('Paused', { timeout: 5000 });

		// Clean up
		await lifecycleApi.resumeBoard();
	});

	test('paused state adds "paused" CSS class to the status bar', async ({
		page,
		lifecycleApi
	}) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const statusBar = page.locator('[data-testid="game-status-bar"]');

		// Running state — should not have paused class
		await expect(statusBar).not.toHaveClass(/paused/);

		// Pause the board via UI
		const toggleButton = page.locator('[data-testid="board-toggle"]');
		await toggleButton.click();

		// Wait for paused class to appear
		await expect(statusBar).toHaveClass(/paused/, { timeout: 5000 });

		// Clean up
		await lifecycleApi.resumeBoard();
	});

	test('clicking Resume changes status label back to "Running"', async ({
		page,
		lifecycleApi
	}) => {
		if (!hasBackend()) test.skip();

		// Start in paused state
		await lifecycleApi.pauseBoard('e2e-test');

		await page.goto('/');
		await waitForHydration(page);

		const statusLabel = page.locator('[data-testid="status-label"]');
		await expect(statusLabel).toContainText('Paused');

		const toggleButton = page.locator('[data-testid="board-toggle"]');
		await toggleButton.click();

		await expect(statusLabel).toContainText('Running', { timeout: 5000 });
	});

	test('paused detail shows "just now" immediately after pausing', async ({
		page,
		lifecycleApi
	}) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const toggleButton = page.locator('[data-testid="board-toggle"]');
		await toggleButton.click();

		const statusDetail = page.locator('[data-testid="status-detail"]');
		await expect(statusDetail).toContainText('just now', { timeout: 5000 });

		// Clean up
		await lifecycleApi.resumeBoard();
	});

	test('toggle button text changes between "Pause" and "Resume"', async ({
		page,
		lifecycleApi
	}) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const toggleButton = page.locator('[data-testid="board-toggle"]');

		// Running state: button shows Pause
		await expect(toggleButton).toContainText('Pause');

		// Click to pause
		await toggleButton.click();
		// Paused state: button shows Resume
		await expect(toggleButton).toContainText('Resume', { timeout: 5000 });

		// Click to resume
		await toggleButton.click();
		// Running state again: button shows Pause
		await expect(toggleButton).toContainText('Pause', { timeout: 5000 });
	});

	test('toggle button aria-label updates to reflect board state', async ({
		page,
		lifecycleApi
	}) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const toggleButton = page.locator('[data-testid="board-toggle"]');
		await expect(toggleButton).toHaveAttribute('aria-label', 'Pause game board');

		await toggleButton.click();
		await expect(toggleButton).toHaveAttribute('aria-label', 'Resume game board', {
			timeout: 5000
		});

		// Clean up
		await lifecycleApi.resumeBoard();
	});

	test('toggle button is disabled while a toggle request is in flight', async ({
		page,
		lifecycleApi
	}) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const toggleButton = page.locator('[data-testid="board-toggle"]');

		// Intercept the pause request to delay it so we can observe the disabled state.
		// Use a longer delay to give slow browsers (Firefox) time to assert.
		await page.route('**/game/board/pause', async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 1000));
			await route.continue();
		});

		// Click and immediately check for disabled state before the response arrives
		const clickPromise = toggleButton.click();
		await expect(toggleButton).toBeDisabled({ timeout: 2000 });

		// Wait for the click to fully resolve
		await clickPromise;

		// After toggle completes, button should be enabled again
		await expect(toggleButton).toBeEnabled({ timeout: 5000 });

		// Clean up
		await lifecycleApi.resumeBoard();
	});

	test('running state shows agent and quest counts in detail', async ({ page, lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const statusLabel = page.locator('[data-testid="status-label"]');
		await expect(statusLabel).toContainText('Running');

		const statusDetail = page.locator('[data-testid="status-detail"]');
		// Running detail shows "{N} agents, {M} quests active"
		await expect(statusDetail).toContainText(/\d+ agents/);
		await expect(statusDetail).toContainText(/\d+ quests active/);
	});

	test('status dot has running class when board is running', async ({ page, lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.resumeBoard();

		await page.goto('/');
		await waitForHydration(page);

		const statusDot = page.locator('[data-testid="status-dot"]');
		await expect(statusDot).toHaveClass(/running/);
		await expect(statusDot).not.toHaveClass(/paused/);
	});

	test('status dot has paused class when board is paused', async ({ page, lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		await lifecycleApi.pauseBoard('e2e-test');

		await page.goto('/');
		await waitForHydration(page);

		const statusDot = page.locator('[data-testid="status-dot"]');
		await expect(statusDot).toHaveClass(/paused/);
		await expect(statusDot).not.toHaveClass(/running/);

		// Clean up
		await lifecycleApi.resumeBoard();
	});
});

}); // end Board Control (outer serial wrapper)

test.describe('Token Stats API', () => {
	test('GET /game/board/tokens returns cost fields', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		const stats = await lifecycleApi.getTokenStats();

		// Cost fields must be present and numeric
		expect(typeof stats.hourly_cost_usd).toBe('number');
		expect(typeof stats.total_cost_usd).toBe('number');
		expect(stats.hourly_cost_usd).toBeGreaterThanOrEqual(0);
		expect(stats.total_cost_usd).toBeGreaterThanOrEqual(0);
	});

	test('GET /game/board/tokens returns token budget fields', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		const stats = await lifecycleApi.getTokenStats();

		expect(typeof stats.hourly_limit).toBe('number');
		expect(typeof stats.budget_pct).toBe('number');
		expect(typeof stats.breaker).toBe('string');
		expect(stats.hourly_usage).toBeDefined();
		expect(typeof stats.hourly_usage.total_tokens).toBe('number');
	});
});

test.describe('World State - Cost Fields', () => {
	test('GET /game/world includes cost fields in stats', async ({ lifecycleApi }) => {
		if (!hasBackend()) test.skip();

		const world = await lifecycleApi.getWorldState();
		const stats = world as Record<string, unknown>;
		const worldStats = (stats.stats ?? stats) as Record<string, unknown>;

		expect(typeof worldStats.cost_used_hourly_usd).toBe('number');
		expect(typeof worldStats.cost_total_usd).toBe('number');
	});
});

test.describe('GameStatusBar - Token Chip', () => {
	test('token chip has accessible aria-label with token usage info', async ({ page }) => {
		if (!hasBackend()) test.skip();

		await page.goto('/');
		await waitForHydration(page);

		const tokenChip = page.locator('[data-testid="token-chip"]');
		// Chip may not be visible if tokens_used is 0 and limit is 0
		const chipCount = await tokenChip.count();
		if (chipCount > 0) {
			await expect(tokenChip).toHaveAttribute('aria-label', /Token usage:/);
		}
	});

	test('token chip title tooltip includes hourly limit info', async ({ page }) => {
		if (!hasBackend()) test.skip();

		await page.goto('/');
		await waitForHydration(page);

		const tokenChip = page.locator('[data-testid="token-chip"]');
		const chipCount = await tokenChip.count();
		if (chipCount > 0) {
			const title = await tokenChip.getAttribute('title');
			// Title should contain token count info
			expect(title).toMatch(/tokens|hourly/i);
		}
	});
});
