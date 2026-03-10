/**
 * Tests for the buildGraphEntity helper exported from graphStore.svelte.ts.
 *
 * buildGraphEntity is a pure function (no Svelte reactivity) that normalises
 * raw adapter data into GraphEntity structures.  We test it in isolation to
 * avoid importing the singleton store (which calls createGraphStore() at
 * module-evaluation time and touches SvelteMap/SvelteSet).
 *
 * Coverage:
 * - Timestamp normalisation: ISO string → ms, numeric passthrough
 * - Outgoing relationship ID construction
 * - Incoming relationship ID construction
 * - Missing optional fields (properties, outgoing, incoming)
 */

import { describe, it, expect, beforeAll, vi } from 'vitest';
import { buildGraphEntity } from './graphStore.svelte';
import { createRelationshipId } from '$lib/api/graph-types';

// =============================================================================
// Constants
// =============================================================================

const AGENT_ID = 'c360.prod.game.board1.agent.dragon';
const QUEST_ID = 'c360.prod.game.board1.quest.fetch-data';
const GUILD_ID = 'c360.prod.game.board1.guild.wranglers';
const ISO_TIMESTAMP = '2026-01-15T10:00:00.000Z';
const ISO_TIMESTAMP_MS = new Date(ISO_TIMESTAMP).getTime();

// =============================================================================
// Timestamp normalisation
// =============================================================================

describe('buildGraphEntity — timestamp normalisation', () => {
	describe('properties', () => {
		it('converts an ISO string timestamp to Unix milliseconds', () => {
			const entity = buildGraphEntity({
				id: AGENT_ID,
				properties: [
					{
						predicate: 'agent.identity.name',
						object: 'Dragon',
						confidence: 0.9,
						source: 'test',
						timestamp: ISO_TIMESTAMP
					}
				]
			});

			expect(entity.properties[0].timestamp).toBe(ISO_TIMESTAMP_MS);
		});

		it('passes through a numeric timestamp unchanged', () => {
			const ms = 1_700_000_000_000;
			const entity = buildGraphEntity({
				id: AGENT_ID,
				properties: [
					{
						predicate: 'agent.identity.name',
						object: 'Dragon',
						confidence: 0.9,
						source: 'test',
						timestamp: ms
					}
				]
			});

			expect(entity.properties[0].timestamp).toBe(ms);
		});

		it('preserves zero as a valid numeric timestamp', () => {
			const entity = buildGraphEntity({
				id: AGENT_ID,
				properties: [
					{
						predicate: 'agent.progression.level',
						object: 5,
						confidence: 1.0,
						source: 'test',
						timestamp: 0
					}
				]
			});

			expect(entity.properties[0].timestamp).toBe(0);
		});

		it('defaults source to "unknown" when not provided', () => {
			const entity = buildGraphEntity({
				id: AGENT_ID,
				properties: [
					{
						predicate: 'agent.identity.name',
						object: 'Dragon',
						confidence: 1.0,
						timestamp: 0
						// no source field
					}
				]
			});

			expect(entity.properties[0].source).toBe('unknown');
		});
	});

	describe('outgoing relationships', () => {
		it('converts an ISO string timestamp to milliseconds', () => {
			const entity = buildGraphEntity({
				id: AGENT_ID,
				outgoing: [
					{
						predicate: 'agent.membership.guild',
						targetId: GUILD_ID,
						confidence: 1.0,
						timestamp: ISO_TIMESTAMP
					}
				]
			});

			expect(entity.outgoing[0].timestamp).toBe(ISO_TIMESTAMP_MS);
		});

		it('passes through a numeric timestamp unchanged', () => {
			const ms = 1_700_000_000_000;
			const entity = buildGraphEntity({
				id: AGENT_ID,
				outgoing: [
					{
						predicate: 'agent.membership.guild',
						targetId: GUILD_ID,
						confidence: 1.0,
						timestamp: ms
					}
				]
			});

			expect(entity.outgoing[0].timestamp).toBe(ms);
		});

		it('falls back to Date.now() when timestamp is missing', () => {
			const before = Date.now();
			const entity = buildGraphEntity({
				id: AGENT_ID,
				outgoing: [
					{
						predicate: 'agent.membership.guild',
						targetId: GUILD_ID,
						confidence: 1.0
						// no timestamp
					}
				]
			});
			const after = Date.now();

			expect(entity.outgoing[0].timestamp).toBeGreaterThanOrEqual(before);
			expect(entity.outgoing[0].timestamp).toBeLessThanOrEqual(after);
		});
	});

	describe('incoming relationships', () => {
		it('converts an ISO string timestamp to milliseconds', () => {
			const entity = buildGraphEntity({
				id: GUILD_ID,
				incoming: [
					{
						predicate: 'agent.membership.guild',
						sourceId: AGENT_ID,
						confidence: 1.0,
						timestamp: ISO_TIMESTAMP
					}
				]
			});

			expect(entity.incoming[0].timestamp).toBe(ISO_TIMESTAMP_MS);
		});

		it('passes through a numeric timestamp unchanged', () => {
			const ms = 1_700_000_000_000;
			const entity = buildGraphEntity({
				id: GUILD_ID,
				incoming: [
					{
						predicate: 'agent.membership.guild',
						sourceId: AGENT_ID,
						confidence: 1.0,
						timestamp: ms
					}
				]
			});

			expect(entity.incoming[0].timestamp).toBe(ms);
		});

		it('falls back to Date.now() when timestamp is missing', () => {
			const before = Date.now();
			const entity = buildGraphEntity({
				id: GUILD_ID,
				incoming: [
					{
						predicate: 'agent.membership.guild',
						sourceId: AGENT_ID,
						confidence: 1.0
					}
				]
			});
			const after = Date.now();

			expect(entity.incoming[0].timestamp).toBeGreaterThanOrEqual(before);
			expect(entity.incoming[0].timestamp).toBeLessThanOrEqual(after);
		});
	});
});

// =============================================================================
// Relationship ID construction
// =============================================================================

describe('buildGraphEntity — relationship ID construction', () => {
	it('constructs outgoing relationship ID as sourceId:predicate:targetId', () => {
		const entity = buildGraphEntity({
			id: AGENT_ID,
			outgoing: [
				{
					predicate: 'agent.membership.guild',
					targetId: GUILD_ID,
					confidence: 1.0,
					timestamp: 0
				}
			]
		});

		const expected = createRelationshipId(AGENT_ID, 'agent.membership.guild', GUILD_ID);
		expect(entity.outgoing[0].id).toBe(expected);
	});

	it('sets sourceId to the entity ID on outgoing relationships', () => {
		const entity = buildGraphEntity({
			id: AGENT_ID,
			outgoing: [
				{
					predicate: 'agent.membership.guild',
					targetId: GUILD_ID,
					confidence: 1.0,
					timestamp: 0
				}
			]
		});

		expect(entity.outgoing[0].sourceId).toBe(AGENT_ID);
		expect(entity.outgoing[0].targetId).toBe(GUILD_ID);
	});

	it('constructs incoming relationship ID as sourceId:predicate:entityId', () => {
		const entity = buildGraphEntity({
			id: GUILD_ID,
			incoming: [
				{
					predicate: 'agent.membership.guild',
					sourceId: AGENT_ID,
					confidence: 1.0,
					timestamp: 0
				}
			]
		});

		const expected = createRelationshipId(AGENT_ID, 'agent.membership.guild', GUILD_ID);
		expect(entity.incoming[0].id).toBe(expected);
	});

	it('sets targetId to the entity ID on incoming relationships', () => {
		const entity = buildGraphEntity({
			id: GUILD_ID,
			incoming: [
				{
					predicate: 'agent.membership.guild',
					sourceId: AGENT_ID,
					confidence: 1.0,
					timestamp: 0
				}
			]
		});

		expect(entity.incoming[0].sourceId).toBe(AGENT_ID);
		expect(entity.incoming[0].targetId).toBe(GUILD_ID);
	});

	it('produces the same ID for a matching outgoing/incoming pair', () => {
		const outgoingEntity = buildGraphEntity({
			id: AGENT_ID,
			outgoing: [
				{
					predicate: 'agent.membership.guild',
					targetId: GUILD_ID,
					confidence: 1.0,
					timestamp: 0
				}
			]
		});

		const incomingEntity = buildGraphEntity({
			id: GUILD_ID,
			incoming: [
				{
					predicate: 'agent.membership.guild',
					sourceId: AGENT_ID,
					confidence: 1.0,
					timestamp: 0
				}
			]
		});

		expect(outgoingEntity.outgoing[0].id).toBe(incomingEntity.incoming[0].id);
	});
});

// =============================================================================
// ID parts
// =============================================================================

describe('buildGraphEntity — idParts', () => {
	it('parses the 6-part entity ID into idParts correctly', () => {
		const entity = buildGraphEntity({ id: AGENT_ID });

		expect(entity.idParts.org).toBe('c360');
		expect(entity.idParts.platform).toBe('prod');
		expect(entity.idParts.domain).toBe('game');
		expect(entity.idParts.system).toBe('board1');
		expect(entity.idParts.type).toBe('agent');
		expect(entity.idParts.instance).toBe('dragon');
	});

	it('sets all parts to "unknown" for a malformed ID', () => {
		const entity = buildGraphEntity({ id: 'short-id' });

		expect(entity.idParts.org).toBe('short-id');
		// short-id has only 1 part via split('.'), remaining parts fall to 'unknown'
		expect(entity.idParts.platform).toBe('unknown');
		expect(entity.idParts.domain).toBe('unknown');
	});
});

// =============================================================================
// Missing optional fields
// =============================================================================

describe('buildGraphEntity — missing optional fields', () => {
	it('returns empty arrays when properties, outgoing, and incoming are omitted', () => {
		const entity = buildGraphEntity({ id: AGENT_ID });

		expect(entity.properties).toEqual([]);
		expect(entity.outgoing).toEqual([]);
		expect(entity.incoming).toEqual([]);
	});

	it('returns empty properties array when properties is undefined', () => {
		const entity = buildGraphEntity({ id: AGENT_ID, properties: undefined });
		expect(entity.properties).toEqual([]);
	});

	it('returns empty outgoing array when outgoing is undefined', () => {
		const entity = buildGraphEntity({ id: AGENT_ID, outgoing: undefined });
		expect(entity.outgoing).toEqual([]);
	});

	it('returns empty incoming array when incoming is undefined', () => {
		const entity = buildGraphEntity({ id: AGENT_ID, incoming: undefined });
		expect(entity.incoming).toEqual([]);
	});

	it('preserves the entity id on the returned GraphEntity', () => {
		const entity = buildGraphEntity({ id: QUEST_ID });
		expect(entity.id).toBe(QUEST_ID);
	});
});

// =============================================================================
// Multiple entries
// =============================================================================

describe('buildGraphEntity — multiple entries', () => {
	it('processes multiple properties in order', () => {
		const entity = buildGraphEntity({
			id: AGENT_ID,
			properties: [
				{ predicate: 'agent.identity.name', object: 'Dragon', confidence: 1.0, timestamp: 0 },
				{ predicate: 'agent.progression.level', object: 5, confidence: 1.0, timestamp: 0 }
			]
		});

		expect(entity.properties).toHaveLength(2);
		expect(entity.properties[0].predicate).toBe('agent.identity.name');
		expect(entity.properties[1].predicate).toBe('agent.progression.level');
	});

	it('processes multiple outgoing relationships', () => {
		const entity = buildGraphEntity({
			id: AGENT_ID,
			outgoing: [
				{ predicate: 'agent.membership.guild', targetId: GUILD_ID, confidence: 1.0, timestamp: 0 },
				{ predicate: 'quest.lifecycle.claimed', targetId: QUEST_ID, confidence: 1.0, timestamp: 0 }
			]
		});

		expect(entity.outgoing).toHaveLength(2);
		expect(entity.outgoing[0].targetId).toBe(GUILD_ID);
		expect(entity.outgoing[1].targetId).toBe(QUEST_ID);
	});
});
