/**
 * SSE Service - Server-Sent Events client for real-time entity updates
 *
 * Connects to the game service SSE endpoint for live entity state streaming.
 */

import type {
	KVChangeEvent,
	Quest,
	Agent,
	BossBattle,
	Party,
	Guild,
	EntityType,
	GameEventType,
	GameEvent
} from '$types';
import { questId, agentId, battleId, partyId, guildId } from '$types';
import { worldStore } from '$stores/worldStore.svelte';
import { transformEntity, isGraphEntity } from './entity-transformer';

// =============================================================================
// ENTITY KEY PARSING
// =============================================================================

/**
 * Extract entity type from a 6-part entity ID key.
 * Format: org.platform.domain.system.TYPE.instance
 */
export function entityTypeFromKey(key: string): EntityType | null {
	const parts = key.split('.');
	if (parts.length !== 6) return null;
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
// EVENT TYPE DERIVATION
// =============================================================================

/**
 * Map backend message_type.category to frontend GameEventType.
 * Backend emits short categories like "quest.posted"; frontend expects
 * "quest.lifecycle.posted". Unmapped categories are silently skipped.
 */
const CATEGORY_TO_EVENT_TYPE: Record<string, GameEventType> = {
	'quest.posted': 'quest.lifecycle.posted',
	'quest.claimed': 'quest.lifecycle.claimed',
	'quest.started': 'quest.lifecycle.started',
	'quest.submitted': 'quest.lifecycle.submitted',
	'quest.completed': 'quest.lifecycle.completed',
	'quest.failed': 'quest.lifecycle.failed',
	'quest.escalated': 'quest.lifecycle.escalated',
	'quest.abandoned': 'quest.lifecycle.abandoned',
	'quest.in_review': 'quest.lifecycle.submitted',
	'agent.progression.xp': 'agent.progression.xp',
	'agent.status.on_quest': 'agent.progression.xp',
	'agent.status.idle': 'agent.progression.xp',
	'agent.status.in_battle': 'agent.progression.xp',
	'agent.released': 'agent.progression.xp',
	'agent.inventory.purchased': 'agent.inventory.updated',
	'battle.started': 'battle.review.started',
	'guild.created': 'guild.membership.joined',
	'guild.member.joined': 'guild.membership.joined',
	'guild.member.promoted': 'guild.membership.promoted',
	'guild.disbanded': 'guild.membership.joined',
	'party.formed': 'party.formation.created',
	'party.joined': 'party.formation.created',
	'party.disbanded': 'party.formation.disbanded',
	'store.item.listed': 'store.item.purchased'
};

/**
 * Derive a GameEventType from the raw SSE value's message_type.category.
 * Returns null for unmapped or missing categories (event is silently skipped).
 */
function deriveEventType(rawValue: unknown): GameEventType | null {
	if (typeof rawValue !== 'object' || rawValue === null || !('message_type' in rawValue)) {
		return null;
	}
	const mt = (rawValue as { message_type?: { category?: string } }).message_type;
	if (!mt?.category) return null;
	return CATEGORY_TO_EVENT_TYPE[mt.category] ?? null;
}

// =============================================================================
// SSE SERVICE
// =============================================================================

export function createSSEService() {
	let source: EventSource | null = null;
	let synced = false;

	function connect(baseUrl: string) {
		if (source !== null) {
			source.close();
			source = null;
		}

		const url = `${baseUrl}/game/events`;
		source = new EventSource(url);

		source.addEventListener('connected', () => {
			worldStore.setConnected(true);
			synced = false;
		});

		source.addEventListener('kv_change', (e: MessageEvent) => {
			let event: KVChangeEvent;
			try {
				event = JSON.parse(e.data) as KVChangeEvent;
			} catch {
				console.error('[SSE] Failed to parse kv_change payload:', e.data);
				return;
			}

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
			synced = false;
			worldStore.setSynced(false);
			// EventSource auto-reconnects after server-sent retry interval
		};
	}

	function handleUpsert(key: string, value: unknown) {
		if (value === null || value === undefined || typeof value !== 'object') {
			console.error('[SSE] Received non-object value for upsert, key:', key);
			return;
		}
		const entityType = entityTypeFromKey(key);
		if (!entityType) return;

		// Transform graph triple format to flat entity if needed
		const entity = isGraphEntity(value) ? transformEntity(entityType, key, value) : value;
		if (!entity) return;

		switch (entityType) {
			case 'quest':
				worldStore.upsertQuest(entity as Quest);
				break;
			case 'agent':
				worldStore.upsertAgent(entity as Agent);
				break;
			case 'battle':
				worldStore.upsertBattle(entity as BossBattle);
				break;
			case 'party':
				worldStore.upsertParty(entity as Party);
				break;
			case 'guild':
				worldStore.upsertGuild(entity as Guild);
				break;
		}

		// Generate activity event after upsert (skip during initial sync to avoid flood)
		if (!synced) return;
		const eventType = deriveEventType(value);
		if (!eventType) return;

		const gameEvent: GameEvent = {
			type: eventType,
			timestamp: Date.now(),
			session_id: '',
			span_id: '',
			data: {}
		};
		if (entityType === 'quest') gameEvent.quest_id = (entity as Quest).id;
		if (entityType === 'agent') gameEvent.agent_id = (entity as Agent).id;
		if (entityType === 'battle') gameEvent.battle_id = (entity as BossBattle).id;
		if (entityType === 'party') gameEvent.party_id = (entity as Party).id;
		if (entityType === 'guild') gameEvent.guild_id = (entity as Guild).id;

		worldStore.addEvent(gameEvent);
	}

	function handleDelete(key: string) {
		const entityType = entityTypeFromKey(key);
		if (!entityType) return;

		// Use the full 6-part key as the entity ID (matches upsert behavior)
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
