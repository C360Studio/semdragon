/**
 * SSE Service - Server-Sent Events client for real-time entity updates
 *
 * Connects to the semstreams message-logger KV watch endpoint.
 * Replaces the WebSocket service with EventSource (auto-reconnect built in).
 */

import type {
	KVChangeEvent,
	Quest,
	Agent,
	BossBattle,
	Party,
	Guild,
	EntityType
} from '$types';
import { questId, agentId, battleId, partyId, guildId } from '$types';
import { worldStore } from '$stores/worldStore.svelte';

// =============================================================================
// ENTITY KEY PARSING
// =============================================================================

/**
 * Extract entity type from a 6-part entity ID key.
 * Format: org.platform.domain.system.TYPE.instance
 */
function entityTypeFromKey(key: string): EntityType | null {
	const parts = key.split('.');
	if (parts.length < 6) return null;
	const type = parts[4];
	switch (type) {
		case 'quest':
		case 'agent':
		case 'battle':
		case 'party':
		case 'guild':
			return type;
		default:
			return null;
	}
}

// =============================================================================
// SSE SERVICE
// =============================================================================

export function createSSEService() {
	let source: EventSource | null = null;
	let synced = false;

	function connect(baseUrl: string) {
		const url = `${baseUrl}/message-logger/kv/ENTITY_STATES/watch?pattern=*`;
		source = new EventSource(url);

		source.addEventListener('connected', () => {
			worldStore.setConnected(true);
			synced = false;
		});

		source.addEventListener('kv_change', (e: MessageEvent) => {
			const event: KVChangeEvent = JSON.parse(e.data);

			if (event.operation === 'initial_sync_complete') {
				synced = true;
				worldStore.setSynced(true);
				worldStore.setLoading(false);
				return;
			}

			if (event.operation === 'delete') {
				handleDelete(event.key);
				return;
			}

			handleUpsert(event.key, event.value);
		});

		source.onerror = () => {
			worldStore.setConnected(false);
			// EventSource auto-reconnects after server-sent retry interval
		};
	}

	function handleUpsert(key: string, value: unknown) {
		const entityType = entityTypeFromKey(key);
		if (!entityType) return;

		switch (entityType) {
			case 'quest':
				worldStore.upsertQuest(value as Quest);
				break;
			case 'agent':
				worldStore.upsertAgent(value as Agent);
				break;
			case 'battle':
				worldStore.upsertBattle(value as BossBattle);
				break;
			case 'party':
				worldStore.upsertParty(value as Party);
				break;
			case 'guild':
				worldStore.upsertGuild(value as Guild);
				break;
		}
	}

	function handleDelete(key: string) {
		const entityType = entityTypeFromKey(key);
		if (!entityType) return;

		switch (entityType) {
			case 'quest':
				worldStore.removeQuest(questId(key));
				break;
			case 'agent':
				worldStore.removeAgent(agentId(key));
				break;
			case 'battle':
				worldStore.removeBattle(battleId(key));
				break;
			case 'party':
				worldStore.removeParty(partyId(key));
				break;
			case 'guild':
				worldStore.removeGuild(guildId(key));
				break;
		}
	}

	function disconnect() {
		source?.close();
		source = null;
		synced = false;
		worldStore.setConnected(false);
	}

	return {
		connect,
		disconnect,
		get synced() {
			return synced;
		}
	};
}

export const sseService = createSSEService();
