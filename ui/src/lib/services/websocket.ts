/**
 * WebSocket Service - Real-time event streaming from backend
 *
 * Handles connection lifecycle, reconnection, and event dispatching.
 */

import type { GameEvent, Agent, Quest, BossBattle, AgentID, QuestID, BattleID } from '$types';
import { worldStore } from '$stores/worldStore.svelte';

// =============================================================================
// MESSAGE TYPES
// =============================================================================

interface WebSocketMessage {
	type: string;
	data?: unknown;
	entity_type?: string;
}

interface WorldStateData {
	agents?: Agent[];
	quests?: Quest[];
	parties?: unknown[];
	guilds?: unknown[];
	battles?: BossBattle[];
	stats?: unknown;
}

// =============================================================================
// TYPE GUARDS
// =============================================================================

function isWebSocketMessage(value: unknown): value is WebSocketMessage {
	return (
		typeof value === 'object' &&
		value !== null &&
		'type' in value &&
		typeof (value as WebSocketMessage).type === 'string'
	);
}

function isWorldStateData(value: unknown): value is WorldStateData {
	return typeof value === 'object' && value !== null;
}

function hasProperty<T extends string>(obj: unknown, prop: T): obj is Record<T, unknown> {
	return typeof obj === 'object' && obj !== null && prop in obj;
}

function isAgent(value: unknown): value is Agent {
	return (
		hasProperty(value, 'id') &&
		hasProperty(value, 'name') &&
		hasProperty(value, 'level') &&
		hasProperty(value, 'tier') &&
		typeof (value as Agent).id === 'string' &&
		typeof (value as Agent).name === 'string' &&
		typeof (value as Agent).level === 'number'
	);
}

function isQuest(value: unknown): value is Quest {
	return (
		hasProperty(value, 'id') &&
		hasProperty(value, 'title') &&
		hasProperty(value, 'status') &&
		hasProperty(value, 'difficulty') &&
		typeof (value as Quest).id === 'string' &&
		typeof (value as Quest).title === 'string'
	);
}

function isBossBattle(value: unknown): value is BossBattle {
	return (
		hasProperty(value, 'id') &&
		hasProperty(value, 'quest_id') &&
		hasProperty(value, 'agent_id') &&
		hasProperty(value, 'status') &&
		typeof (value as BossBattle).id === 'string'
	);
}

// =============================================================================
// CONFIGURATION
// =============================================================================

const DEFAULT_WS_URL = 'ws://localhost:8080/events';
const RECONNECT_DELAY_MS = 2000;
const MAX_RECONNECT_ATTEMPTS = 10;

// =============================================================================
// STATE
// =============================================================================

let ws: WebSocket | null = null;
let reconnectAttempts = 0;
let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
let isIntentionallyClosed = false;

// =============================================================================
// CONNECTION MANAGEMENT
// =============================================================================

/**
 * Connect to the WebSocket server
 */
export function connect(url: string = DEFAULT_WS_URL): void {
	if (ws?.readyState === WebSocket.OPEN) {
		console.log('[WS] Already connected');
		return;
	}

	isIntentionallyClosed = false;
	reconnectAttempts = 0;

	createConnection(url);
}

/**
 * Disconnect from the WebSocket server
 */
export function disconnect(): void {
	isIntentionallyClosed = true;

	if (reconnectTimeout) {
		clearTimeout(reconnectTimeout);
		reconnectTimeout = null;
	}

	if (ws) {
		ws.close();
		ws = null;
	}

	worldStore.setConnected(false);
}

/**
 * Create a new WebSocket connection
 */
function createConnection(url: string): void {
	try {
		ws = new WebSocket(url);

		ws.onopen = () => {
			console.log('[WS] Connected');
			reconnectAttempts = 0;
			worldStore.setConnected(true);
			worldStore.setError(null);
		};

		ws.onclose = (event) => {
			console.log('[WS] Disconnected:', event.code, event.reason);
			worldStore.setConnected(false);

			if (!isIntentionallyClosed && reconnectAttempts < MAX_RECONNECT_ATTEMPTS) {
				scheduleReconnect(url);
			}
		};

		ws.onerror = (event) => {
			console.error('[WS] Error:', event);
			worldStore.setError('WebSocket connection error');
		};

		ws.onmessage = (event) => {
			handleMessage(event.data);
		};
	} catch (err) {
		console.error('[WS] Failed to create connection:', err);
		worldStore.setError('Failed to connect to server');
	}
}

/**
 * Schedule a reconnection attempt
 */
function scheduleReconnect(url: string): void {
	reconnectAttempts++;
	const delay = RECONNECT_DELAY_MS * Math.pow(1.5, reconnectAttempts - 1);

	console.log(`[WS] Reconnecting in ${delay}ms (attempt ${reconnectAttempts}/${MAX_RECONNECT_ATTEMPTS})`);

	reconnectTimeout = setTimeout(() => {
		createConnection(url);
	}, delay);
}

// =============================================================================
// MESSAGE HANDLING
// =============================================================================

/**
 * Handle incoming WebSocket messages
 */
function handleMessage(data: string): void {
	try {
		const parsed = JSON.parse(data);

		// Validate message structure
		if (!isWebSocketMessage(parsed)) {
			console.error('[WS] Invalid message structure:', parsed);
			return;
		}

		// Handle different message types
		switch (parsed.type) {
			case 'world_state':
				if (isWorldStateData(parsed.data)) {
					handleWorldStateMessage(parsed.data);
				} else {
					console.error('[WS] Invalid world_state data');
				}
				break;

			case 'event':
				handleEventMessage(parsed.data as GameEvent);
				break;

			case 'entity_update':
				if (typeof parsed.entity_type === 'string') {
					handleEntityUpdateMessage(parsed.entity_type, parsed.data);
				} else {
					console.error('[WS] Missing entity_type in entity_update');
				}
				break;

			default:
				console.warn('[WS] Unknown message type:', parsed.type);
		}
	} catch (err) {
		console.error('[WS] Failed to parse message:', err, data);
	}
}

/**
 * Handle world state snapshot message
 */
function handleWorldStateMessage(data: WorldStateData): void {
	// Validate and filter agents
	const agents = (data.agents ?? []).filter(isAgent);
	const quests = (data.quests ?? []).filter(isQuest);
	const battles = (data.battles ?? []).filter(isBossBattle);

	if (data.agents && agents.length !== data.agents.length) {
		console.warn('[WS] Some agents failed validation');
	}
	if (data.quests && quests.length !== data.quests.length) {
		console.warn('[WS] Some quests failed validation');
	}
	if (data.battles && battles.length !== data.battles.length) {
		console.warn('[WS] Some battles failed validation');
	}

	worldStore.setWorldState({
		agents,
		quests,
		parties: (data.parties ?? []) as never[],
		guilds: (data.guilds ?? []) as never[],
		battles,
		stats: (data.stats ?? {}) as never
	});
}

/**
 * Handle individual game event message
 */
function handleEventMessage(event: GameEvent): void {
	worldStore.addEvent(event);

	// Apply event to state based on type
	switch (event.type) {
		case 'quest.posted':
		case 'quest.claimed':
		case 'quest.started':
		case 'quest.completed':
		case 'quest.failed':
		case 'quest.escalated':
			if (hasProperty(event.data, 'quest') && isQuest(event.data.quest)) {
				worldStore.updateQuest(event.data.quest);
			}
			break;

		case 'agent.recruited':
		case 'agent.level_up':
		case 'agent.level_down':
		case 'agent.death':
		case 'agent.revived':
			if (hasProperty(event.data, 'agent') && isAgent(event.data.agent)) {
				worldStore.updateAgent(event.data.agent);
			}
			break;

		case 'battle.started':
		case 'battle.victory':
		case 'battle.defeat':
			if (hasProperty(event.data, 'battle') && isBossBattle(event.data.battle)) {
				worldStore.updateBattle(event.data.battle);
			}
			break;

		default:
			// Other events just get added to the event stream
			break;
	}
}

/**
 * Handle entity update message (direct entity push)
 */
function handleEntityUpdateMessage(entityType: string, data: unknown): void {
	switch (entityType) {
		case 'agent':
			if (isAgent(data)) {
				worldStore.updateAgent(data);
			} else {
				console.error('[WS] Invalid agent data in entity_update');
			}
			break;
		case 'quest':
			if (isQuest(data)) {
				worldStore.updateQuest(data);
			} else {
				console.error('[WS] Invalid quest data in entity_update');
			}
			break;
		case 'battle':
			if (isBossBattle(data)) {
				worldStore.updateBattle(data);
			} else {
				console.error('[WS] Invalid battle data in entity_update');
			}
			break;
		default:
			console.warn('[WS] Unknown entity type:', entityType);
	}
}

// =============================================================================
// SENDING MESSAGES
// =============================================================================

/**
 * Send a message through the WebSocket
 */
export function send(type: string, data: unknown): boolean {
	if (!ws || ws.readyState !== WebSocket.OPEN) {
		console.error('[WS] Cannot send: not connected');
		return false;
	}

	try {
		ws.send(JSON.stringify({ type, data }));
		return true;
	} catch (err) {
		console.error('[WS] Failed to send message:', err);
		return false;
	}
}

/**
 * Subscribe to specific event types
 */
export function subscribe(eventTypes: string[]): void {
	send('subscribe', { types: eventTypes });
}

/**
 * Unsubscribe from specific event types
 */
export function unsubscribe(eventTypes: string[]): void {
	send('unsubscribe', { types: eventTypes });
}

// =============================================================================
// EXPORT SERVICE
// =============================================================================

export const websocketService = {
	connect,
	disconnect,
	send,
	subscribe,
	unsubscribe,
	get isConnected() {
		return ws?.readyState === WebSocket.OPEN;
	}
};
