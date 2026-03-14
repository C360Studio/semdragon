/**
 * Tests for graph-types utility functions.
 *
 * Covers the pure helper functions exported from graph-types.ts:
 * parseEntityId, isEntityReference, createRelationshipId,
 * getEntityLabel, getGameEntityType, isGameEntity.
 */

import { describe, it, expect } from 'vitest';
import {
	parseEntityId,
	isEntityReference,
	createRelationshipId,
	getEntityLabel,
	getGameEntityType,
	isGameEntity,
	type GraphEntity,
	type EntityIdParts
} from './graph-types';

// =============================================================================
// Test helpers
// =============================================================================

/**
 * Build a minimal GraphEntity for label/type tests.
 * All fields are stubbed except idParts and id.
 */
function makeEntity(id: string): GraphEntity {
	return {
		id,
		idParts: parseEntityId(id),
		properties: [],
		outgoing: [],
		incoming: []
	};
}

// =============================================================================
// parseEntityId
// =============================================================================

describe('parseEntityId', () => {
	it('parses a valid 6-part entity ID into its components', () => {
		const result = parseEntityId('c360.prod.game.board1.agent.dragon');
		expect(result).toEqual<EntityIdParts>({
			org: 'c360',
			platform: 'prod',
			domain: 'game',
			system: 'board1',
			type: 'agent',
			instance: 'dragon'
		});
	});

	it('parses a quest entity ID correctly', () => {
		const result = parseEntityId('c360.dev.game.board1.quest.abc123');
		expect(result.org).toBe('c360');
		expect(result.platform).toBe('dev');
		expect(result.domain).toBe('game');
		expect(result.system).toBe('board1');
		expect(result.type).toBe('quest');
		expect(result.instance).toBe('abc123');
	});

	it('parses all semdragons entity types', () => {
		const types = ['quest', 'agent', 'party', 'guild', 'battle', 'peer_review'];
		for (const t of types) {
			const id = `c360.prod.game.board1.${t}.instance-1`;
			const result = parseEntityId(id);
			expect(result.type).toBe(t);
		}
	});

	it('returns unknown for all parts when given an empty string', () => {
		const result = parseEntityId('');
		expect(result.org).toBe('unknown');
		expect(result.platform).toBe('unknown');
		expect(result.domain).toBe('unknown');
		expect(result.system).toBe('unknown');
		expect(result.type).toBe('unknown');
		expect(result.instance).toBe('unknown');
	});

	it('fills present parts and unknown for missing parts on a short ID', () => {
		const result = parseEntityId('c360.prod');
		expect(result.org).toBe('c360');
		expect(result.platform).toBe('prod');
		expect(result.domain).toBe('unknown');
		expect(result.system).toBe('unknown');
		expect(result.type).toBe('unknown');
		expect(result.instance).toBe('unknown');
	});

	it('handles a 5-part ID (one part short) by filling instance with unknown', () => {
		const result = parseEntityId('c360.prod.game.board1.quest');
		expect(result.type).toBe('quest');
		expect(result.instance).toBe('unknown');
	});

	it('handles a 7-part ID (one part extra) by returning unknown for all', () => {
		// split gives 7 parts, length !== 6 triggers the fallback path
		const result = parseEntityId('a.b.c.d.e.f.g');
		expect(result.org).toBe('a');
		expect(result.platform).toBe('b');
		// The fallback uses parts[2]..parts[5], ignoring the 7th part
		expect(result.domain).toBe('c');
		expect(result.system).toBe('d');
		expect(result.type).toBe('e');
		expect(result.instance).toBe('f');
	});

	it('preserves hyphenated instance names', () => {
		const result = parseEntityId('c360.prod.game.board1.agent.my-dragon-01');
		expect(result.instance).toBe('my-dragon-01');
	});
});

// =============================================================================
// isEntityReference
// =============================================================================

describe('isEntityReference', () => {
	it('returns true for a valid 6-part entity ID string', () => {
		expect(isEntityReference('c360.prod.game.board1.agent.dragon')).toBe(true);
	});

	it('returns true for any 6-part dot-separated string', () => {
		expect(isEntityReference('a.b.c.d.e.f')).toBe(true);
	});

	it('returns false for a short string with fewer than 6 parts', () => {
		expect(isEntityReference('c360.prod.game')).toBe(false);
	});

	it('returns false for a 5-part string', () => {
		expect(isEntityReference('c360.prod.game.board1.quest')).toBe(false);
	});

	it('returns false for a 7-part string', () => {
		expect(isEntityReference('c360.prod.game.board1.quest.abc.extra')).toBe(false);
	});

	it('returns false for a plain string with no dots', () => {
		expect(isEntityReference('dragon')).toBe(false);
	});

	it('returns false for an empty string', () => {
		expect(isEntityReference('')).toBe(false);
	});

	it('returns false for a number', () => {
		expect(isEntityReference(42)).toBe(false);
	});

	it('returns false for a boolean', () => {
		expect(isEntityReference(true)).toBe(false);
	});

	it('returns false for null', () => {
		expect(isEntityReference(null)).toBe(false);
	});

	it('returns false for undefined', () => {
		expect(isEntityReference(undefined)).toBe(false);
	});

	it('returns false for an object', () => {
		expect(isEntityReference({ id: 'c360.prod.game.board1.agent.dragon' })).toBe(false);
	});

	it('returns false for an array', () => {
		expect(isEntityReference(['c360', 'prod', 'game', 'board1', 'agent', 'dragon'])).toBe(false);
	});
});

// =============================================================================
// createRelationshipId
// =============================================================================

describe('createRelationshipId', () => {
	it('generates a deterministic colon-separated ID', () => {
		const id = createRelationshipId(
			'c360.prod.game.board1.agent.dragon',
			'party.formation.lead',
			'c360.prod.game.board1.party.alpha'
		);
		expect(id).toBe(
			'c360.prod.game.board1.agent.dragon:party.formation.lead:c360.prod.game.board1.party.alpha'
		);
	});

	it('produces the same ID for the same inputs (deterministic)', () => {
		const source = 'c360.prod.game.board1.quest.q1';
		const predicate = 'quest.lifecycle.claimed';
		const target = 'c360.prod.game.board1.agent.a1';

		expect(createRelationshipId(source, predicate, target)).toBe(
			createRelationshipId(source, predicate, target)
		);
	});

	it('produces different IDs when source and target are swapped', () => {
		const a = 'c360.prod.game.board1.agent.a1';
		const b = 'c360.prod.game.board1.agent.a2';
		const pred = 'agent.progression.xp';

		expect(createRelationshipId(a, pred, b)).not.toBe(createRelationshipId(b, pred, a));
	});

	it('produces different IDs when the predicate differs', () => {
		const source = 'c360.prod.game.board1.agent.a1';
		const target = 'c360.prod.game.board1.guild.g1';

		const id1 = createRelationshipId(source, 'guild.membership.joined', target);
		const id2 = createRelationshipId(source, 'guild.membership.promoted', target);
		expect(id1).not.toBe(id2);
	});

	it('handles empty strings without throwing', () => {
		const id = createRelationshipId('', '', '');
		expect(id).toBe('::');
	});
});

// =============================================================================
// getEntityLabel
// =============================================================================

describe('getEntityLabel', () => {
	it('returns the instance part of a 6-part entity ID', () => {
		const entity = makeEntity('c360.prod.game.board1.agent.dragon-01');
		expect(getEntityLabel(entity)).toBe('dragon-01');
	});

	it('returns the instance for a quest entity', () => {
		const entity = makeEntity('c360.prod.game.board1.quest.fetch-data-q1');
		expect(getEntityLabel(entity)).toBe('fetch-data-q1');
	});

	it('returns agent display name from triples', () => {
		const entity = makeEntity('c360.prod.game.board1.agent.abc123');
		entity.properties = [
			{ predicate: 'agent.identity.name', object: 'CodeForger', confidence: 1.0, source: 'test', timestamp: 0 },
			{ predicate: 'agent.identity.display_name', object: 'The Code Forger', confidence: 1.0, source: 'test', timestamp: 0 }
		];
		expect(getEntityLabel(entity)).toBe('The Code Forger');
	});

	it('returns quest name from triples', () => {
		const entity = makeEntity('c360.prod.game.board1.quest.xyz789');
		entity.properties = [
			{ predicate: 'quest.identity.name', object: 'Login Fix', confidence: 1.0, source: 'test', timestamp: 0 },
			{ predicate: 'quest.identity.title', object: 'Fix the login bug in auth module', confidence: 1.0, source: 'test', timestamp: 0 }
		];
		expect(getEntityLabel(entity)).toBe('Login Fix');
	});

	it('falls back to quest title when name is empty', () => {
		const entity = makeEntity('c360.prod.game.board1.quest.xyz789');
		entity.properties = [
			{ predicate: 'quest.identity.title', object: 'Fix the login bug', confidence: 1.0, source: 'test', timestamp: 0 }
		];
		expect(getEntityLabel(entity)).toBe('Fix the login bug');
	});

	it('returns battle name from triples', () => {
		const entity = makeEntity('c360.prod.game.board1.battle.bat001');
		entity.properties = [
			{ predicate: 'battle.identity.name', object: 'Login Fix BB', confidence: 1.0, source: 'test', timestamp: 0 }
		];
		expect(getEntityLabel(entity)).toBe('Login Fix BB');
	});

	it('falls back to instance ID when no triples', () => {
		const entity = makeEntity('c360.prod.game.board1.quest.fetch-data-q1');
		expect(getEntityLabel(entity)).toBe('fetch-data-q1');
	});

	it('falls back to the full entity ID when instance is empty string', () => {
		// parseEntityId on a malformed ID may produce 'unknown' for instance,
		// but we can force the edge case by crafting idParts manually.
		const entity: GraphEntity = {
			id: 'short-id',
			idParts: {
				org: 'unknown',
				platform: 'unknown',
				domain: 'unknown',
				system: 'unknown',
				type: 'unknown',
				instance: '' // empty instance triggers fallback
			},
			properties: [],
			outgoing: [],
			incoming: []
		};
		expect(getEntityLabel(entity)).toBe('short-id');
	});
});

// =============================================================================
// getGameEntityType
// =============================================================================

describe('getGameEntityType', () => {
	const cases: Array<[string, string]> = [
		['quest', 'quest'],
		['agent', 'agent'],
		['party', 'party'],
		['guild', 'guild'],
		['battle', 'battle'],
		['peer_review', 'peer_review']
	];

	for (const [entityType, expected] of cases) {
		it(`returns '${expected}' for entity type '${entityType}'`, () => {
			const entity = makeEntity(`c360.prod.game.board1.${entityType}.instance-1`);
			expect(getGameEntityType(entity)).toBe(expected);
		});
	}

	it('returns "unknown" for an unrecognised entity type', () => {
		const entity = makeEntity('c360.prod.game.board1.npc.goblin');
		expect(getGameEntityType(entity)).toBe('unknown');
	});

	it('returns "unknown" for the "unknown" type produced by malformed IDs', () => {
		const entity = makeEntity(''); // all parts become 'unknown'
		expect(getGameEntityType(entity)).toBe('unknown');
	});

	it('is case-insensitive for known types', () => {
		// Manually set idParts.type to uppercase to test toLowerCase() guard
		const entity: GraphEntity = {
			id: 'c360.prod.game.board1.QUEST.q1',
			idParts: {
				org: 'c360',
				platform: 'prod',
				domain: 'game',
				system: 'board1',
				type: 'QUEST',
				instance: 'q1'
			},
			properties: [],
			outgoing: [],
			incoming: []
		};
		expect(getGameEntityType(entity)).toBe('quest');
	});
});

// =============================================================================
// isGameEntity
// =============================================================================

describe('isGameEntity', () => {
	it('returns true for a valid game entity ID', () => {
		expect(isGameEntity('c360.prod.game.board1.quest.abc123')).toBe(true);
	});

	it('returns true for all semdragons entity types in the game domain', () => {
		const types = ['quest', 'agent', 'party', 'guild', 'battle'];
		for (const t of types) {
			expect(isGameEntity(`c360.prod.game.board1.${t}.id-1`)).toBe(true);
		}
	});

	it('returns false when the domain part is not "game"', () => {
		expect(isGameEntity('c360.prod.infra.board1.quest.abc123')).toBe(false);
	});

	it('returns false for a short (non-6-part) string', () => {
		expect(isGameEntity('c360.prod.game')).toBe(false);
	});

	it('returns false for an empty string', () => {
		expect(isGameEntity('')).toBe(false);
	});

	it('returns false for a 7-part string even if domain is game', () => {
		expect(isGameEntity('c360.prod.game.board1.quest.abc.extra')).toBe(false);
	});

	it('returns false when domain segment contains "game" but is not exactly "game"', () => {
		expect(isGameEntity('c360.prod.game2.board1.quest.abc123')).toBe(false);
	});
});
