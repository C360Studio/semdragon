/**
 * Entity Transformer Tests
 *
 * Tests the conversion of graph triple format (from SSE/KV) to flat TypeScript types.
 * Predicates tested here match the Go Graphable.Triples() implementations in:
 *   - graphable.go (Agent, Quest, Guild, BossBattle, Party)
 *   - processor subdirectories (extended implementations)
 */

import { describe, it, expect } from 'vitest';
import {
	isGraphEntity,
	transformEntity
} from './entity-transformer';
import type {
	Agent,
	Quest,
	Guild,
	BossBattle,
	Party,
	EntityType
} from '$types';
import { agentId, questId, guildId, battleId, partyId } from '$types';
import schema from './entity-schema.generated.json';

// =============================================================================
// TEST HELPERS
// =============================================================================

interface TestTriple {
	subject: string;
	predicate: string;
	object: unknown;
	source: string;
	timestamp: string;
	confidence: number;
}

function triple(predicate: string, object: unknown): TestTriple {
	return {
		subject: 'test-entity',
		predicate,
		object,
		source: 'test',
		timestamp: '2026-01-15T10:00:00Z',
		confidence: 1.0
	};
}

function graphEntity(id: string, triples: TestTriple[]) {
	return {
		id,
		triples,
		message_type: { domain: 'game', category: 'entity', version: '1.0' },
		version: 1,
		updated_at: '2026-01-15T10:00:00Z'
	};
}

const AGENT_KEY = 'c360.prod.game.board1.agent.dragon-01';
const QUEST_KEY = 'c360.prod.game.board1.quest.fetch-data';
const GUILD_KEY = 'c360.prod.game.board1.guild.datawranglers';
const BATTLE_KEY = 'c360.prod.game.board1.battle.review-001';
const PARTY_KEY = 'c360.prod.game.board1.party.alpha-team';

// =============================================================================
// isGraphEntity
// =============================================================================

describe('isGraphEntity', () => {
	it('returns true for valid graph entity with triples array', () => {
		expect(isGraphEntity({ id: 'test', triples: [] })).toBe(true);
	});

	it('returns true for graph entity with populated triples', () => {
		const entity = graphEntity('test', [triple('agent.identity.name', 'Dragon')]);
		expect(isGraphEntity(entity)).toBe(true);
	});

	it('returns false for null', () => {
		expect(isGraphEntity(null)).toBe(false);
	});

	it('returns false for undefined', () => {
		expect(isGraphEntity(undefined)).toBe(false);
	});

	it('returns false for primitive string', () => {
		expect(isGraphEntity('hello')).toBe(false);
	});

	it('returns false for number', () => {
		expect(isGraphEntity(42)).toBe(false);
	});

	it('returns false for object without triples', () => {
		expect(isGraphEntity({ id: 'test', name: 'Dragon' })).toBe(false);
	});

	it('returns false when triples is not an array', () => {
		expect(isGraphEntity({ id: 'test', triples: 'not-array' })).toBe(false);
	});

	it('returns false when triples is an object', () => {
		expect(isGraphEntity({ id: 'test', triples: {} })).toBe(false);
	});

	it('returns true for flat entity that happens to have triples array', () => {
		// A flat entity with a triples field would be treated as graph entity
		expect(isGraphEntity({ id: 'test', name: 'Agent', triples: [] })).toBe(true);
	});
});

// =============================================================================
// transformEntity — routing
// =============================================================================

describe('transformEntity', () => {
	it('routes agent entities to transformAgent', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.identity.name', 'TestAgent')
		]);
		const result = transformEntity('agent', AGENT_KEY, entity) as Agent;
		expect(result.id).toBe(agentId(AGENT_KEY));
		expect(result.name).toBe('TestAgent');
	});

	it('routes quest entities to transformQuest', () => {
		const entity = graphEntity(QUEST_KEY, [
			triple('quest.identity.title', 'Fetch Data')
		]);
		const result = transformEntity('quest', QUEST_KEY, entity) as Quest;
		expect(result.id).toBe(questId(QUEST_KEY));
		expect(result.title).toBe('Fetch Data');
	});

	it('routes guild entities to transformGuild', () => {
		const entity = graphEntity(GUILD_KEY, [
			triple('guild.identity.name', 'Data Wranglers')
		]);
		const result = transformEntity('guild', GUILD_KEY, entity) as Guild;
		expect(result.id).toBe(guildId(GUILD_KEY));
		expect(result.name).toBe('Data Wranglers');
	});

	it('routes battle entities to transformBattle', () => {
		const entity = graphEntity(BATTLE_KEY, [
			triple('battle.status.state', 'active')
		]);
		const result = transformEntity('battle', BATTLE_KEY, entity) as BossBattle;
		expect(result.id).toBe(battleId(BATTLE_KEY));
		expect(result.status).toBe('active');
	});

	it('routes party entities to transformParty', () => {
		const entity = graphEntity(PARTY_KEY, [
			triple('party.identity.name', 'Alpha Team')
		]);
		const result = transformEntity('party', PARTY_KEY, entity) as Party;
		expect(result.id).toBe(partyId(PARTY_KEY));
		expect(result.name).toBe('Alpha Team');
	});

	it('passes through non-graph entities unchanged', () => {
		const flat = { id: agentId('agent-1'), name: 'Flat Agent' };
		const result = transformEntity('agent', 'agent-1', flat);
		expect(result).toBe(flat); // same reference
	});

	it('returns null for unknown entity type', () => {
		const entity = graphEntity('test', []);
		const result = transformEntity('unknown' as EntityType, 'test', entity);
		expect(result).toBeNull();
	});
});

// =============================================================================
// transformAgent — happy path
// =============================================================================

describe('transformAgent', () => {
	function fullAgentTriples() {
		return [
			triple('agent.identity.name', 'Shadowweaver'),
			triple('agent.identity.display_name', 'The Shadowweaver'),
			triple('agent.status.state', 'on_quest'),
			triple('agent.progression.level', 12),
			triple('agent.progression.xp.current', 4500),
			triple('agent.progression.xp.to_level', 6000),
			triple('agent.progression.tier', 2),
			triple('agent.progression.death_count', 1),
			triple('agent.npc.flag', true),
			triple('agent.stats.quests_completed', 45),
			triple('agent.stats.quests_failed', 3),
			triple('agent.stats.bosses_defeated', 40),
			triple('agent.stats.total_xp_earned', 125000),
			triple('agent.skill.analysis.level', 3),
			triple('agent.skill.analysis.total_xp', 8000),
			triple('agent.skill.code_generation.level', 4),
			triple('agent.skill.code_generation.total_xp', 12000),
			triple('agent.membership.guild', 'c360.prod.game.board1.guild.datawranglers'),
			triple('agent.membership.guild', 'c360.prod.game.board1.guild.codesmiths'),
			triple('agent.lifecycle.created_at', '2026-01-01T00:00:00Z'),
			triple('agent.lifecycle.updated_at', '2026-01-15T10:00:00Z')
		];
	}

	it('transforms all agent fields correctly', () => {
		const entity = graphEntity(AGENT_KEY, fullAgentTriples());
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.id).toBe(agentId(AGENT_KEY));
		expect(agent.name).toBe('Shadowweaver');
		expect(agent.display_name).toBe('The Shadowweaver');
		expect(agent.status).toBe('on_quest');
		expect(agent.level).toBe(12);
		expect(agent.xp).toBe(4500);
		expect(agent.xp_to_level).toBe(6000);
		expect(agent.tier).toBe(2);
		expect(agent.death_count).toBe(1);
		expect(agent.is_npc).toBe(true);
		expect(agent.created_at).toBe('2026-01-01T00:00:00Z');
		expect(agent.updated_at).toBe('2026-01-15T10:00:00Z');
	});

	it('extracts agent stats', () => {
		const entity = graphEntity(AGENT_KEY, fullAgentTriples());
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.stats.quests_completed).toBe(45);
		expect(agent.stats.quests_failed).toBe(3);
		expect(agent.stats.bosses_defeated).toBe(40);
		expect(agent.stats.total_xp_earned).toBe(125000);
		// Hardcoded fields
		expect(agent.stats.bosses_failed).toBe(0);
		expect(agent.stats.total_xp_spent).toBe(0);
		expect(agent.stats.avg_quality_score).toBe(0);
		expect(agent.stats.avg_efficiency).toBe(0);
		expect(agent.stats.parties_led).toBe(0);
		expect(agent.stats.quests_decomposed).toBe(0);
	});

	it('extracts skill proficiencies via regex', () => {
		const entity = graphEntity(AGENT_KEY, fullAgentTriples());
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.skill_proficiencies['analysis']).toEqual({
			level: 3,
			progress: 50,
			total_xp: 8000,
			quests_used: 0
		});
		expect(agent.skill_proficiencies['code_generation']).toEqual({
			level: 4,
			progress: 50,
			total_xp: 12000,
			quests_used: 0
		});
	});

	it('extracts single guild membership (last value wins via tripleMap)', () => {
		const entity = graphEntity(AGENT_KEY, fullAgentTriples());
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		// tripleMap uses last-write-wins, so the last guild triple wins
		expect(agent.guild_id).toBe(guildId('c360.prod.game.board1.guild.codesmiths'));
	});

	it('provides hardcoded defaults for non-triple fields', () => {
		const entity = graphEntity(AGENT_KEY, fullAgentTriples());
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.equipment).toEqual([]);
		expect(agent.config).toEqual({
			provider: '',
			model: '',
			system_prompt: '',
			temperature: 0,
			max_tokens: 0,
			metadata: {}
		});
	});

	// ---------------------------------------------------------------------------
	// transformAgent — defaults when predicates are missing
	// ---------------------------------------------------------------------------

	it('uses key as name fallback when agent.identity.name is missing', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.name).toBe(AGENT_KEY);
	});

	it('defaults display_name to empty string', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.display_name).toBe('');
	});

	it('defaults status to idle', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.status).toBe('idle');
	});

	it('defaults level to 1', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.level).toBe(1);
	});

	it('defaults xp_to_level to 100', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.xp_to_level).toBe(100);
	});

	it('defaults xp to 0', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.xp).toBe(0);
	});

	it('defaults death_count to 0', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.death_count).toBe(0);
	});

	it('defaults is_npc to false when flag is missing', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.is_npc).toBeFalsy();
	});

	it('treats non-true values for npc flag as false', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.npc.flag', 'yes') // string, not boolean true
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.is_npc).toBeFalsy();
	});

	// ---------------------------------------------------------------------------
	// transformAgent — tier calculation
	// ---------------------------------------------------------------------------

	it('uses explicit tier from triples when present', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.progression.level', 5),
			triple('agent.progression.tier', 3) // override: master tier at level 5
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.tier).toBe(3);
	});

	it('calculates tier from level when tier predicate is missing', () => {
		const cases = [
			{ level: 1, expectedTier: 0 },
			{ level: 5, expectedTier: 0 },
			{ level: 6, expectedTier: 1 },
			{ level: 10, expectedTier: 1 },
			{ level: 11, expectedTier: 2 },
			{ level: 15, expectedTier: 2 },
			{ level: 16, expectedTier: 3 },
			{ level: 18, expectedTier: 3 },
			{ level: 19, expectedTier: 4 },
			{ level: 20, expectedTier: 4 }
		];

		for (const { level, expectedTier } of cases) {
			const entity = graphEntity(AGENT_KEY, [
				triple('agent.progression.level', level)
			]);
			const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;
			expect(agent.tier).toBe(expectedTier);
		}
	});

	// ---------------------------------------------------------------------------
	// transformAgent — skill proficiency edge cases
	// ---------------------------------------------------------------------------

	it('handles skill with level but no XP triple', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.skill.research.level', 2)
			// No agent.skill.research.total_xp
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.skill_proficiencies['research']).toEqual({
			level: 2,
			progress: 50,
			total_xp: 0,
			quests_used: 0
		});
	});

	it('ignores XP triple without matching level triple', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.skill.planning.total_xp', 5000)
			// No agent.skill.planning.level — regex won't match
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.skill_proficiencies['planning']).toBeUndefined();
	});

	it('handles multiple skills', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.skill.code_review.level', 5),
			triple('agent.skill.code_review.total_xp', 20000),
			triple('agent.skill.summarization.level', 1),
			triple('agent.skill.summarization.total_xp', 100),
			triple('agent.skill.training.level', 3),
			triple('agent.skill.training.total_xp', 5500)
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(Object.keys(agent.skill_proficiencies)).toHaveLength(3);
		expect(agent.skill_proficiencies['code_review'].level).toBe(5);
		expect(agent.skill_proficiencies['summarization'].level).toBe(1);
		expect(agent.skill_proficiencies['training'].level).toBe(3);
	});

	it('defaults skill level to 1 when object is non-numeric', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.skill.analysis.level', 'high') // string, not number
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.skill_proficiencies['analysis'].level).toBe(1);
	});

	it('returns null guild_id when no membership triple', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.guild_id).toBeNull();
	});
});

// =============================================================================
// transformQuest
// =============================================================================

describe('transformQuest', () => {
	function fullQuestTriples() {
		return [
			triple('quest.identity.title', 'Analyze Sales Data'),
			triple('quest.identity.description', 'Run analysis on Q4 sales figures'),
			triple('quest.status.state', 'in_progress'),
			triple('quest.difficulty.level', 3),
			triple('quest.tier.minimum', 1),
			triple('quest.party.required', true),
			triple('quest.xp.base', 500),
			triple('quest.review.level', 2),
			triple('quest.lifecycle.posted_at', '2026-01-10T08:00:00Z'),
			triple('quest.attempts.current', 1),
			triple('quest.attempts.max', 5)
		];
	}

	it('transforms all quest fields correctly', () => {
		const entity = graphEntity(QUEST_KEY, fullQuestTriples());
		const quest = transformEntity('quest', QUEST_KEY, entity) as Quest;

		expect(quest.id).toBe(questId(QUEST_KEY));
		expect(quest.title).toBe('Analyze Sales Data');
		expect(quest.description).toBe('Run analysis on Q4 sales figures');
		expect(quest.status).toBe('in_progress');
		expect(quest.difficulty).toBe(3);
		expect(quest.min_tier).toBe(1);
		expect(quest.party_required).toBe(true);
		expect(quest.base_xp).toBe(500);
		expect(quest.constraints.review_level).toBe(2);
		expect(quest.posted_at).toBe('2026-01-10T08:00:00Z');
		expect(quest.attempts).toBe(1);
		expect(quest.max_attempts).toBe(5);
	});

	it('uses correct defaults when all predicates are missing', () => {
		const entity = graphEntity(QUEST_KEY, []);
		const quest = transformEntity('quest', QUEST_KEY, entity) as Quest;

		expect(quest.title).toBe('Untitled Quest');
		expect(quest.description).toBe('');
		expect(quest.status).toBe('posted');
		expect(quest.difficulty).toBe(0);
		expect(quest.min_tier).toBe(0);
		expect(quest.party_required).toBe(false);
		expect(quest.base_xp).toBe(100);
		expect(quest.max_attempts).toBe(3);
		expect(quest.attempts).toBe(0);
		expect(quest.posted_at).toBe('');
	});

	it('defaults party_required to false when not strictly true', () => {
		const entity = graphEntity(QUEST_KEY, [
			triple('quest.party.required', 'yes') // string, not boolean true
		]);
		const quest = transformEntity('quest', QUEST_KEY, entity) as Quest;

		expect(quest.party_required).toBe(false);
	});

	it('provides hardcoded defaults for non-triple fields', () => {
		const entity = graphEntity(QUEST_KEY, []);
		const quest = transformEntity('quest', QUEST_KEY, entity) as Quest;

		expect(quest.required_skills).toEqual([]);
		expect(quest.required_tools).toEqual([]);
		expect(quest.bonus_xp).toBe(0);
		expect(quest.guild_xp).toBe(0);
		expect(quest.input).toBeNull();
		expect(quest.sub_quests).toEqual([]);
		expect(quest.escalated).toBe(false);
		expect(quest.loop_id).toBe('');
		expect(quest.min_party_size).toBe(0);
		expect(quest.parent_quest).toBeUndefined();
	});

	it('provides hardcoded constraint defaults', () => {
		const entity = graphEntity(QUEST_KEY, []);
		const quest = transformEntity('quest', QUEST_KEY, entity) as Quest;

		expect(quest.constraints.max_duration).toBe(0);
		expect(quest.constraints.max_cost).toBe(0);
		expect(quest.constraints.max_tokens).toBe(0);
		expect(quest.constraints.require_review).toBe(false);
	});
});

// =============================================================================
// transformGuild
// =============================================================================

describe('transformGuild', () => {
	function fullGuildTriples() {
		return [
			triple('guild.identity.name', 'Data Wranglers'),
			triple('guild.identity.description', 'Specialists in data'),
			triple('guild.identity.culture', 'Data-driven excellence'),
			triple('guild.identity.motto', 'In data we trust'),
			triple('guild.status.state', 'active'),
			triple('guild.config.max_members', 50),
			triple('guild.config.min_level', 3),
			triple('guild.founding.date', '2026-01-01T00:00:00Z'),
			triple('guild.founding.agent', 'c360.prod.game.board1.agent.founder'),
			triple('guild.stats.reputation', 0.92),
			triple('guild.stats.quests_handled', 150),
			triple('guild.stats.success_rate', 0.87),
			triple('guild.stats.quests_failed', 20),
			triple('guild.routing.quest_type', 'analysis'),
			triple('guild.routing.quest_type', 'data_transformation'),
			triple('guild.routing.quest_type', 'research'),
			triple('guild.lifecycle.created_at', '2026-01-01T00:00:00Z')
		];
	}

	it('transforms all guild fields correctly', () => {
		const entity = graphEntity(GUILD_KEY, fullGuildTriples());
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild.id).toBe(guildId(GUILD_KEY));
		expect(guild.name).toBe('Data Wranglers');
		expect(guild.description).toBe('Specialists in data');
		expect(guild.culture).toBe('Data-driven excellence');
		expect(guild.motto).toBe('In data we trust');
		expect(guild.status).toBe('active');
		expect(guild.max_members).toBe(50);
		expect(guild.min_level).toBe(3);
		expect(guild.founded).toBe('2026-01-01T00:00:00Z');
		expect(guild.reputation).toBe(0.92);
		expect(guild.quests_handled).toBe(150);
		expect(guild.success_rate).toBe(0.87);
		expect(guild.quests_failed).toBe(20);
		expect(guild.created_at).toBe('2026-01-01T00:00:00Z');
	});

	it('collects multiple quest types from repeated predicates', () => {
		const entity = graphEntity(GUILD_KEY, fullGuildTriples());
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild.quest_types).toEqual(['analysis', 'data_transformation', 'research']);
	});

	it('sets quest_types to undefined when no routing predicates exist', () => {
		const entity = graphEntity(GUILD_KEY, []);
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild.quest_types).toBeUndefined();
	});

	it('uses correct defaults when all predicates are missing', () => {
		const entity = graphEntity(GUILD_KEY, []);
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild.name).toBe('Unknown Guild');
		expect(guild.description).toBe('');
		expect(guild.status).toBe('active');
		expect(guild.max_members).toBe(50);
		expect(guild.min_level).toBe(1);
		expect(guild.reputation).toBe(0);
		expect(guild.quests_handled).toBe(0);
		expect(guild.success_rate).toBe(0);
		expect(guild.quests_failed).toBe(0);
	});

	it('provides empty defaults for non-triple fields', () => {
		const entity = graphEntity(GUILD_KEY, []);
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild.members).toEqual([]);
		expect(guild.shared_tools).toEqual([]);
	});

	it('extracts guild members from membership triples', () => {
		const entity = graphEntity(GUILD_KEY, [
			...fullGuildTriples(),
			triple('guild.membership.agent', 'agent-alpha'),
			triple('guild.member.agent-alpha.rank', 'guildmaster'),
			triple('guild.member.agent-alpha.contribution', 42),
			triple('guild.membership.agent', 'agent-beta'),
			triple('guild.member.agent-beta.rank', 'member'),
			triple('guild.member.agent-beta.contribution', 10),
			triple('guild.resource.tool', 'grep'),
			triple('guild.resource.tool', 'curl')
		]);
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild.members).toHaveLength(2);
		expect(guild.members[0].rank).toBe('guildmaster');
		expect(guild.members[0].contribution).toBe(42);
		expect(guild.members[1].rank).toBe('member');
		expect(guild.members[1].contribution).toBe(10);
		expect(guild.shared_tools).toEqual(['grep', 'curl']);
	});
});

// =============================================================================
// transformBattle
// =============================================================================

describe('transformBattle', () => {
	it('transforms battle fields correctly', () => {
		const entity = graphEntity(BATTLE_KEY, [
			triple('battle.assignment.quest', QUEST_KEY),
			triple('battle.assignment.agent', AGENT_KEY),
			triple('battle.review.level', 2),
			triple('battle.status.state', 'victory'),
			triple('battle.lifecycle.started_at', '2026-01-15T09:00:00Z')
		]);
		const battle = transformEntity('battle', BATTLE_KEY, entity) as BossBattle;

		expect(battle.id).toBe(battleId(BATTLE_KEY));
		expect(battle.quest_id).toBe(questId(QUEST_KEY));
		expect(battle.agent_id).toBe(agentId(AGENT_KEY));
		expect(battle.level).toBe(2);
		expect(battle.status).toBe('victory');
		expect(battle.started_at).toBe('2026-01-15T09:00:00Z');
	});

	it('uses correct defaults when all predicates are missing', () => {
		const entity = graphEntity(BATTLE_KEY, []);
		const battle = transformEntity('battle', BATTLE_KEY, entity) as BossBattle;

		expect(battle.status).toBe('active');
		expect(battle.level).toBe(0);
		expect(battle.quest_id).toBe(questId(''));
		expect(battle.agent_id).toBe(agentId(''));
		expect(battle.started_at).toBe('');
	});

	it('provides hardcoded defaults for non-triple fields', () => {
		const entity = graphEntity(BATTLE_KEY, []);
		const battle = transformEntity('battle', BATTLE_KEY, entity) as BossBattle;

		expect(battle.criteria).toEqual([]);
		expect(battle.results).toBeUndefined();
		expect(battle.judges).toEqual([]);
	});
});

// =============================================================================
// transformParty
// =============================================================================

describe('transformParty', () => {
	it('transforms party fields correctly', () => {
		const entity = graphEntity(PARTY_KEY, [
			triple('party.identity.name', 'Alpha Strike'),
			triple('party.status.state', 'active'),
			triple('party.quest', QUEST_KEY),
			triple('party.lead', AGENT_KEY),
			triple('party.strategy', 'balanced'),
			triple('party.lifecycle.formed_at', '2026-01-14T12:00:00Z')
		]);
		const party = transformEntity('party', PARTY_KEY, entity) as Party;

		expect(party.id).toBe(partyId(PARTY_KEY));
		expect(party.name).toBe('Alpha Strike');
		expect(party.status).toBe('active');
		expect(party.quest_id).toBe(questId(QUEST_KEY));
		expect(party.lead).toBe(agentId(AGENT_KEY));
		expect(party.strategy).toBe('balanced');
		expect(party.formed_at).toBe('2026-01-14T12:00:00Z');
	});

	it('uses correct defaults when all predicates are missing', () => {
		const entity = graphEntity(PARTY_KEY, []);
		const party = transformEntity('party', PARTY_KEY, entity) as Party;

		expect(party.name).toBe('Unknown Party');
		expect(party.status).toBe('forming');
		expect(party.quest_id).toBe(questId(''));
		expect(party.lead).toBe(agentId(''));
		expect(party.strategy).toBe('balanced');
		expect(party.formed_at).toBe('');
	});

	it('provides empty defaults for non-triple fields', () => {
		const entity = graphEntity(PARTY_KEY, []);
		const party = transformEntity('party', PARTY_KEY, entity) as Party;

		expect(party.members).toEqual([]);
		expect(party.sub_quest_map).toEqual({});
		expect(party.shared_context).toEqual([]);
		expect(party.sub_results).toEqual({});
	});

	it('extracts party members and sub-quest assignments from triples', () => {
		const entity = graphEntity(PARTY_KEY, [
			triple('party.identity.name', 'Alpha Strike'),
			triple('party.status.state', 'active'),
			triple('party.quest', QUEST_KEY),
			triple('party.lead', AGENT_KEY),
			triple('party.lifecycle.formed_at', '2026-01-14T12:00:00Z'),
			triple('party.membership.member', 'agent-1'),
			triple('party.member.agent-1.role', 'lead'),
			triple('party.membership.member', 'agent-2'),
			triple('party.member.agent-2.role', 'executor'),
			triple('party.assignment.quest-sub-1', 'agent-2')
		]);
		const party = transformEntity('party', PARTY_KEY, entity) as Party;

		expect(party.members).toHaveLength(2);
		expect(party.members[0].role).toBe('lead');
		expect(party.members[1].role).toBe('executor');
		expect(party.sub_quest_map).toEqual({ 'quest-sub-1': 'agent-2' });
	});
});

// =============================================================================
// TYPE COERCION EDGE CASES
// =============================================================================

describe('type coercion', () => {
	it('str() returns fallback for non-string values', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.identity.name', 42), // number instead of string
			triple('agent.status.state', null) // null instead of string
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.name).toBe(AGENT_KEY); // fallback to key
		expect(agent.status).toBe('idle'); // fallback to 'idle'
	});

	it('num() returns fallback for non-number values', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.progression.level', 'ten'), // string instead of number
			triple('agent.progression.xp.current', true) // boolean instead of number
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.level).toBe(1); // fallback
		expect(agent.xp).toBe(0); // fallback
	});

	it('num() accepts zero as valid (not treated as falsy)', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.progression.level', 0),
			triple('agent.progression.xp.current', 0)
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.level).toBe(0);
		expect(agent.xp).toBe(0);
	});

	it('str() accepts empty string as valid (not treated as falsy)', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.identity.name', '')
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		// Empty string IS a string, so str() returns it (not the key fallback)
		expect(agent.name).toBe('');
	});

	it('num() handles float values from Go', () => {
		const entity = graphEntity(GUILD_KEY, [
			triple('guild.stats.reputation', 0.92),
			triple('guild.stats.success_rate', 0.87)
		]);
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild.reputation).toBe(0.92);
		expect(guild.success_rate).toBe(0.87);
	});
});

// =============================================================================
// DUPLICATE PREDICATE BEHAVIOR
// =============================================================================

describe('duplicate predicates', () => {
	it('tripleMap uses last-write-wins for scalar predicates', () => {
		const entity = graphEntity(AGENT_KEY, [
			triple('agent.identity.name', 'First'),
			triple('agent.identity.name', 'Second') // overwrites
		]);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent.name).toBe('Second');
	});

	it('tripleValues collects all matching predicates for multi-value fields', () => {
		const entity = graphEntity(GUILD_KEY, [
			triple('guild.routing.quest_type', 'analysis'),
			triple('guild.routing.quest_type', 'code_review'),
			triple('guild.routing.quest_type', 'research')
		]);
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild.quest_types).toHaveLength(3);
		expect(guild.quest_types).toContain('analysis');
		expect(guild.quest_types).toContain('code_review');
		expect(guild.quest_types).toContain('research');
	});
});

// =============================================================================
// EMPTY TRIPLES
// =============================================================================

describe('empty triples array', () => {
	it('produces valid agent with all defaults', () => {
		const entity = graphEntity(AGENT_KEY, []);
		const agent = transformEntity('agent', AGENT_KEY, entity) as Agent;

		expect(agent).toBeTruthy();
		expect(agent.id).toBe(agentId(AGENT_KEY));
		expect(agent.level).toBe(1);
		expect(agent.tier).toBe(0); // tierFromLevel(1) = Apprentice
		expect(Object.keys(agent.skill_proficiencies)).toHaveLength(0);
		expect(agent.guild_id).toBeNull();
	});

	it('produces valid quest with all defaults', () => {
		const entity = graphEntity(QUEST_KEY, []);
		const quest = transformEntity('quest', QUEST_KEY, entity) as Quest;

		expect(quest).toBeTruthy();
		expect(quest.id).toBe(questId(QUEST_KEY));
		expect(quest.title).toBe('Untitled Quest');
	});

	it('produces valid guild with all defaults', () => {
		const entity = graphEntity(GUILD_KEY, []);
		const guild = transformEntity('guild', GUILD_KEY, entity) as Guild;

		expect(guild).toBeTruthy();
		expect(guild.id).toBe(guildId(GUILD_KEY));
		expect(guild.name).toBe('Unknown Guild');
	});

	it('produces valid battle with all defaults', () => {
		const entity = graphEntity(BATTLE_KEY, []);
		const battle = transformEntity('battle', BATTLE_KEY, entity) as BossBattle;

		expect(battle).toBeTruthy();
		expect(battle.id).toBe(battleId(BATTLE_KEY));
		expect(battle.status).toBe('active');
	});

	it('produces valid party with all defaults', () => {
		const entity = graphEntity(PARTY_KEY, []);
		const party = transformEntity('party', PARTY_KEY, entity) as Party;

		expect(party).toBeTruthy();
		expect(party.id).toBe(partyId(PARTY_KEY));
		expect(party.name).toBe('Unknown Party');
	});
});

// =============================================================================
// SCHEMA CONTRACT VALIDATION
// =============================================================================
// Every predicate from the Go schema must be either HANDLED (mapped to a field)
// or IGNORED (with a reason). Any predicate in neither set = test failure.
// This forces a conscious decision when Go adds new predicates.

describe('schema contract validation', () => {
	// -------------------------------------------------------------------------
	// AGENT
	// -------------------------------------------------------------------------

	const AGENT_HANDLED: Record<string, string> = {
		'agent.identity.name': 'name',
		'agent.identity.display_name': 'display_name',
		'agent.status.state': 'status',
		'agent.npc.flag': 'is_npc',
		'agent.progression.level': 'level',
		'agent.progression.xp.current': 'xp',
		'agent.progression.xp.to_level': 'xp_to_level',
		'agent.progression.tier': 'tier',
		'agent.progression.death_count': 'death_count',
		'agent.stats.quests_completed': 'stats.quests_completed',
		'agent.stats.quests_failed': 'stats.quests_failed',
		'agent.stats.bosses_defeated': 'stats.bosses_defeated',
		'agent.stats.total_xp_earned': 'stats.total_xp_earned',
		'agent.lifecycle.created_at': 'created_at',
		'agent.lifecycle.updated_at': 'updated_at',
		'agent.skill.{tag}.level': 'skill_proficiencies[tag].level',
		'agent.skill.{tag}.total_xp': 'skill_proficiencies[tag].total_xp',
		'agent.membership.guild': 'guild_id'
	};

	const AGENT_IGNORED: Record<string, string> = {
		'agent.assignment.quest': 'current_quest not displayed in list view',
		'agent.membership.party': 'current_party not displayed in list view',
		'agent.status.cooldown_until': 'cooldown UI not yet built'
	};

	// -------------------------------------------------------------------------
	// QUEST
	// -------------------------------------------------------------------------

	const QUEST_HANDLED: Record<string, string> = {
		'quest.identity.name': 'name',
		'quest.identity.title': 'title',
		'quest.identity.description': 'description',
		'quest.status.state': 'status',
		'quest.difficulty.level': 'difficulty',
		'quest.tier.minimum': 'min_tier',
		'quest.party.required': 'party_required',
		'quest.party.min_size': 'min_party_size',
		'quest.xp.base': 'base_xp',
		'quest.review.level': 'constraints.review_level',
		'quest.lifecycle.posted_at': 'posted_at',
		'quest.attempts.current': 'attempts',
		'quest.attempts.max': 'max_attempts',
		'quest.execution.loop_id': 'loop_id',
		'quest.classification.type': 'quest_type'
	};

	const QUEST_IGNORED: Record<string, string> = {
		'quest.assignment.agent': 'claimed_by not shown in quest card',
		'quest.assignment.party': 'party assignment not shown in quest card',
		'quest.lifecycle.claimed_at': 'lifecycle timestamps not displayed',
		'quest.lifecycle.started_at': 'lifecycle timestamps not displayed',
		'quest.lifecycle.completed_at': 'lifecycle timestamps not displayed',
		'quest.parent.quest': 'quest hierarchy not yet displayed',
		'quest.priority.guild': 'guild priority not displayed',
		'quest.review.needs_review': 'backend constraint not displayed in quest card',
		'quest.skill.required': 'required_skills populated from separate source',
		'quest.tool.required': 'required_tools populated from separate source'
	};

	// -------------------------------------------------------------------------
	// GUILD
	// -------------------------------------------------------------------------

	const GUILD_HANDLED: Record<string, string> = {
		'guild.identity.name': 'name',
		'guild.identity.description': 'description',
		'guild.identity.culture': 'culture',
		'guild.identity.motto': 'motto',
		'guild.status.state': 'status',
		'guild.config.max_members': 'max_members',
		'guild.config.min_level': 'min_level',
		'guild.founding.date': 'founded',
		'guild.founding.agent': 'founded_by',
		'guild.stats.reputation': 'reputation',
		'guild.stats.quests_handled': 'quests_handled',
		'guild.stats.success_rate': 'success_rate',
		'guild.stats.quests_failed': 'quests_failed',
		'guild.lifecycle.created_at': 'created_at',
		'guild.routing.quest_type': 'quest_types[]'
	};

	const GUILD_IGNORED: Record<string, string> = {
		'guild.membership.agent': 'member list populated from separate source',
		'guild.member.{id}.rank': 'member details not in guild card',
		'guild.member.{id}.contribution': 'member details not in guild card',
		'guild.resource.tool': 'shared_tools not displayed in guild card',
		'guild.routing.preferred_client': 'client routing not displayed'
	};

	// -------------------------------------------------------------------------
	// BATTLE
	// -------------------------------------------------------------------------

	const BATTLE_HANDLED: Record<string, string> = {
		'battle.identity.name': 'name',
		'battle.assignment.quest': 'quest_id',
		'battle.assignment.agent': 'agent_id',
		'battle.status.state': 'status',
		'battle.review.level': 'level',
		'battle.lifecycle.started_at': 'started_at',
		'battle.verdict.level_change': 'verdict.level_change'
	};

	const BATTLE_IGNORED: Record<string, string> = {
		'battle.lifecycle.completed_at': 'battle detail view not yet built',
		'battle.verdict.passed': 'verdict display not yet built',
		'battle.verdict.quality_score': 'verdict display not yet built',
		'battle.verdict.xp_awarded': 'verdict display not yet built',
		'battle.verdict.xp_penalty': 'verdict display not yet built',
		'battle.verdict.feedback': 'verdict display not yet built',
		'battle.criteria.count': 'criteria count not displayed',
		'battle.judge.0.id': 'judge list not displayed',
		'battle.judge.0.type': 'judge list not displayed',
		'battle.judge.1.id': 'judge list not displayed',
		'battle.judge.1.type': 'judge list not displayed'
	};

	// -------------------------------------------------------------------------
	// PARTY
	// -------------------------------------------------------------------------

	const PARTY_HANDLED: Record<string, string> = {
		'party.identity.name': 'name',
		'party.status.state': 'status',
		'party.quest': 'quest_id',
		'party.lead': 'lead',
		'party.strategy': 'strategy',
		'party.lifecycle.formed_at': 'formed_at'
	};

	const PARTY_IGNORED: Record<string, string> = {
		'party.lifecycle.disbanded_at': 'disbanded timestamp not displayed',
		'party.membership.member': 'member list populated from separate source',
		'party.membership.count': 'member count derived from member list',
		'party.member.{id}.role': 'member roles not in party card',
		'party.assignment.{id}': 'sub-quest assignments not in party card',
		'party.context.count': 'shared context count not displayed',
		'party.results.count': 'sub-results count not displayed'
	};

	// -------------------------------------------------------------------------
	// Test helper
	// -------------------------------------------------------------------------

	function allPredicates(entity: { static: string[]; dynamic: string[]; optional: string[]; multi_value: string[] }): string[] {
		return [...entity.static, ...entity.dynamic, ...entity.optional, ...entity.multi_value];
	}

	function validateEntity(
		name: string,
		entitySchema: { static: string[]; dynamic: string[]; optional: string[]; multi_value: string[] },
		handled: Record<string, string>,
		ignored: Record<string, string>
	) {
		const predicates = allPredicates(entitySchema);

		it(`every ${name} predicate is accounted for`, () => {
			const unaccounted: string[] = [];
			for (const pred of predicates) {
				if (!handled[pred] && !ignored[pred]) {
					unaccounted.push(pred);
				}
			}
			expect(unaccounted, `Unhandled ${name} predicates — add to HANDLED or IGNORED`).toEqual([]);
		});

		it(`no stale ${name} entries in HANDLED`, () => {
			const stale: string[] = [];
			for (const pred of Object.keys(handled)) {
				if (!predicates.includes(pred)) {
					stale.push(pred);
				}
			}
			expect(stale, `Stale ${name} entries in HANDLED — predicate no longer in Go schema`).toEqual([]);
		});

		it(`no stale ${name} entries in IGNORED`, () => {
			const stale: string[] = [];
			for (const pred of Object.keys(ignored)) {
				if (!predicates.includes(pred)) {
					stale.push(pred);
				}
			}
			expect(stale, `Stale ${name} entries in IGNORED — predicate no longer in Go schema`).toEqual([]);
		});
	}

	validateEntity('agent', schema.entities.agent, AGENT_HANDLED, AGENT_IGNORED);
	validateEntity('quest', schema.entities.quest, QUEST_HANDLED, QUEST_IGNORED);
	validateEntity('guild', schema.entities.guild, GUILD_HANDLED, GUILD_IGNORED);
	validateEntity('battle', schema.entities.battle, BATTLE_HANDLED, BATTLE_IGNORED);
	validateEntity('party', schema.entities.party, PARTY_HANDLED, PARTY_IGNORED);
});
