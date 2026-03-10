/**
 * API Service - HTTP client for Semdragons backend
 *
 * All domain endpoints use /game/ prefix.
 * SSE handles initial hydration and live updates; REST is for mutations and on-demand queries.
 */

import type {
	Agent,
	AgentID,
	Quest,
	QuestID,
	BossBattle,
	BattleID,
	WorldState,
	QuestHints,
	Intervention,
	AgentConfig,
	StoreItem,
	AgentInventory,
	PurchaseRequest,
	PurchaseResponse,
	UseConsumableRequest,
	UseConsumableResponse,
	ActiveEffect,
	PeerReview,
	PeerReviewID,
	PeerReviewStatus,
	CreateReviewRequest,
	SubmitReviewRequest,
	Trajectory,
	ChatContextRef,
	ChatHistoryMessage,
	ChatMode,
	ChatResponse,
	DMChatSession,
	BoardStatus,
	TokenStats,
	SettingsResponse,
	HealthResponse
} from '$types';

// =============================================================================
// CONFIGURATION
// =============================================================================

const DEFAULT_API_URL = 'http://localhost:8081';

let apiUrl = DEFAULT_API_URL;

export function setApiUrl(url: string): void {
	apiUrl = url;
}

// =============================================================================
// FETCH HELPERS
// =============================================================================

export class ApiError extends Error {
	constructor(
		public readonly status: number,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

async function fetchJson<T>(path: string, options?: RequestInit): Promise<T> {
	const url = `${apiUrl}${path}`;
	const hasBody = options?.body !== undefined;
	const response = await fetch(url, {
		...options,
		headers: {
			...(hasBody ? { 'Content-Type': 'application/json' } : {}),
			...options?.headers
		}
	});

	if (!response.ok) {
		const errorText = await response.text();
		throw new ApiError(response.status, `API Error ${response.status}: ${errorText}`);
	}

	return response.json();
}

async function postJson<T>(path: string, body: unknown): Promise<T> {
	return fetchJson<T>(path, {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

async function postVoid(path: string, body: unknown): Promise<void> {
	const url = `${apiUrl}${path}`;
	const response = await fetch(url, {
		method: 'POST',
		body: JSON.stringify(body),
		headers: { 'Content-Type': 'application/json' }
	});
	if (!response.ok) {
		const errorText = await response.text();
		throw new Error(`API Error ${response.status}: ${errorText}`);
	}
}

// =============================================================================
// WORLD STATE (fallback — SSE replay is the primary hydration path)
// =============================================================================

export async function getWorldState(): Promise<WorldState> {
	return fetchJson<WorldState>('/game/world');
}

// =============================================================================
// QUESTS
// =============================================================================

export async function getQuest(id: QuestID): Promise<Quest> {
	return fetchJson<Quest>(`/game/quests/${id}`);
}

export async function createQuest(objective: string, hints?: QuestHints): Promise<Quest> {
	return postJson<Quest>('/game/quests', { objective, hints });
}

// =============================================================================
// AGENTS
// =============================================================================

export async function getAgent(id: AgentID): Promise<Agent> {
	return fetchJson<Agent>(`/game/agents/${id}`);
}

export async function recruitAgent(config: AgentConfig): Promise<Agent> {
	return postJson<Agent>('/game/agents', config);
}

export async function retireAgent(id: AgentID, reason: string): Promise<void> {
	await postVoid(`/game/agents/${id}/retire`, { reason });
}

// =============================================================================
// BATTLES
// =============================================================================

export async function getBattle(id: BattleID): Promise<BossBattle> {
	return fetchJson<BossBattle>(`/game/battles/${id}`);
}

// =============================================================================
// TRAJECTORIES
// =============================================================================

export async function getTrajectory(id: string, detail?: 'full'): Promise<Trajectory> {
	const params = detail ? `?detail=${detail}` : '';
	return fetchJson<Trajectory>(`/game/trajectories/${id}${params}`);
}

// =============================================================================
// DUNGEON MASTER
// =============================================================================

export async function getDMSession(sessionId: string): Promise<DMChatSession | null> {
	try {
		return await fetchJson<DMChatSession>(`/game/dm/sessions/${sessionId}`);
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			return null;
		}
		throw e;
	}
}

export async function sendDMChat(
	message: string,
	mode?: ChatMode,
	context?: ChatContextRef[],
	history?: ChatHistoryMessage[],
	sessionId?: string
): Promise<ChatResponse> {
	return postJson<ChatResponse>('/game/dm/chat', {
		message,
		mode,
		context,
		history,
		session_id: sessionId
	});
}

export async function postQuestChain(chain: {
	quests: Array<{
		title: string;
		description?: string;
		difficulty?: number;
		skills?: string[];
		acceptance?: string[];
		depends_on?: number[];
	}>;
}): Promise<Quest[]> {
	return postJson<Quest[]>('/game/quests/chain', chain);
}

export async function intervene(questId: QuestID, intervention: Intervention): Promise<void> {
	await postVoid(`/game/dm/intervene/${questId}`, intervention);
}

// =============================================================================
// STORE
// =============================================================================

export async function getStoreItems(agentId: AgentID): Promise<StoreItem[]> {
	const params = new URLSearchParams({ agent_id: String(agentId) });
	return fetchJson<StoreItem[]>(`/game/store?${params}`);
}

export async function getStoreItem(itemId: string): Promise<StoreItem> {
	return fetchJson<StoreItem>(`/game/store/${itemId}`);
}

export async function getInventory(agentId: AgentID): Promise<AgentInventory> {
	return fetchJson<AgentInventory>(`/game/agents/${agentId}/inventory`);
}

export async function purchase(request: PurchaseRequest): Promise<PurchaseResponse> {
	return postJson<PurchaseResponse>('/game/store/purchase', request);
}

export async function useConsumable(request: UseConsumableRequest): Promise<UseConsumableResponse> {
	return postJson<UseConsumableResponse>(`/game/agents/${request.agent_id}/inventory/use`, {
		consumable_id: request.consumable_id,
		quest_id: request.quest_id
	});
}

export async function getActiveEffects(agentId: AgentID): Promise<ActiveEffect[]> {
	return fetchJson<ActiveEffect[]>(`/game/agents/${agentId}/effects`);
}

// =============================================================================
// PEER REVIEWS
// =============================================================================

export async function createReview(request: CreateReviewRequest): Promise<PeerReview> {
	return postJson<PeerReview>('/game/reviews', request);
}

export async function submitReview(
	reviewId: PeerReviewID,
	submission: SubmitReviewRequest
): Promise<PeerReview> {
	return postJson<PeerReview>(`/game/reviews/${reviewId}/submit`, submission);
}

export async function getReview(id: PeerReviewID): Promise<PeerReview> {
	return fetchJson<PeerReview>(`/game/reviews/${id}`);
}

export async function listReviews(
	status?: PeerReviewStatus,
	questId?: string
): Promise<PeerReview[]> {
	const params = new URLSearchParams();
	if (status) params.set('status', status);
	if (questId) params.set('quest_id', questId);
	const qs = params.toString();
	return fetchJson<PeerReview[]>(`/game/reviews${qs ? `?${qs}` : ''}`);
}

export async function getAgentReviews(agentId: AgentID): Promise<PeerReview[]> {
	return fetchJson<PeerReview[]>(`/game/agents/${agentId}/reviews`);
}

// =============================================================================
// BOARD CONTROL (PLAY/PAUSE)
// =============================================================================

export async function getBoardStatus(): Promise<BoardStatus> {
	return fetchJson<BoardStatus>('/game/board/status');
}

export async function pauseBoard(actor?: string): Promise<BoardStatus> {
	return postJson<BoardStatus>('/game/board/pause', actor ? { actor } : {});
}

export async function resumeBoard(): Promise<BoardStatus> {
	return postJson<BoardStatus>('/game/board/resume', {});
}

// =============================================================================
// TOKEN TRACKING
// =============================================================================

export async function getTokenStats(): Promise<TokenStats> {
	return fetchJson<TokenStats>('/game/board/tokens');
}

export async function setTokenBudget(limit: number): Promise<TokenStats> {
	return postJson<TokenStats>('/game/board/tokens/budget', { global_hourly_limit: limit });
}

// =============================================================================
// WORKSPACE (read-only file browser)
// =============================================================================

export interface WorkspaceEntry {
	name: string;
	path: string;
	is_dir: boolean;
	size: number;
	modified: string;
	children?: WorkspaceEntry[];
}

export async function getWorkspaceTree(): Promise<WorkspaceEntry[]> {
	return fetchJson<WorkspaceEntry[]>('/game/workspace');
}

export async function getWorkspaceFile(path: string): Promise<string> {
	const url = `${apiUrl}/game/workspace/file?path=${encodeURIComponent(path)}`;
	const res = await fetch(url);
	if (!res.ok) {
		const errorText = await res.text();
		throw new ApiError(res.status, `API Error ${res.status}: ${errorText}`);
	}
	return res.text();
}

// =============================================================================
// MODEL REGISTRY
// =============================================================================

export interface ModelEndpointSummary {
	name: string;
	provider: string;
	model: string;
	max_tokens: number;
	supports_tools: boolean;
	reasoning_effort?: string;
}

export interface ModelRegistrySummary {
	endpoints: ModelEndpointSummary[];
	capabilities: string[];
}

export interface ModelResolveResponse {
	capability: string;
	endpoint_name: string;
	model?: string;
	provider?: string;
	fallback_chain?: string[];
}

export async function getModelRegistry(): Promise<ModelRegistrySummary> {
	return fetchJson<ModelRegistrySummary>('/game/models');
}

export async function resolveCapability(capability: string): Promise<ModelResolveResponse> {
	return fetchJson<ModelResolveResponse>(`/game/models?resolve=${encodeURIComponent(capability)}`);
}

// =============================================================================
// SETTINGS
// =============================================================================

export async function getSettings(): Promise<SettingsResponse> {
	return fetchJson<SettingsResponse>('/game/settings');
}

export async function getSettingsHealth(): Promise<HealthResponse> {
	return fetchJson<HealthResponse>('/game/settings/health');
}

export async function updateSettings(body: {
	websocket_input?: { enabled?: boolean; url?: string };
	token_budget?: { global_hourly_limit: number };
	model_registry?: {
		endpoints?: Record<string, EndpointUpdate>;
		capabilities?: Record<string, CapabilityUpdate>;
		defaults?: { model?: string; capability?: string };
	};
}): Promise<SettingsResponse> {
	return postJson<SettingsResponse>('/game/settings', body);
}

// =============================================================================
// HEALTH (system endpoint — no /game/ prefix)
// =============================================================================

export async function healthCheck(): Promise<{ status: string }> {
	return fetchJson<{ status: string }>('/health');
}

// =============================================================================
// SETTINGS UPDATE TYPES
// =============================================================================

export interface EndpointUpdate {
	provider: string;
	model: string;
	url?: string;
	max_tokens: number;
	supports_tools: boolean;
	tool_format?: string;
	api_key_env?: string;
	stream?: boolean;
	reasoning_effort?: string;
	input_price_per_1m_tokens?: number;
	output_price_per_1m_tokens?: number;
	remove?: boolean;
}

export interface CapabilityUpdate {
	description?: string;
	preferred?: string[];
	fallback?: string[];
	requires_tools?: boolean;
	remove?: boolean;
}

// =============================================================================
// EXPORT SERVICE
// =============================================================================

export const api = {
	setApiUrl,
	getWorldState,
	getQuest,
	createQuest,
	getAgent,
	recruitAgent,
	retireAgent,
	getBattle,
	getTrajectory,
	getDMSession,
	sendDMChat,
	postQuestChain,
	intervene,
	getStoreItems,
	getStoreItem,
	getInventory,
	purchase,
	useConsumable,
	getActiveEffects,
	createReview,
	submitReview,
	getReview,
	listReviews,
	getAgentReviews,
	getBoardStatus,
	pauseBoard,
	resumeBoard,
	getTokenStats,
	setTokenBudget,
	getWorkspaceTree,
	getWorkspaceFile,
	getModelRegistry,
	resolveCapability,
	getSettings,
	getSettingsHealth,
	updateSettings,
	healthCheck
};
