/**
 * Unit tests for SSE service
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import type { KVChangeEvent, Agent, Quest, BossBattle, Party, Guild } from '$types';
import { agentId, questId, battleId, partyId, guildId } from '$types';

// Mock worldStore before importing sse module
vi.mock('$stores/worldStore.svelte', () => ({
	worldStore: {
		setConnected: vi.fn(),
		setSynced: vi.fn(),
		setLoading: vi.fn(),
		upsertQuest: vi.fn(),
		upsertAgent: vi.fn(),
		upsertBattle: vi.fn(),
		upsertParty: vi.fn(),
		upsertGuild: vi.fn(),
		removeQuest: vi.fn(),
		removeAgent: vi.fn(),
		removeBattle: vi.fn(),
		removeParty: vi.fn(),
		removeGuild: vi.fn(),
		addEvent: vi.fn()
	}
}));

// Mock EventSource
class MockEventSource {
	static instances: MockEventSource[] = [];
	url: string;
	readyState = 0;
	listeners = new Map<string, ((e: MessageEvent | Event) => void)[]>();
	onerror: ((e: Event) => void) | null = null;

	constructor(url: string) {
		this.url = url;
		MockEventSource.instances.push(this);
	}

	addEventListener(type: string, listener: (e: MessageEvent | Event) => void) {
		const existing = this.listeners.get(type) || [];
		existing.push(listener);
		this.listeners.set(type, existing);
	}

	close() {
		this.readyState = 2;
	}

	// Test helpers
	emit(type: string, data?: string) {
		const listeners = this.listeners.get(type) || [];
		const event = data !== undefined
			? new MessageEvent(type, { data })
			: new Event(type);
		for (const listener of listeners) {
			listener(event);
		}
	}

	triggerError() {
		if (this.onerror) {
			this.onerror(new Event('error'));
		}
	}
}

// Install mock
const originalEventSource = globalThis.EventSource;
beforeEach(() => {
	MockEventSource.instances = [];
	(globalThis as unknown as Record<string, unknown>).EventSource = MockEventSource;
});
afterEach(() => {
	(globalThis as unknown as Record<string, unknown>).EventSource = originalEventSource;
});

describe('SSE Service', () => {
	let worldStore: {
		setConnected: ReturnType<typeof vi.fn>;
		setSynced: ReturnType<typeof vi.fn>;
		setLoading: ReturnType<typeof vi.fn>;
		upsertQuest: ReturnType<typeof vi.fn>;
		upsertAgent: ReturnType<typeof vi.fn>;
		upsertBattle: ReturnType<typeof vi.fn>;
		upsertParty: ReturnType<typeof vi.fn>;
		upsertGuild: ReturnType<typeof vi.fn>;
		removeQuest: ReturnType<typeof vi.fn>;
		removeAgent: ReturnType<typeof vi.fn>;
		removeBattle: ReturnType<typeof vi.fn>;
		removeParty: ReturnType<typeof vi.fn>;
		removeGuild: ReturnType<typeof vi.fn>;
		addEvent: ReturnType<typeof vi.fn>;
	};

	beforeEach(async () => {
		vi.clearAllMocks();
		const mod = await import('$stores/worldStore.svelte');
		worldStore = mod.worldStore as unknown as typeof worldStore;
	});

	async function getService() {
		// Re-import to get fresh module with mocks applied
		const { createSSEService } = await import('./sse');
		return createSSEService();
	}

	function getLastSource(): MockEventSource {
		return MockEventSource.instances[MockEventSource.instances.length - 1];
	}

	describe('connect', () => {
		it('creates EventSource with correct URL', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			expect(source.url).toBe('http://localhost:8080/game/events');
		});

		it('sets connected state on connected event', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			source.emit('connected');

			expect(worldStore.setConnected).toHaveBeenCalledWith(true);
		});

		it('resets synced on reconnection', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			source.emit('connected');

			expect(service.synced).toBe(false);
		});

		it('closes existing connection before reconnecting', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');
			const firstSource = getLastSource();

			service.connect('http://localhost:8080');
			expect(firstSource.readyState).toBe(2); // CLOSED
		});
	});

	describe('kv_change events', () => {
		it('upserts quest on create/update', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const questData = { id: questId('quest-1'), title: 'Test Quest' };
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.quest.quest-1',
				operation: 'create',
				value: questData,
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).toHaveBeenCalledWith(questData);
		});

		it('upserts agent on update', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const agentData = { id: agentId('agent-1'), name: 'Test Agent' };
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.agent.agent-1',
				operation: 'update',
				value: agentData,
				revision: 2,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertAgent).toHaveBeenCalledWith(agentData);
		});

		it('upserts battle', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const battleData = { id: battleId('battle-1'), status: 'active' };
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.battle.battle-1',
				operation: 'create',
				value: battleData,
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertBattle).toHaveBeenCalledWith(battleData);
		});

		it('upserts party', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const partyData = { id: partyId('party-1'), name: 'Alpha Squad' };
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.party.party-1',
				operation: 'create',
				value: partyData,
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertParty).toHaveBeenCalledWith(partyData);
		});

		it('upserts guild', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const guildData = { id: guildId('guild-1'), name: 'Data Wranglers' };
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.guild.guild-1',
				operation: 'create',
				value: guildData,
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertGuild).toHaveBeenCalledWith(guildData);
		});

		it('ignores unknown entity types', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.unknown.foo-1',
				operation: 'create',
				value: {},
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).not.toHaveBeenCalled();
			expect(worldStore.upsertAgent).not.toHaveBeenCalled();
			expect(worldStore.upsertBattle).not.toHaveBeenCalled();
			expect(worldStore.upsertParty).not.toHaveBeenCalled();
			expect(worldStore.upsertGuild).not.toHaveBeenCalled();
		});

		it('ignores keys with fewer than 6 parts', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'short.key',
				operation: 'create',
				value: {},
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).not.toHaveBeenCalled();
			expect(worldStore.upsertAgent).not.toHaveBeenCalled();
		});

		it('ignores keys with more than 6 parts', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.quest.id.extra',
				operation: 'create',
				value: { id: 'test' },
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).not.toHaveBeenCalled();
		});

		it('handles malformed JSON in kv_change without throwing', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			expect(() => source.emit('kv_change', '{ invalid json')).not.toThrow();

			expect(worldStore.upsertQuest).not.toHaveBeenCalled();
			expect(worldStore.upsertAgent).not.toHaveBeenCalled();
		});

		it('ignores upsert with null value', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.quest.quest-1',
				operation: 'create',
				value: null as unknown as undefined,
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).not.toHaveBeenCalled();
		});

		it('ignores upsert with undefined value', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.quest.quest-1',
				operation: 'create',
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).not.toHaveBeenCalled();
		});
	});

	describe('delete operations', () => {
		it('removes quest on delete', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.quest.quest-1',
				operation: 'delete',
				revision: 3,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.removeQuest).toHaveBeenCalledWith(
				questId('c360.prod.game.board1.quest.quest-1')
			);
		});

		it('removes agent on delete', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.agent.agent-1',
				operation: 'delete',
				revision: 3,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.removeAgent).toHaveBeenCalledWith(
				agentId('c360.prod.game.board1.agent.agent-1')
			);
		});

		it('removes battle on delete', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.battle.battle-1',
				operation: 'delete',
				revision: 3,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.removeBattle).toHaveBeenCalledWith(
				battleId('c360.prod.game.board1.battle.battle-1')
			);
		});

		it('removes party on delete', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.party.party-1',
				operation: 'delete',
				revision: 3,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.removeParty).toHaveBeenCalledWith(
				partyId('c360.prod.game.board1.party.party-1')
			);
		});

		it('removes guild on delete', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.guild.guild-1',
				operation: 'delete',
				revision: 3,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.removeGuild).toHaveBeenCalledWith(
				guildId('c360.prod.game.board1.guild.guild-1')
			);
		});
	});

	describe('initial_sync_complete', () => {
		it('sets synced and loading states', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: '',
				operation: 'initial_sync_complete',
				revision: 0,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.setSynced).toHaveBeenCalledWith(true);
			expect(worldStore.setLoading).toHaveBeenCalledWith(false);
			expect(service.synced).toBe(true);
		});
	});

	describe('activity events', () => {
		it('generates activity event for graph entity after sync', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();

			// Complete initial sync first
			const syncEvent: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: '',
				operation: 'initial_sync_complete',
				revision: 0,
				timestamp: '2024-01-01T00:00:00Z'
			};
			source.emit('kv_change', JSON.stringify(syncEvent));
			expect(service.synced).toBe(true);

			// Send a graph entity (has message_type.category)
			const questGraphEntity = {
				id: 'c360.prod.game.board1.quest.quest-99',
				triples: [
					{ subject: 'c360.prod.game.board1.quest.quest-99', predicate: 'quest.identity.title', object: 'Test Quest', source: 'test', timestamp: '2024-01-01T00:00:00Z', confidence: 1 },
					{ subject: 'c360.prod.game.board1.quest.quest-99', predicate: 'quest.status.state', object: 'posted', source: 'test', timestamp: '2024-01-01T00:00:00Z', confidence: 1 }
				],
				message_type: { domain: 'semdragons', category: 'quest.posted', version: 'v1' },
				version: 1,
				updated_at: '2024-01-01T00:00:00Z'
			};

			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.quest.quest-99',
				operation: 'create',
				value: questGraphEntity,
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).toHaveBeenCalled();
			expect(worldStore.addEvent).toHaveBeenCalledWith(
				expect.objectContaining({
					type: 'quest.lifecycle.posted',
					quest_id: expect.any(String)
				})
			);
		});

		it('skips activity events during initial sync', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			// Do NOT complete initial sync

			const questGraphEntity = {
				id: 'c360.prod.game.board1.quest.quest-99',
				triples: [
					{ subject: 'c360.prod.game.board1.quest.quest-99', predicate: 'quest.identity.title', object: 'Test Quest', source: 'test', timestamp: '2024-01-01T00:00:00Z', confidence: 1 },
					{ subject: 'c360.prod.game.board1.quest.quest-99', predicate: 'quest.status.state', object: 'posted', source: 'test', timestamp: '2024-01-01T00:00:00Z', confidence: 1 }
				],
				message_type: { domain: 'semdragons', category: 'quest.posted', version: 'v1' },
				version: 1,
				updated_at: '2024-01-01T00:00:00Z'
			};

			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.quest.quest-99',
				operation: 'create',
				value: questGraphEntity,
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).toHaveBeenCalled();
			expect(worldStore.addEvent).not.toHaveBeenCalled();
		});

		it('skips activity events for flat entities without message_type', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();

			// Complete initial sync
			source.emit('kv_change', JSON.stringify({
				bucket: 'ENTITY_STATES', key: '', operation: 'initial_sync_complete',
				revision: 0, timestamp: '2024-01-01T00:00:00Z'
			}));

			// Send flat entity (no message_type)
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.quest.quest-99',
				operation: 'create',
				value: { id: questId('quest-99'), title: 'Flat Quest' },
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(worldStore.upsertQuest).toHaveBeenCalled();
			expect(worldStore.addEvent).not.toHaveBeenCalled();
		});

		it('generates agent activity events', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();

			// Complete sync
			source.emit('kv_change', JSON.stringify({
				bucket: 'ENTITY_STATES', key: '', operation: 'initial_sync_complete',
				revision: 0, timestamp: '2024-01-01T00:00:00Z'
			}));

			const agentGraphEntity = {
				id: 'c360.prod.game.board1.agent.agent-1',
				triples: [
					{ subject: 'c360.prod.game.board1.agent.agent-1', predicate: 'agent.identity.name', object: 'TestBot', source: 'test', timestamp: '2024-01-01T00:00:00Z', confidence: 1 },
					{ subject: 'c360.prod.game.board1.agent.agent-1', predicate: 'agent.status.state', object: 'idle', source: 'test', timestamp: '2024-01-01T00:00:00Z', confidence: 1 }
				],
				message_type: { domain: 'semdragons', category: 'agent.status.idle', version: 'v1' },
				version: 1,
				updated_at: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify({
				bucket: 'ENTITY_STATES',
				key: 'c360.prod.game.board1.agent.agent-1',
				operation: 'update',
				value: agentGraphEntity,
				revision: 2,
				timestamp: '2024-01-01T00:00:00Z'
			}));

			expect(worldStore.upsertAgent).toHaveBeenCalled();
			expect(worldStore.addEvent).toHaveBeenCalledWith(
				expect.objectContaining({
					type: 'agent.progression.xp',
					agent_id: expect.any(String)
				})
			);
		});
	});

	describe('error handling', () => {
		it('sets disconnected on error', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			source.triggerError();

			expect(worldStore.setConnected).toHaveBeenCalledWith(false);
		});

		it('resets synced state on error', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			// Complete initial sync first
			const syncEvent: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: '',
				operation: 'initial_sync_complete',
				revision: 0,
				timestamp: '2024-01-01T00:00:00Z'
			};
			source.emit('kv_change', JSON.stringify(syncEvent));
			expect(service.synced).toBe(true);

			// Error should reset synced
			source.triggerError();
			expect(service.synced).toBe(false);
			expect(worldStore.setSynced).toHaveBeenCalledWith(false);
		});
	});

	describe('disconnect', () => {
		it('closes the EventSource', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			const source = getLastSource();
			service.disconnect();

			expect(source.readyState).toBe(2);
			expect(worldStore.setConnected).toHaveBeenCalledWith(false);
		});

		it('resets synced state', async () => {
			const service = await getService();
			service.connect('http://localhost:8080');

			// Simulate sync completion
			const source = getLastSource();
			const syncEvent: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: '',
				operation: 'initial_sync_complete',
				revision: 0,
				timestamp: '2024-01-01T00:00:00Z'
			};
			source.emit('kv_change', JSON.stringify(syncEvent));
			expect(service.synced).toBe(true);

			service.disconnect();
			expect(service.synced).toBe(false);
		});

		it('handles disconnect when not connected', async () => {
			const service = await getService();
			// Should not throw
			service.disconnect();
		});
	});
});

describe('Entity key parsing', () => {
	it('extracts entity type from 6-part key', async () => {
		// This is tested indirectly through the upsert routing
		const { createSSEService } = await import('./sse');
		const worldStoreMod = await import('$stores/worldStore.svelte');
		const store = worldStoreMod.worldStore as unknown as Record<string, ReturnType<typeof vi.fn>>;

		const service = createSSEService();
		service.connect('http://localhost:8080');

		const source = MockEventSource.instances[MockEventSource.instances.length - 1];

		// Test each entity type
		const entityTypes = ['quest', 'agent', 'battle', 'party', 'guild'] as const;
		const upsertMethods = [
			'upsertQuest', 'upsertAgent', 'upsertBattle', 'upsertParty', 'upsertGuild'
		] as const;

		for (let i = 0; i < entityTypes.length; i++) {
			vi.clearAllMocks();
			const event: KVChangeEvent = {
				bucket: 'ENTITY_STATES',
				key: `org.plat.domain.sys.${entityTypes[i]}.instance-${i}`,
				operation: 'create',
				value: { id: `${entityTypes[i]}-${i}` },
				revision: 1,
				timestamp: '2024-01-01T00:00:00Z'
			};

			source.emit('kv_change', JSON.stringify(event));

			expect(store[upsertMethods[i]]).toHaveBeenCalledWith({ id: `${entityTypes[i]}-${i}` });
		}

		service.disconnect();
	});
});
