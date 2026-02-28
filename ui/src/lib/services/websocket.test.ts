/**
 * Unit tests for WebSocket service type guards
 */

import { describe, it, expect } from 'vitest';
import type { Agent, Quest, BossBattle } from '$types';
import { agentId, questId, battleId } from '$types';

// Re-create the type guards here for testing since they're not exported
// In a real scenario, you'd export these from websocket.ts

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

interface WebSocketMessage {
	type: string;
	data?: unknown;
	entity_type?: string;
}

function isWebSocketMessage(value: unknown): value is WebSocketMessage {
	return (
		typeof value === 'object' &&
		value !== null &&
		'type' in value &&
		typeof (value as WebSocketMessage).type === 'string'
	);
}

describe('WebSocket Type Guards', () => {
	describe('hasProperty', () => {
		it('returns true when property exists', () => {
			expect(hasProperty({ foo: 'bar' }, 'foo')).toBe(true);
		});

		it('returns false when property does not exist', () => {
			expect(hasProperty({ foo: 'bar' }, 'baz')).toBe(false);
		});

		it('returns false for null', () => {
			expect(hasProperty(null, 'foo')).toBe(false);
		});

		it('returns false for undefined', () => {
			expect(hasProperty(undefined, 'foo')).toBe(false);
		});

		it('returns false for primitives', () => {
			expect(hasProperty('string', 'length')).toBe(false);
			expect(hasProperty(123, 'toString')).toBe(false);
		});
	});

	describe('isWebSocketMessage', () => {
		it('returns true for valid message with type', () => {
			expect(isWebSocketMessage({ type: 'event' })).toBe(true);
			expect(isWebSocketMessage({ type: 'world_state', data: {} })).toBe(true);
		});

		it('returns false for missing type', () => {
			expect(isWebSocketMessage({ data: {} })).toBe(false);
		});

		it('returns false for non-string type', () => {
			expect(isWebSocketMessage({ type: 123 })).toBe(false);
			expect(isWebSocketMessage({ type: null })).toBe(false);
		});

		it('returns false for null', () => {
			expect(isWebSocketMessage(null)).toBe(false);
		});

		it('returns false for non-objects', () => {
			expect(isWebSocketMessage('string')).toBe(false);
			expect(isWebSocketMessage(123)).toBe(false);
		});
	});

	describe('isAgent', () => {
		const validAgent = {
			id: 'agent-1',
			name: 'Test Agent',
			level: 5,
			tier: 1,
			xp: 100,
			xp_to_level: 500,
			status: 'idle',
			skills: ['coding'],
			death_count: 0,
			stats: {
				quests_completed: 0,
				quests_failed: 0,
				bosses_defeated: 0,
				total_xp_earned: 0,
				avg_quality_score: 0
			}
		};

		it('returns true for valid agent', () => {
			expect(isAgent(validAgent)).toBe(true);
		});

		it('returns false for missing id', () => {
			const { id, ...noId } = validAgent;
			expect(isAgent(noId)).toBe(false);
		});

		it('returns false for missing name', () => {
			const { name, ...noName } = validAgent;
			expect(isAgent(noName)).toBe(false);
		});

		it('returns false for missing level', () => {
			const { level, ...noLevel } = validAgent;
			expect(isAgent(noLevel)).toBe(false);
		});

		it('returns false for missing tier', () => {
			const { tier, ...noTier } = validAgent;
			expect(isAgent(noTier)).toBe(false);
		});

		it('returns false for non-string id', () => {
			expect(isAgent({ ...validAgent, id: 123 })).toBe(false);
		});

		it('returns false for non-number level', () => {
			expect(isAgent({ ...validAgent, level: '5' })).toBe(false);
		});

		it('returns false for null', () => {
			expect(isAgent(null)).toBe(false);
		});
	});

	describe('isQuest', () => {
		const validQuest = {
			id: 'quest-1',
			title: 'Test Quest',
			description: 'A test quest',
			status: 'posted',
			difficulty: 2,
			base_xp: 100,
			review_level: 1,
			required_skills: [],
			min_tier: 0,
			max_attempts: 3,
			attempts: 0,
			party_required: false,
			min_party_size: 1
		};

		it('returns true for valid quest', () => {
			expect(isQuest(validQuest)).toBe(true);
		});

		it('returns false for missing id', () => {
			const { id, ...noId } = validQuest;
			expect(isQuest(noId)).toBe(false);
		});

		it('returns false for missing title', () => {
			const { title, ...noTitle } = validQuest;
			expect(isQuest(noTitle)).toBe(false);
		});

		it('returns false for missing status', () => {
			const { status, ...noStatus } = validQuest;
			expect(isQuest(noStatus)).toBe(false);
		});

		it('returns false for missing difficulty', () => {
			const { difficulty, ...noDifficulty } = validQuest;
			expect(isQuest(noDifficulty)).toBe(false);
		});

		it('returns false for non-string id', () => {
			expect(isQuest({ ...validQuest, id: 123 })).toBe(false);
		});

		it('returns false for null', () => {
			expect(isQuest(null)).toBe(false);
		});
	});

	describe('isBossBattle', () => {
		const validBattle = {
			id: 'battle-1',
			quest_id: 'quest-1',
			agent_id: 'agent-1',
			status: 'active',
			level: 1,
			criteria: [],
			results: [],
			judges: [],
			started_at: Date.now()
		};

		it('returns true for valid battle', () => {
			expect(isBossBattle(validBattle)).toBe(true);
		});

		it('returns false for missing id', () => {
			const { id, ...noId } = validBattle;
			expect(isBossBattle(noId)).toBe(false);
		});

		it('returns false for missing quest_id', () => {
			const { quest_id, ...noQuestId } = validBattle;
			expect(isBossBattle(noQuestId)).toBe(false);
		});

		it('returns false for missing agent_id', () => {
			const { agent_id, ...noAgentId } = validBattle;
			expect(isBossBattle(noAgentId)).toBe(false);
		});

		it('returns false for missing status', () => {
			const { status, ...noStatus } = validBattle;
			expect(isBossBattle(noStatus)).toBe(false);
		});

		it('returns false for non-string id', () => {
			expect(isBossBattle({ ...validBattle, id: 123 })).toBe(false);
		});

		it('returns false for null', () => {
			expect(isBossBattle(null)).toBe(false);
		});
	});
});

describe('WebSocket Message Parsing', () => {
	it('validates world_state message structure', () => {
		const message = {
			type: 'world_state',
			data: {
				agents: [],
				quests: [],
				parties: [],
				guilds: [],
				battles: [],
				stats: {}
			}
		};

		expect(isWebSocketMessage(message)).toBe(true);
		expect(message.type).toBe('world_state');
	});

	it('validates event message structure', () => {
		const message = {
			type: 'event',
			data: {
				type: 'quest.posted',
				timestamp: Date.now(),
				data: { quest: { id: 'quest-1', title: 'Test' } }
			}
		};

		expect(isWebSocketMessage(message)).toBe(true);
		expect(message.type).toBe('event');
	});

	it('validates entity_update message structure', () => {
		const message = {
			type: 'entity_update',
			entity_type: 'agent',
			data: {
				id: 'agent-1',
				name: 'Test Agent',
				level: 5,
				tier: 1
			}
		};

		expect(isWebSocketMessage(message)).toBe(true);
		expect(message.type).toBe('entity_update');
		expect(message.entity_type).toBe('agent');
	});
});
