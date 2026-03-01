import { type Page, expect } from '@playwright/test';

/**
 * Helper class for testing SSE-driven real-time updates.
 *
 * Semdragons uses Server-Sent Events via EventSource for pushing
 * entity updates to the UI from the NATS KV watch endpoint.
 */
export class SSEHelper {
	constructor(private page: Page) {}

	/**
	 * Wait for SSE connection to be established.
	 * The UI shows a "Connected" status indicator when connected.
	 */
	async waitForConnection(timeout = 10000): Promise<void> {
		await this.page.waitForSelector('[data-testid="connection-status"].connected', { timeout });
	}

	/**
	 * Wait for SSE disconnection.
	 */
	async waitForDisconnection(timeout = 5000): Promise<void> {
		await this.page.waitForSelector('[data-testid="connection-status"]:not(.connected)', { timeout });
	}

	/**
	 * Verify connection status text.
	 */
	async expectConnected(): Promise<void> {
		await expect(this.page.locator('[data-testid="connection-status"]')).toHaveClass(/connected/);
		await expect(this.page.locator('[data-testid="connection-status"]')).toContainText('Connected');
	}

	/**
	 * Verify disconnection status text.
	 */
	async expectDisconnected(): Promise<void> {
		await expect(this.page.locator('[data-testid="connection-status"]')).not.toHaveClass(/connected/);
		await expect(this.page.locator('[data-testid="connection-status"]')).toContainText('Disconnected');
	}

	/**
	 * Wait for an entity to appear or update with specific text.
	 * Useful for verifying real-time updates from SSE events.
	 */
	async waitForEntityUpdate(selector: string, text: string, timeout = 5000): Promise<void> {
		await expect(this.page.locator(selector)).toContainText(text, { timeout });
	}

	/**
	 * Wait for a new event to appear in the activity feed.
	 */
	async waitForEventInFeed(eventType: string, timeout = 5000): Promise<void> {
		const eventSelector = '[data-testid="event-item"] .event-type';
		await expect(
			this.page.locator(eventSelector).filter({ hasText: eventType }).first()
		).toBeVisible({ timeout });
	}

	/**
	 * Get the number of events in the activity feed.
	 */
	async getEventCount(): Promise<number> {
		return await this.page.locator('[data-testid="event-item"]').count();
	}

	/**
	 * Wait for a specific number of events in the feed.
	 */
	async waitForEventCount(count: number, timeout = 5000): Promise<void> {
		await expect(this.page.locator('[data-testid="event-item"]')).toHaveCount(count, { timeout });
	}

	/**
	 * Intercept and block SSE connections.
	 * Useful for testing disconnection handling.
	 */
	async blockSSE(): Promise<void> {
		await this.page.route('**/message-logger/**', (route) => route.abort());
	}

	/**
	 * Unblock SSE connections.
	 */
	async unblockSSE(): Promise<void> {
		await this.page.unroute('**/message-logger/**');
	}

	/**
	 * Wait for a stat card to show a specific value.
	 * Stats are updated via SSE in real-time.
	 */
	async waitForStatValue(testId: string, value: string, timeout = 5000): Promise<void> {
		const statCard = this.page.locator(`[data-testid="${testId}"]`);
		await expect(statCard.locator('.stat-value')).toContainText(value, { timeout });
	}
}
