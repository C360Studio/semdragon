import { test as base, expect, type Page, type APIRequestContext } from '@playwright/test';
import { DashboardPage } from '../pages/dashboard.page';
import { QuestsPage } from '../pages/quests.page';
import { AgentsPage } from '../pages/agents.page';
import { SSEHelper } from './sse-helpers';

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
 * Extract the short instance ID from a full dotted entity ID.
 *
 * The backend returns full entity IDs like "local.dev.game.board1.quest.abc123".
 * Lifecycle endpoints expect only the instance part ("abc123").
 */
export function extractInstance(fullId: string): string {
	const parts = fullId.split('.');
	return parts[parts.length - 1];
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
 * Typed response shapes for lifecycle API operations.
 */
export interface QuestResponse {
	id: string;
	objective: string;
	status: string;
	agent_id?: string;
	attempts?: number;
	completed_at?: string;
	require_human_review?: boolean;
	review_level?: number;
	trajectory_id?: string;
	[key: string]: unknown;
}

export interface AgentResponse {
	id: string;
	name: string;
	level: number;
	tier: number;
	status: string;
	xp?: number;
	skills?: string[];
	[key: string]: unknown;
}

export interface BattleResponse {
	id: string;
	quest_id: string;
	status: string;
	verdict?: string;
	[key: string]: unknown;
}

export interface WorldStateResponse {
	agents?: AgentResponse[];
	quests?: QuestResponse[];
	[key: string]: unknown;
}

/**
 * Lifecycle API fixture type definition.
 *
 * Provides typed helpers for all quest and agent lifecycle operations.
 * All quest/agent IDs passed to these methods should be the short instance
 * portion extracted via extractInstance().
 */
export interface LifecycleApi {
	claimQuest: (questId: string, agentId: string) => Promise<Response>;
	startQuest: (questId: string) => Promise<Response>;
	submitQuest: (questId: string, output: string) => Promise<Response>;
	completeQuest: (questId: string) => Promise<Response>;
	failQuest: (questId: string, reason: string) => Promise<Response>;
	abandonQuest: (questId: string, reason: string) => Promise<Response>;
	createQuestWithReview: (objective: string, reviewLevel?: number) => Promise<QuestResponse>;
	createQuest: (objective: string, difficulty?: number) => Promise<QuestResponse>;
	recruitAgent: (name: string, skills?: string[]) => Promise<AgentResponse>;
	getQuest: (questId: string) => Promise<QuestResponse>;
	getAgent: (agentId: string) => Promise<AgentResponse>;
	listBattles: () => Promise<BattleResponse[]>;
	getWorldState: () => Promise<WorldStateResponse>;
}

/**
 * Extended test fixtures for Semdragons E2E tests.
 *
 * Provides:
 * - Page objects for each major page
 * - SSE helpers for real-time testing
 * - API client for test data seeding
 * - Lifecycle API for quest and agent state transitions
 */
export const test = base.extend<{
	dashboardPage: DashboardPage;
	questsPage: QuestsPage;
	agentsPage: AgentsPage;
	sseHelper: SSEHelper;
	seedQuests: (quests: QuestPayload[]) => Promise<string[]>;
	apiRequest: APIRequestContext;
	lifecycleApi: LifecycleApi;
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

	// SSE helper
	sseHelper: async ({ page }, use) => {
		const sseHelper = new SSEHelper(page);
		await use(sseHelper);
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
				const response = await apiRequest.post('/game/quests', {
					data: {
						objective: quest.title,
						hints: {
							suggested_difficulty: difficultyToNumber(quest.difficulty || 'easy'),
							require_human_review: false,
							budget: quest.base_xp || 100
						}
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
		void createdQuestIds;
	},

	// Lifecycle API fixture — all methods operate on short instance IDs
	lifecycleApi: async ({ playwright }, use) => {
		const apiContext = await playwright.request.newContext({
			baseURL: API_URL
		});

		const api: LifecycleApi = {
			claimQuest: (questId, agentId) =>
				fetch(`${API_URL}/game/quests/${questId}/claim`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ agent_id: agentId })
				}),

			startQuest: (questId) =>
				fetch(`${API_URL}/game/quests/${questId}/start`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({})
				}),

			submitQuest: (questId, output) =>
				fetch(`${API_URL}/game/quests/${questId}/submit`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ output })
				}),

			completeQuest: (questId) =>
				fetch(`${API_URL}/game/quests/${questId}/complete`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({})
				}),

			failQuest: (questId, reason) =>
				fetch(`${API_URL}/game/quests/${questId}/fail`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ reason })
				}),

			abandonQuest: (questId, reason) =>
				fetch(`${API_URL}/game/quests/${questId}/abandon`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ reason })
				}),

			createQuestWithReview: async (objective, reviewLevel = 1) => {
				const res = await apiContext.post('/game/quests', {
					data: {
						objective,
						hints: {
							suggested_difficulty: 2,
							suggested_skills: [],
							require_human_review: true,
							review_level: reviewLevel,
							budget: 100
						}
					}
				});
				if (!res.ok()) {
					throw new Error(`createQuestWithReview failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			createQuest: async (objective, difficulty = 1) => {
				const res = await apiContext.post('/game/quests', {
					data: {
						objective,
						hints: {
							suggested_difficulty: difficulty,
							suggested_skills: [],
							require_human_review: false,
							budget: 100
						}
					}
				});
				if (!res.ok()) {
					throw new Error(`createQuest failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			recruitAgent: async (name, skills = []) => {
				const res = await apiContext.post('/game/agents', {
					data: { name, skills, is_npc: false }
				});
				if (!res.ok()) {
					throw new Error(`recruitAgent failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getQuest: async (questId) => {
				const res = await apiContext.get(`/game/quests/${questId}`);
				if (!res.ok()) {
					throw new Error(`getQuest failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getAgent: async (agentId) => {
				const res = await apiContext.get(`/game/agents/${agentId}`);
				if (!res.ok()) {
					throw new Error(`getAgent failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			listBattles: async () => {
				const res = await apiContext.get('/game/battles');
				if (!res.ok()) {
					throw new Error(`listBattles failed: ${res.status()} ${await res.text()}`);
				}
				const data = await res.json();
				// Handle either array or wrapped response
				return Array.isArray(data) ? data : (data.battles ?? data.items ?? []);
			},

			getWorldState: async () => {
				const res = await apiContext.get('/game/world');
				if (!res.ok()) {
					throw new Error(`getWorldState failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			}
		};

		await use(api);
		await apiContext.dispose();
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

/**
 * Check if the backend is available (set by global-setup.ts).
 *
 * Tests that need the backend should call:
 *   test('my test', async () => {
 *     if (!hasBackend()) test.skip();
 *     ...
 *   });
 */
export function hasBackend(): boolean {
	return process.env.E2E_BACKEND_AVAILABLE === 'true';
}

// Re-export expect for convenience
export { expect };

// Re-export Page type
export type { Page };
