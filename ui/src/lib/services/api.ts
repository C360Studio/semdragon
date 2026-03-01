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
	ActiveEffect
} from '$types';

// =============================================================================
// CONFIGURATION
// =============================================================================

const DEFAULT_API_URL = 'http://localhost:8080';

let apiUrl = DEFAULT_API_URL;

export function setApiUrl(url: string): void {
	apiUrl = url;
}

// =============================================================================
// FETCH HELPERS
// =============================================================================

async function fetchJson<T>(path: string, options?: RequestInit): Promise<T> {
	const url = `${apiUrl}${path}`;
	const response = await fetch(url, {
		...options,
		headers: {
			'Content-Type': 'application/json',
			...options?.headers
		}
	});

	if (!response.ok) {
		const errorText = await response.text();
		throw new Error(`API Error ${response.status}: ${errorText}`);
	}

	return response.json();
}

async function postJson<T>(path: string, body: unknown): Promise<T> {
	return fetchJson<T>(path, {
		method: 'POST',
		body: JSON.stringify(body)
	});
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
	await postJson(`/game/agents/${id}/retire`, { reason });
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

export interface TrajectoryEvent {
	timestamp: number;
	type: string;
	data: unknown;
}

export async function getTrajectory(id: string): Promise<TrajectoryEvent[]> {
	return fetchJson<TrajectoryEvent[]>(`/game/trajectories/${id}`);
}

// =============================================================================
// DUNGEON MASTER
// =============================================================================

export interface ChatMessage {
	role: 'user' | 'dm';
	content: string;
	timestamp: number;
}

export interface ChatResponse {
	message: string;
	actions?: unknown[];
}

export async function sendDMChat(message: string): Promise<ChatResponse> {
	return postJson<ChatResponse>('/game/dm/chat', { message });
}

export async function intervene(questId: QuestID, intervention: Intervention): Promise<void> {
	await postJson(`/game/dm/intervene/${questId}`, intervention);
}

// =============================================================================
// STORE
// =============================================================================

export async function getStoreItems(agentId: AgentID): Promise<StoreItem[]> {
	return fetchJson<StoreItem[]>(`/game/store?agent_id=${agentId}`);
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
// HEALTH (system endpoint — no /game/ prefix)
// =============================================================================

export async function healthCheck(): Promise<{ status: string }> {
	return fetchJson<{ status: string }>('/health');
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
	sendDMChat,
	intervene,
	getStoreItems,
	getStoreItem,
	getInventory,
	purchase,
	useConsumable,
	getActiveEffects,
	healthCheck
};
