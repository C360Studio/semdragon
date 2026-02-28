import { test as base, expect, type Page, type APIRequestContext } from '@playwright/test';
import { DashboardPage } from '../pages/dashboard.page';
import { QuestsPage } from '../pages/quests.page';
import { AgentsPage } from '../pages/agents.page';
import { WebSocketHelper } from './websocket-helpers';

/**
 * API URL for backend interactions.
 * Playwright runs outside Docker, so we use localhost.
 */
const API_URL = process.env.API_URL || 'http://localhost:8080';

/**
 * Wait for SvelteKit hydration to complete.
 *
 * CRITICAL: Hydration must complete before Svelte 5 reactivity ($state, $derived) works.
 * Use this before interacting with reactive components.
 *
 * The app.html should have: <body class="%sveltekit.page_class%">
 * And +layout.svelte should add 'hydrated' class on mount.
 *
 * If the app doesn't use a hydrated class, we fall back to waiting for
 * network idle and DOM content loaded.
 */
export async function waitForHydration(page: Page, timeout = 10000): Promise<void> {
	// Try to wait for hydrated class if the app uses it
	try {
		await page.locator('body.hydrated').waitFor({ state: 'attached', timeout: 2000 });
		return;
	} catch {
		// App doesn't use hydrated class, fall back to network idle
	}

	// Fallback: wait for network to be idle (no pending requests for 500ms)
	await page.waitForLoadState('networkidle', { timeout });
}

/**
 * Wait for the backend to be healthy.
 *
 * Use this before tests that need the full backend stack.
 */
export async function waitForBackendHealth(baseURL: string, timeout = 30000): Promise<void> {
	const start = Date.now();
	const healthURL = `${baseURL}/health`;

	while (Date.now() - start < timeout) {
		try {
			const response = await fetch(healthURL);
			if (response.ok) {
				return;
			}
		} catch {
			// Backend not ready yet
		}
		await new Promise((resolve) => setTimeout(resolve, 500));
	}

	throw new Error(`Backend health check timed out after ${timeout}ms`);
}

/**
 * Retry a function until it succeeds or times out.
 *
 * Useful for waiting on async state updates.
 */
export async function retry<T>(
	fn: () => Promise<T>,
	options: { timeout?: number; interval?: number; message?: string } = {}
): Promise<T> {
	const { timeout = 10000, interval = 500, message = 'Retry timed out' } = options;
	const start = Date.now();

	while (Date.now() - start < timeout) {
		try {
			return await fn();
		} catch {
			await new Promise((resolve) => setTimeout(resolve, interval));
		}
	}

	throw new Error(message);
}

/**
 * Quest creation payload for test data seeding.
 */
interface QuestPayload {
	title: string;
	description?: string;
	difficulty?: 'trivial' | 'easy' | 'moderate' | 'hard' | 'epic' | 'legendary';
	base_xp?: number;
	required_skills?: string[];
}

/**
 * Extended test fixtures for Semdragons E2E tests.
 *
 * Provides:
 * - Page objects for each major page
 * - WebSocket helpers for real-time testing
 * - API client for test data seeding
 */
export const test = base.extend<{
	dashboardPage: DashboardPage;
	questsPage: QuestsPage;
	agentsPage: AgentsPage;
	wsHelper: WebSocketHelper;
	seedQuests: (quests: QuestPayload[]) => Promise<string[]>;
	apiRequest: APIRequestContext;
}>({
	// Page objects
	dashboardPage: async ({ page }, use) => {
		const dashboardPage = new DashboardPage(page);
		await use(dashboardPage);
	},

	questsPage: async ({ page }, use) => {
		const questsPage = new QuestsPage(page);
		await use(questsPage);
	},

	agentsPage: async ({ page }, use) => {
		const agentsPage = new AgentsPage(page);
		await use(agentsPage);
	},

	// WebSocket helper
	wsHelper: async ({ page }, use) => {
		const wsHelper = new WebSocketHelper(page);
		await use(wsHelper);
	},

	// API request context for backend operations
	apiRequest: async ({ playwright }, use) => {
		const apiContext = await playwright.request.newContext({
			baseURL: API_URL
		});
		await use(apiContext);
		await apiContext.dispose();
	},

	// Quest seeding fixture
	seedQuests: async ({ apiRequest }, use) => {
		const createdQuestIds: string[] = [];

		const seedQuests = async (quests: QuestPayload[]): Promise<string[]> => {
			const ids: string[] = [];

			for (const quest of quests) {
				const response = await apiRequest.post('/quests', {
					data: {
						title: quest.title,
						description: quest.description || `Test quest: ${quest.title}`,
						difficulty: difficultyToNumber(quest.difficulty || 'easy'),
						base_xp: quest.base_xp || 100,
						required_skills: quest.required_skills || []
					}
				});

				if (response.ok()) {
					const data = await response.json();
					ids.push(data.id);
					createdQuestIds.push(data.id);
				}
			}

			return ids;
		};

		await use(seedQuests);

		// Cleanup: Cancel any quests we created (if needed)
		// Note: E2E tests typically run against a fresh environment
		// so cleanup may not be necessary if using docker-compose down -v
	}
});

/**
 * Convert difficulty string to numeric value.
 */
function difficultyToNumber(
	difficulty: 'trivial' | 'easy' | 'moderate' | 'hard' | 'epic' | 'legendary'
): number {
	const map: Record<string, number> = {
		trivial: 0,
		easy: 1,
		moderate: 2,
		hard: 3,
		epic: 4,
		legendary: 5
	};
	return map[difficulty] ?? 1;
}

// Re-export expect for convenience
export { expect };

// Re-export Page type and helpers
export type { Page };
export { waitForHydration, waitForBackendHealth, retry };
