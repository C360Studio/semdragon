import { type Page, expect } from '@playwright/test';

/**
 * Helper class for testing WebSocket-driven real-time updates.
 *
 * Semdragons uses WebSocket for pushing game events to the UI.
 * This helper provides utilities to wait for connection status
 * and verify real-time updates.
 */
export class WebSocketHelper {
	constructor(private page: Page) {}

	/**
	 * Wait for WebSocket connection to be established.
	 * The UI shows a "Connected" status indicator when connected.
	 */
	async waitForConnection(timeout = 10000): Promise<void> {
		await this.page.waitForSelector('.connection-status.connected', { timeout });
	}

	/**
	 * Wait for WebSocket disconnection.
	 */
	async waitForDisconnection(timeout = 5000): Promise<void> {
		await this.page.waitForSelector('.connection-status:not(.connected)', { timeout });
	}

	/**
	 * Verify connection status text.
	 */
	async expectConnected(): Promise<void> {
		await expect(this.page.locator('.connection-status')).toHaveClass(/connected/);
		await expect(this.page.locator('.connection-status')).toContainText('Connected');
	}

	/**
	 * Verify disconnection status text.
	 */
	async expectDisconnected(): Promise<void> {
		await expect(this.page.locator('.connection-status')).not.toHaveClass(/connected/);
		await expect(this.page.locator('.connection-status')).toContainText('Disconnected');
	}

	/**
	 * Wait for an entity to appear or update with specific text.
	 * Useful for verifying real-time updates from WebSocket events.
	 */
	async waitForEntityUpdate(selector: string, text: string, timeout = 5000): Promise<void> {
		await expect(this.page.locator(selector)).toContainText(text, { timeout });
	}

	/**
	 * Wait for a new event to appear in the activity feed.
	 */
	async waitForEventInFeed(eventType: string, timeout = 5000): Promise<void> {
		const eventSelector = `.event-item .event-type`;
		await expect(
			this.page.locator(eventSelector).filter({ hasText: eventType }).first()
		).toBeVisible({ timeout });
	}

	/**
	 * Get the number of events in the activity feed.
	 */
	async getEventCount(): Promise<number> {
		return await this.page.locator('.event-item').count();
	}

	/**
	 * Wait for a specific number of events in the feed.
	 */
	async waitForEventCount(count: number, timeout = 5000): Promise<void> {
		await expect(this.page.locator('.event-item')).toHaveCount(count, { timeout });
	}

	/**
	 * Intercept and block WebSocket connections.
	 * Useful for testing disconnection handling.
	 */
	async blockWebSocket(): Promise<void> {
		await this.page.route('**/events', (route) => route.abort());
	}

	/**
	 * Unblock WebSocket connections.
	 */
	async unblockWebSocket(): Promise<void> {
		await this.page.unroute('**/events');
	}

	/**
	 * Wait for a stat card to show a specific value.
	 * Stats are updated via WebSocket in real-time.
	 */
	async waitForStatValue(statLabel: string, value: string, timeout = 5000): Promise<void> {
		const statCard = this.page.locator('.stat-card').filter({
			has: this.page.locator('.stat-label', { hasText: statLabel })
		});
		await expect(statCard.locator('.stat-value')).toContainText(value, { timeout });
	}
}
