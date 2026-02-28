/**
 * API Service - HTTP client for Semdragons backend
 *
 * Provides typed API methods for all backend endpoints.
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
	AgentConfig
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

async function putJson<T>(path: string, body: unknown): Promise<T> {
	return fetchJson<T>(path, {
		method: 'PUT',
		body: JSON.stringify(body)
	});
}

async function deleteRequest(path: string): Promise<void> {
	const url = `${apiUrl}${path}`;
	const response = await fetch(url, { method: 'DELETE' });

	if (!response.ok) {
		const errorText = await response.text();
		throw new Error(`API Error ${response.status}: ${errorText}`);
	}
}

// =============================================================================
// WORLD STATE
// =============================================================================

export async function getWorldState(): Promise<WorldState> {
	return fetchJson<WorldState>('/world');
}

// =============================================================================
// QUESTS
// =============================================================================

export interface QuestFilters {
	status?: string;
	difficulty?: number;
	guild_id?: string;
}

export async function getQuests(filters?: QuestFilters): Promise<Quest[]> {
	const params = new URLSearchParams();
	if (filters?.status) params.set('status', filters.status);
	if (filters?.difficulty !== undefined) params.set('difficulty', String(filters.difficulty));
	if (filters?.guild_id) params.set('guild_id', filters.guild_id);

	const queryString = params.toString();
	const path = queryString ? `/quests?${queryString}` : '/quests';
	return fetchJson<Quest[]>(path);
}

export async function getQuest(id: QuestID): Promise<Quest> {
	return fetchJson<Quest>(`/quests/${id}`);
}

export async function createQuest(objective: string, hints?: QuestHints): Promise<Quest> {
	return postJson<Quest>('/quests', { objective, hints });
}

// =============================================================================
// AGENTS
// =============================================================================

export async function getAgents(): Promise<Agent[]> {
	return fetchJson<Agent[]>('/agents');
}

export async function getAgent(id: AgentID): Promise<Agent> {
	return fetchJson<Agent>(`/agents/${id}`);
}

export async function recruitAgent(config: AgentConfig): Promise<Agent> {
	return postJson<Agent>('/agents', config);
}

export async function retireAgent(id: AgentID, reason: string): Promise<void> {
	await postJson(`/agents/${id}/retire`, { reason });
}

// =============================================================================
// BATTLES
// =============================================================================

export async function getBattles(): Promise<BossBattle[]> {
	return fetchJson<BossBattle[]>('/battles');
}

export async function getBattle(id: BattleID): Promise<BossBattle> {
	return fetchJson<BossBattle>(`/battles/${id}`);
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
	return fetchJson<TrajectoryEvent[]>(`/trajectories/${id}`);
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
	return postJson<ChatResponse>('/dm/chat', { message });
}

export async function intervene(questId: QuestID, intervention: Intervention): Promise<void> {
	await postJson(`/dm/intervene/${questId}`, intervention);
}

// =============================================================================
// HEALTH
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
	getQuests,
	getQuest,
	createQuest,
	getAgents,
	getAgent,
	recruitAgent,
	retireAgent,
	getBattles,
	getBattle,
	getTrajectory,
	sendDMChat,
	intervene,
	healthCheck
};
