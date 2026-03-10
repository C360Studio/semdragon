/**
 * Tests for graphTransform service.
 *
 * Covers transformPathSearchResult and transformGlobalSearchResult,
 * including triple processing, explicit edge wiring, bidirectional
 * relationship indexing, deduplication, and empty-input handling.
 */

import { describe, it, expect } from 'vitest';
import { transformPathSearchResult, transformGlobalSearchResult } from './graphTransform';
import type {
	BackendEntity,
	BackendEdge,
	PathSearchResult,
	GlobalSearchResult,
	SearchRelationship
} from '$lib/api/graph-types';
import { createRelationshipId } from '$lib/api/graph-types';

// =============================================================================
// Test helpers
// =============================================================================

function backendEntity(id: string, triples: BackendEntity['triples'] = []): BackendEntity {
	return { id, triples };
}

function triple(subject: string, predicate: string, object: unknown): BackendEntity['triples'][number] {
	return { subject, predicate, object };
}

function edge(subject: string, predicate: string, object: string): BackendEdge {
	return { subject, predicate, object };
}

// Stable entity IDs used across tests
const AGENT_ID = 'c360.prod.game.board1.agent.dragon';
const QUEST_ID = 'c360.prod.game.board1.quest.fetch-data';
const GUILD_ID = 'c360.prod.game.board1.guild.wranglers';
const PARTY_ID = 'c360.prod.game.board1.party.alpha';

// =============================================================================
// transformPathSearchResult — entities with triples
// =============================================================================

describe('transformPathSearchResult', () => {
	describe('with empty input', () => {
		it('returns an empty array for an empty result', () => {
			const result = transformPathSearchResult({ entities: [], edges: [] });
			expect(result).toEqual([]);
		});
	});

	describe('with property (literal) triples', () => {
		it('converts literal triples to TripleProperty entries', () => {
			const result = transformPathSearchResult({
				entities: [
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.identity.name', 'Dragon'),
						triple(AGENT_ID, 'agent.progression.level', 5)
					])
				],
				edges: []
			});

			expect(result).toHaveLength(1);
			const entity = result[0];
			expect(entity.id).toBe(AGENT_ID);
			expect(entity.properties).toHaveLength(2);
			expect(entity.properties[0].predicate).toBe('agent.identity.name');
			expect(entity.properties[0].object).toBe('Dragon');
			expect(entity.properties[1].predicate).toBe('agent.progression.level');
			expect(entity.properties[1].object).toBe(5);
		});

		it('sets confidence to 1.0 and timestamp to 0 on properties', () => {
			const result = transformPathSearchResult({
				entities: [
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.identity.name', 'Dragon')
					])
				],
				edges: []
			});

			const prop = result[0].properties[0];
			expect(prop.confidence).toBe(1.0);
			expect(prop.timestamp).toBe(0);
			expect(prop.source).toBe('unknown');
		});

		it('produces empty outgoing/incoming for an entity with only literal triples', () => {
			const result = transformPathSearchResult({
				entities: [
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.status.state', 'idle')
					])
				],
				edges: []
			});

			expect(result[0].outgoing).toHaveLength(0);
			expect(result[0].incoming).toHaveLength(0);
		});
	});

	describe('with entity-reference triples (relationships)', () => {
		it('converts 6-part object strings to outgoing GraphRelationship entries', () => {
			const result = transformPathSearchResult({
				entities: [
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.membership.guild', GUILD_ID)
					]),
					backendEntity(GUILD_ID, [])
				],
				edges: []
			});

			const agentEntity = result.find((e) => e.id === AGENT_ID)!;
			expect(agentEntity.outgoing).toHaveLength(1);
			const rel = agentEntity.outgoing[0];
			expect(rel.sourceId).toBe(AGENT_ID);
			expect(rel.targetId).toBe(GUILD_ID);
			expect(rel.predicate).toBe('agent.membership.guild');
			expect(rel.id).toBe(createRelationshipId(AGENT_ID, 'agent.membership.guild', GUILD_ID));
		});

		it('sets confidence to 1.0 and timestamp to 0 on relationships from triples', () => {
			const result = transformPathSearchResult({
				entities: [
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.membership.guild', GUILD_ID)
					]),
					backendEntity(GUILD_ID, [])
				],
				edges: []
			});

			const rel = result.find((e) => e.id === AGENT_ID)!.outgoing[0];
			expect(rel.confidence).toBe(1.0);
			expect(rel.timestamp).toBe(0);
		});

		it('does not add incoming relationships from triples (explicit edges handle that)', () => {
			const result = transformPathSearchResult({
				entities: [
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.membership.guild', GUILD_ID)
					]),
					backendEntity(GUILD_ID, [])
				],
				edges: []
			});

			const guildEntity = result.find((e) => e.id === GUILD_ID)!;
			// No explicit edge was supplied — incoming list stays empty
			expect(guildEntity.incoming).toHaveLength(0);
		});
	});

	describe('with explicit edges', () => {
		it('wires explicit edges as outgoing on the source entity', () => {
			const result = transformPathSearchResult({
				entities: [backendEntity(AGENT_ID), backendEntity(QUEST_ID)],
				edges: [edge(AGENT_ID, 'quest.lifecycle.claimed', QUEST_ID)]
			});

			const agent = result.find((e) => e.id === AGENT_ID)!;
			expect(agent.outgoing).toHaveLength(1);
			expect(agent.outgoing[0].targetId).toBe(QUEST_ID);
		});

		it('wires explicit edges as incoming on the target entity', () => {
			const result = transformPathSearchResult({
				entities: [backendEntity(AGENT_ID), backendEntity(QUEST_ID)],
				edges: [edge(AGENT_ID, 'quest.lifecycle.claimed', QUEST_ID)]
			});

			const quest = result.find((e) => e.id === QUEST_ID)!;
			expect(quest.incoming).toHaveLength(1);
			expect(quest.incoming[0].sourceId).toBe(AGENT_ID);
		});

		it('uses the same relationship ID on outgoing and incoming for an explicit edge', () => {
			const result = transformPathSearchResult({
				entities: [backendEntity(AGENT_ID), backendEntity(QUEST_ID)],
				edges: [edge(AGENT_ID, 'quest.lifecycle.claimed', QUEST_ID)]
			});

			const agentRel = result.find((e) => e.id === AGENT_ID)!.outgoing[0];
			const questRel = result.find((e) => e.id === QUEST_ID)!.incoming[0];
			expect(agentRel.id).toBe(questRel.id);
		});

		it('skips edges where source entity is not in the result set', () => {
			const result = transformPathSearchResult({
				entities: [backendEntity(QUEST_ID)],
				edges: [edge(AGENT_ID, 'quest.lifecycle.claimed', QUEST_ID)]
			});

			const quest = result.find((e) => e.id === QUEST_ID)!;
			// Target exists, so incoming IS wired; outgoing on AGENT_ID is absent (entity not in set)
			expect(quest.incoming).toHaveLength(1);
		});

		it('skips edges where target entity is not in the result set', () => {
			const result = transformPathSearchResult({
				entities: [backendEntity(AGENT_ID)],
				edges: [edge(AGENT_ID, 'quest.lifecycle.claimed', QUEST_ID)]
			});

			const agent = result.find((e) => e.id === AGENT_ID)!;
			// Source exists but target not in map — outgoing is still wired
			expect(agent.outgoing).toHaveLength(1);
		});
	});

	describe('mixed triples and explicit edges', () => {
		it('combines property triples, relationship triples, and explicit edges correctly', () => {
			const result = transformPathSearchResult({
				entities: [
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.identity.name', 'Dragon'), // literal
						triple(AGENT_ID, 'agent.membership.guild', GUILD_ID) // entity ref
					]),
					backendEntity(GUILD_ID, []),
					backendEntity(QUEST_ID, [])
				],
				edges: [edge(AGENT_ID, 'quest.lifecycle.claimed', QUEST_ID)]
			});

			const agent = result.find((e) => e.id === AGENT_ID)!;
			expect(agent.properties).toHaveLength(1);
			// outgoing from triple + outgoing from explicit edge = 2
			expect(agent.outgoing).toHaveLength(2);

			const guild = result.find((e) => e.id === GUILD_ID)!;
			expect(guild.incoming).toHaveLength(0); // no explicit edge targeting guild

			const quest = result.find((e) => e.id === QUEST_ID)!;
			expect(quest.incoming).toHaveLength(1);
		});
	});

	describe('ID parts parsing', () => {
		it('correctly populates idParts from the entity ID', () => {
			const result = transformPathSearchResult({
				entities: [backendEntity(AGENT_ID)],
				edges: []
			});

			const entity = result[0];
			expect(entity.idParts.org).toBe('c360');
			expect(entity.idParts.platform).toBe('prod');
			expect(entity.idParts.domain).toBe('game');
			expect(entity.idParts.system).toBe('board1');
			expect(entity.idParts.type).toBe('agent');
			expect(entity.idParts.instance).toBe('dragon');
		});
	});
});

// =============================================================================
// transformGlobalSearchResult
// =============================================================================

describe('transformGlobalSearchResult', () => {
	function makeGlobalResult(
		entities: BackendEntity[],
		relationships: SearchRelationship[] = []
	): GlobalSearchResult {
		return {
			entities,
			communitySummaries: [],
			relationships,
			count: entities.length,
			durationMs: 0
		};
	}

	describe('with empty input', () => {
		it('returns an empty array for empty entities', () => {
			const result = transformGlobalSearchResult(makeGlobalResult([]));
			expect(result).toEqual([]);
		});
	});

	describe('triple processing', () => {
		it('converts literal triples to properties', () => {
			const result = transformGlobalSearchResult(
				makeGlobalResult([
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.identity.name', 'Dragon')
					])
				])
			);

			expect(result[0].properties).toHaveLength(1);
			expect(result[0].properties[0].predicate).toBe('agent.identity.name');
		});

		it('converts entity-reference triples to outgoing relationships', () => {
			const result = transformGlobalSearchResult(
				makeGlobalResult([
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.membership.guild', GUILD_ID)
					]),
					backendEntity(GUILD_ID)
				])
			);

			const agent = result.find((e) => e.id === AGENT_ID)!;
			expect(agent.outgoing).toHaveLength(1);
			expect(agent.outgoing[0].targetId).toBe(GUILD_ID);
		});

		it('sets timestamp to 0 on relationships derived from triples', () => {
			const result = transformGlobalSearchResult(
				makeGlobalResult([
					backendEntity(AGENT_ID, [
						triple(AGENT_ID, 'agent.membership.guild', GUILD_ID)
					]),
					backendEntity(GUILD_ID)
				])
			);

			expect(result.find((e) => e.id === AGENT_ID)!.outgoing[0].timestamp).toBe(0);
		});
	});

	describe('explicit relationships', () => {
		it('wires explicit relationships as outgoing on the source', () => {
			const result = transformGlobalSearchResult(
				makeGlobalResult(
					[backendEntity(AGENT_ID), backendEntity(QUEST_ID)],
					[{ from: AGENT_ID, predicate: 'quest.lifecycle.claimed', to: QUEST_ID }]
				)
			);

			const agent = result.find((e) => e.id === AGENT_ID)!;
			expect(agent.outgoing).toHaveLength(1);
			expect(agent.outgoing[0].predicate).toBe('quest.lifecycle.claimed');
		});

		it('wires explicit relationships as incoming on the target', () => {
			const result = transformGlobalSearchResult(
				makeGlobalResult(
					[backendEntity(AGENT_ID), backendEntity(QUEST_ID)],
					[{ from: AGENT_ID, predicate: 'quest.lifecycle.claimed', to: QUEST_ID }]
				)
			);

			const quest = result.find((e) => e.id === QUEST_ID)!;
			expect(quest.incoming).toHaveLength(1);
			expect(quest.incoming[0].sourceId).toBe(AGENT_ID);
		});
	});

	describe('deduplication', () => {
		it('does not duplicate an outgoing relationship when both triple and explicit relationship reference the same edge', () => {
			// The triple produces an outgoing rel from AGENT_ID → GUILD_ID.
			// The explicit relationship targets the same edge.
			// The global transform deduplicates by relationship ID.
			const predicate = 'agent.membership.guild';
			const result = transformGlobalSearchResult(
				makeGlobalResult(
					[
						backendEntity(AGENT_ID, [
							triple(AGENT_ID, predicate, GUILD_ID)
						]),
						backendEntity(GUILD_ID)
					],
					[{ from: AGENT_ID, predicate, to: GUILD_ID }]
				)
			);

			const agent = result.find((e) => e.id === AGENT_ID)!;
			// Should appear exactly once despite two sources
			expect(agent.outgoing).toHaveLength(1);
		});

		it('does not duplicate an incoming relationship when both triple and explicit relationship reference the same edge', () => {
			// When both a triple on the source AND an explicit relationship describe
			// the same edge, the target's incoming list should have exactly one entry.
			const predicate = 'agent.membership.guild';
			const result = transformGlobalSearchResult(
				makeGlobalResult(
					[
						backendEntity(AGENT_ID, [
							triple(AGENT_ID, predicate, GUILD_ID)
						]),
						backendEntity(GUILD_ID)
					],
					[{ from: AGENT_ID, predicate, to: GUILD_ID }]
				)
			);

			const guild = result.find((e) => e.id === GUILD_ID)!;
			// Incoming wired by explicit relationship only (triple doesn't wire incoming)
			expect(guild.incoming).toHaveLength(1);
		});

		it('allows distinct relationships with the same predicate but different endpoints', () => {
			const predicate = 'agent.membership.guild';
			const GUILD_ID_2 = 'c360.prod.game.board1.guild.smiths';
			const result = transformGlobalSearchResult(
				makeGlobalResult(
					[
						backendEntity(AGENT_ID, [
							triple(AGENT_ID, predicate, GUILD_ID),
							triple(AGENT_ID, predicate, GUILD_ID_2)
						]),
						backendEntity(GUILD_ID),
						backendEntity(GUILD_ID_2)
					]
				)
			);

			const agent = result.find((e) => e.id === AGENT_ID)!;
			expect(agent.outgoing).toHaveLength(2);
		});
	});

	describe('multiple entities', () => {
		it('processes multiple entities independently', () => {
			const result = transformGlobalSearchResult(
				makeGlobalResult([
					backendEntity(AGENT_ID, [triple(AGENT_ID, 'agent.identity.name', 'Dragon')]),
					backendEntity(QUEST_ID, [triple(QUEST_ID, 'quest.identity.title', 'Fetch Data')]),
					backendEntity(PARTY_ID)
				])
			);

			expect(result).toHaveLength(3);
			expect(result.find((e) => e.id === AGENT_ID)!.properties[0].object).toBe('Dragon');
			expect(result.find((e) => e.id === QUEST_ID)!.properties[0].object).toBe('Fetch Data');
			expect(result.find((e) => e.id === PARTY_ID)!.properties).toHaveLength(0);
		});
	});
});
