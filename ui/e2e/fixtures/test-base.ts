import { test as base, expect, type Page, type APIRequestContext } from '@playwright/test';
import { DashboardPage } from '../pages/dashboard.page';
import { QuestsPage } from '../pages/quests.page';
import { AgentsPage } from '../pages/agents.page';
import { AgentDetailPage } from '../pages/agent-detail.page';
import { GuildsPage, GuildDetailPage } from '../pages/guilds.page';
import { SettingsPage } from '../pages/settings.page';
import { SSEHelper } from './sse-helpers';
import type { components } from '../../src/lib/api/generated';

/**
 * API URL for backend interactions.
 * Playwright runs outside Docker, so we use localhost.
 */
const API_URL = process.env.API_URL || `http://localhost:${process.env.BACKEND_PORT || '8081'}`;

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
 *
 * Entity types are derived from Go structs via OpenAPI → openapi-typescript.
 * Regenerate with: make openapi && cd ui && npm run gen:api
 */
export type QuestResponse = components['schemas']['Quest'];
export type AgentResponse = components['schemas']['Agent'];
export type BattleVerdictResponse = components['schemas']['BattleVerdict'];
export type BattleResponse = components['schemas']['BossBattle'];

export type StoreItemResponse = components['schemas']['StoreItem'];

export interface PurchaseResponse {
	success: boolean;
	item?: StoreItemResponse;
	xp_spent?: number;
	xp_remaining?: number;
	inventory?: InventoryResponse;
	error?: string;
	[key: string]: unknown;
}

export interface InventoryResponse {
	agent_id: string;
	owned_tools: Record<string, unknown>;
	consumables: Record<string, number>;
	total_spent: number;
	[key: string]: unknown;
}

export type ActiveEffectResponse = components['schemas']['ActiveEffect'];

export type DMChatResponse = components['schemas']['DMChatResponse'];
export type ChatMode = DMChatResponse['mode'];

export type ReviewResponse = components['schemas']['PeerReview'];

export type DMSessionResponse = components['schemas']['DMChatSession'];
export type TokenStatsResponse = components['schemas']['TokenStats'];
export type BoardStatusResponse = components['schemas']['BoardStatusResponse'];

export type PartyResponse = components['schemas']['Party'];
export type GuildResponse = components['schemas']['Guild'];

export interface UseConsumableResponse {
	success: boolean;
	remaining?: number;
	active_effects?: ActiveEffectResponse[];
	error?: string;
	[key: string]: unknown;
}

export interface TrajectoryStep {
	timestamp: string;
	step_type: 'model_call' | 'tool_call';
	request_id?: string;
	prompt?: string;
	response?: string;
	tokens_in?: number;
	tokens_out?: number;
	tool_name?: string;
	tool_arguments?: Record<string, unknown>;
	tool_result?: string;
	duration: number;
}

export interface TrajectoryResponse {
	loop_id: string;
	start_time: string;
	end_time?: string;
	steps: TrajectoryStep[];
	outcome?: string;
	total_tokens_in: number;
	total_tokens_out: number;
	duration: number;
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
	listQuests: () => Promise<QuestResponse[]>;
	listStore: (agentId?: string) => Promise<StoreItemResponse[]>;
	getStoreItem: (itemId: string) => Promise<StoreItemResponse>;
	purchaseItem: (agentId: string, itemId: string) => Promise<PurchaseResponse>;
	getInventory: (agentId: string) => Promise<InventoryResponse>;
	useConsumable: (
		agentId: string,
		consumableId: string,
		questId?: string
	) => Promise<UseConsumableResponse>;
	getEffects: (agentId: string) => Promise<ActiveEffectResponse[]>;
	sendDMChat: (
		message: string,
		sessionId?: string,
		mode?: ChatMode,
		timeoutMs?: number
	) => Promise<DMChatResponse>;
	postQuestChain: (chain: components['schemas']['QuestChainBrief']) => Promise<QuestResponse[]>;
	getTrajectory: (loopId: string) => Promise<TrajectoryResponse>;
	createReview: (
		questId: string,
		leaderId: string,
		memberId: string,
		isSoloTask?: boolean
	) => Promise<ReviewResponse>;
	submitReview: (
		reviewId: string,
		reviewerId: string,
		ratings: { q1: number; q2: number; q3: number },
		explanation?: string
	) => Promise<ReviewResponse>;
	getReview: (reviewId: string) => Promise<ReviewResponse>;
	listReviews: (status?: string, questId?: string) => Promise<ReviewResponse[]>;
	getAgentReviews: (agentId: string) => Promise<ReviewResponse[]>;
	getDMSession: (sessionId: string) => Promise<DMSessionResponse | null>;
	getTokenStats: () => Promise<TokenStatsResponse>;
	getBoardStatus: () => Promise<BoardStatusResponse>;
	pauseBoard: (actor?: string) => Promise<BoardStatusResponse>;
	resumeBoard: () => Promise<BoardStatusResponse>;
	listParties: () => Promise<PartyResponse[]>;
	getParty: (id: string) => Promise<PartyResponse>;
	createQuestWithParty: (objective: string, minPartySize?: number) => Promise<QuestResponse>;
	listGuilds: () => Promise<GuildResponse[]>;
	getGuild: (id: string) => Promise<GuildResponse>;
	recruitAgentAtLevel: (name: string, level: number, skills?: string[]) => Promise<AgentResponse>;
	listQuestArtifacts: (questId: string) => Promise<{ quest_id: string; files: string[]; count: number }>;
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
	agentDetailPage: AgentDetailPage;
	guildsPage: GuildsPage;
	guildDetailPage: GuildDetailPage;
	settingsPage: SettingsPage;
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

	agentDetailPage: async ({ page }, use) => {
		await use(new AgentDetailPage(page));
	},

	guildsPage: async ({ page }, use) => {
		await use(new GuildsPage(page));
	},

	guildDetailPage: async ({ page }, use) => {
		await use(new GuildDetailPage(page));
	},

	settingsPage: async ({ page }, use) => {
		await use(new SettingsPage(page));
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
							suggested_difficulty: 1,
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
			},

			listQuests: async () => {
				const res = await apiContext.get('/game/quests');
				if (!res.ok()) {
					throw new Error(`listQuests failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			listStore: async (agentId?) => {
				const query = agentId ? `?agent_id=${agentId}` : '';
				const res = await apiContext.get(`/game/store${query}`);
				if (!res.ok()) {
					throw new Error(`listStore failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getStoreItem: async (itemId) => {
				const res = await apiContext.get(`/game/store/${itemId}`);
				if (!res.ok()) {
					throw new Error(`getStoreItem failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			purchaseItem: async (agentId, itemId) => {
				const res = await apiContext.post('/game/store/purchase', {
					data: { agent_id: agentId, item_id: itemId }
				});
				// Parse body regardless of status — the purchase endpoint uses
				// the body to communicate success/failure. Non-2xx responses
				// (e.g., 403 tier gate) return {"error": "..."} without a
				// success field, so we normalize to always include it.
				const body = await res.text();
				try {
					const parsed = JSON.parse(body);
					if (!res.ok() && parsed.success === undefined) {
						parsed.success = false;
					}
					return parsed;
				} catch {
					return { success: false, error: `HTTP ${res.status()}: ${body}` };
				}
			},

			getInventory: async (agentId) => {
				const res = await apiContext.get(`/game/agents/${agentId}/inventory`);
				if (!res.ok()) {
					throw new Error(`getInventory failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			useConsumable: async (agentId, consumableId, questId?) => {
				const data: Record<string, string> = { consumable_id: consumableId };
				if (questId) data.quest_id = questId;
				const res = await apiContext.post(`/game/agents/${agentId}/inventory/use`, {
					data
				});
				if (!res.ok()) {
					throw new Error(`useConsumable failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getEffects: async (agentId) => {
				const res = await apiContext.get(`/game/agents/${agentId}/effects`);
				if (!res.ok()) {
					throw new Error(`getEffects failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			sendDMChat: async (message, sessionId?, mode?, timeoutMs?) => {
				const body: Record<string, unknown> = { message };
				if (sessionId) body.session_id = sessionId;
				if (mode) body.mode = mode;
				const res = await apiContext.post('/game/dm/chat', {
					data: body,
					timeout: timeoutMs ?? 130_000
				});
				if (!res.ok()) {
					throw new Error(`sendDMChat failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			postQuestChain: async (chain) => {
				const res = await apiContext.post('/game/quests/chain', { data: chain });
				if (!res.ok()) {
					throw new Error(`postQuestChain failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getTrajectory: async (loopId) => {
				const res = await apiContext.get(`/game/trajectories/${loopId}`);
				if (!res.ok()) {
					throw new Error(`getTrajectory failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			createReview: async (questId, leaderId, memberId, isSoloTask = false) => {
				const res = await apiContext.post('/game/reviews', {
					data: {
						quest_id: questId,
						leader_id: leaderId,
						member_id: memberId,
						is_solo_task: isSoloTask
					}
				});
				if (!res.ok()) {
					throw new Error(`createReview failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			submitReview: async (reviewId, reviewerId, ratings, explanation?) => {
				const data: Record<string, unknown> = {
					reviewer_id: reviewerId,
					ratings
				};
				if (explanation) data.explanation = explanation;
				const res = await apiContext.post(`/game/reviews/${reviewId}/submit`, { data });
				if (!res.ok()) {
					throw new Error(`submitReview failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getReview: async (reviewId) => {
				const res = await apiContext.get(`/game/reviews/${reviewId}`);
				if (!res.ok()) {
					throw new Error(`getReview failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			listReviews: async (status?, questId?) => {
				const params = new URLSearchParams();
				if (status) params.set('status', status);
				if (questId) params.set('quest_id', questId);
				const qs = params.toString();
				const res = await apiContext.get(`/game/reviews${qs ? `?${qs}` : ''}`);
				if (!res.ok()) {
					throw new Error(`listReviews failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getAgentReviews: async (agentId) => {
				const res = await apiContext.get(`/game/agents/${agentId}/reviews`);
				if (!res.ok()) {
					throw new Error(`getAgentReviews failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getDMSession: async (sessionId) => {
				const res = await apiContext.get(`/game/dm/sessions/${sessionId}`);
				if (res.status() === 404) return null;
				if (!res.ok()) {
					throw new Error(`getDMSession failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getTokenStats: async () => {
				const res = await apiContext.get('/game/board/tokens');
				if (!res.ok()) {
					throw new Error(`getTokenStats failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			getBoardStatus: async () => {
				const res = await apiContext.get('/game/board/status');
				if (!res.ok()) {
					throw new Error(`getBoardStatus failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			pauseBoard: async (actor?) => {
				const data: Record<string, string> = {};
				if (actor) data.actor = actor;
				const res = await apiContext.post('/game/board/pause', { data });
				if (!res.ok()) {
					throw new Error(`pauseBoard failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			resumeBoard: async () => {
				const res = await apiContext.post('/game/board/resume', { data: {} });
				if (!res.ok()) {
					throw new Error(`resumeBoard failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			listParties: async () => {
				const res = await apiContext.get('/game/parties');
				if (!res.ok()) {
					throw new Error(`listParties failed: ${res.status()} ${await res.text()}`);
				}
				const data = await res.json();
				return Array.isArray(data) ? data : (data.parties ?? data.items ?? []);
			},

			getParty: async (id) => {
				const res = await apiContext.get(`/game/parties/${id}`);
				if (!res.ok()) {
					throw new Error(`getParty failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			createQuestWithParty: async (objective, minPartySize = 2) => {
				const res = await apiContext.post('/game/quests', {
					data: {
						objective,
						hints: {
							suggested_difficulty: 1,
							suggested_skills: [],
							require_human_review: false,
							party_required: true,
							min_party_size: minPartySize,
							budget: 100
						}
					}
				});
				if (!res.ok()) {
					throw new Error(`createQuestWithParty failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			listGuilds: async () => {
				const res = await apiContext.get('/game/guilds');
				if (!res.ok()) {
					throw new Error(`listGuilds failed: ${res.status()} ${await res.text()}`);
				}
				const data = await res.json();
				return Array.isArray(data) ? data : (data.guilds ?? data.items ?? []);
			},

			getGuild: async (id) => {
				const res = await apiContext.get(`/game/guilds/${id}`);
				if (!res.ok()) {
					throw new Error(`getGuild failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			recruitAgentAtLevel: async (name, level, skills = []) => {
				const res = await apiContext.post('/game/agents', {
					data: { name, skills, is_npc: false, level }
				});
				if (!res.ok()) {
					throw new Error(`recruitAgentAtLevel failed: ${res.status()} ${await res.text()}`);
				}
				return res.json();
			},

			listQuestArtifacts: async (questId) => {
				const res = await apiContext.get(`/game/quests/${questId}/artifacts/list`);
				if (!res.ok()) {
					// 503 = filestore not available, 404 = no artifacts — both mean empty.
					if (res.status() === 503 || res.status() === 404) {
						return { quest_id: questId, files: [], count: 0 };
					}
					throw new Error(`listQuestArtifacts failed: ${res.status()} ${await res.text()}`);
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

/**
 * LLM mode for E2E tests.
 *
 * Controls which LLM backend the tests expect:
 *   - 'none'      — skip all LLM-dependent tests (default)
 *   - 'mock'      — deterministic mock LLM server (canned responses)
 *   - 'gemini'    — Google Gemini (real, nondeterministic)
 *   - 'openai'    — OpenAI (real, nondeterministic)
 *   - 'anthropic' — Anthropic (real, nondeterministic)
 *   - 'ollama'    — Ollama local (real, nondeterministic)
 *
 * Set via E2E_LLM_MODE env var. For backwards compatibility,
 * E2E_MOCK_LLM=true is treated as mode 'mock' and
 * E2E_REAL_LLM=true is treated as mode 'real' (generic).
 */
export type LLMMode = 'none' | 'mock' | 'gemini' | 'openai' | 'anthropic' | 'ollama' | 'real';

export function getLLMMode(): LLMMode {
	const mode = process.env.E2E_LLM_MODE;
	if (mode) return mode as LLMMode;

	// Backwards compatibility with legacy env vars
	if (process.env.E2E_MOCK_LLM === 'true') return 'mock';
	if (process.env.E2E_REAL_LLM === 'true') return 'real';
	if (process.env.E2E_OLLAMA === 'true') return 'ollama';
	return 'none';
}

/** Whether any LLM (mock or real) is available for tests. */
export function hasLLM(): boolean {
	return getLLMMode() !== 'none';
}

/** Whether the LLM is a real provider (not mock, not none). */
export function isRealLLM(): boolean {
	return !['none', 'mock'].includes(getLLMMode());
}

/** Whether the LLM is the deterministic mock server. */
export function isMockLLM(): boolean {
	return getLLMMode() === 'mock';
}

/**
 * Check if a specific component is running on the backend.
 * Calls GET /game/settings and checks the components list.
 * Returns false if backend is unavailable or component not found.
 */
export async function isComponentEnabled(componentName: string): Promise<boolean> {
	if (!hasBackend()) return false;
	try {
		const port = process.env.BACKEND_PORT || '8081';
		const resp = await fetch(`http://localhost:${port}/game/settings`);
		if (!resp.ok) return false;
		const settings = (await resp.json()) as {
			components?: Array<{ name: string; status?: string }>;
		};
		return (
			settings.components?.some(
				(c) => c.name === componentName && c.status === 'running'
			) ?? false
		);
	} catch {
		return false;
	}
}

// Re-export expect for convenience
export { expect };

// Re-export Page type
export type { Page };
