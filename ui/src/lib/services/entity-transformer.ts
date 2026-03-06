/**
 * Entity Transformer — converts graph triple format to flat TypeScript types.
 *
 * The SSE endpoint streams entities as {id, triples: [...], message_type, version}.
 * The worldStore expects flat domain objects (Agent, Quest, Guild, etc.).
 * This module bridges the two formats.
 */

import type {
	Agent,
	AgentID,
	AgentStats,
	Quest,
	QuestID,
	QuestStatus,
	QuestDifficulty,
	ReviewLevel,
	Guild,
	GuildID,
	GuildStatus,
	BossBattle,
	BattleID,
	Party,
	PartyID,
	TrustTier,
	SkillTag,
	SkillProficiency,
	ProficiencyLevel,
	AgentStatus,
	EntityType
} from '$types';
import { agentId, questId, guildId, battleId, partyId, tierFromLevel } from '$types';

// =============================================================================
// TRIPLE FORMAT (from SSE)
// =============================================================================

interface Triple {
	subject: string;
	predicate: string;
	object: unknown;
	source: string;
	timestamp: string;
	confidence: number;
}

interface GraphEntity {
	id: string;
	triples: Triple[];
	message_type?: { domain: string; category: string; version: string };
	version?: number;
	updated_at?: string;
}

// =============================================================================
// GENERIC TRIPLE HELPERS
// =============================================================================

/** Build a lookup map from predicate → object for quick access. */
function tripleMap(triples: Triple[]): Map<string, unknown> {
	const map = new Map<string, unknown>();
	for (const t of triples) {
		map.set(t.predicate, t.object);
	}
	return map;
}

/** Collect all values for predicates matching a prefix. */
function tripleValues(triples: Triple[], prefix: string): unknown[] {
	return triples.filter((t) => t.predicate.startsWith(prefix)).map((t) => t.object);
}

function str(v: unknown, fallback = ''): string {
	return typeof v === 'string' ? v : fallback;
}

function num(v: unknown, fallback = 0): number {
	return typeof v === 'number' ? v : fallback;
}

// =============================================================================
// PUBLIC API
// =============================================================================

/** Check if the SSE value is in graph entity format (has triples array). */
export function isGraphEntity(value: unknown): value is GraphEntity {
	return (
		typeof value === 'object' &&
		value !== null &&
		'triples' in value &&
		Array.isArray((value as GraphEntity).triples)
	);
}

/** Transform an SSE value into the correct flat entity type. */
export function transformEntity(
	entityType: EntityType,
	key: string,
	value: unknown
): Agent | Quest | Guild | BossBattle | Party | null {
	if (!isGraphEntity(value)) {
		// Already a flat entity (or unknown format) — pass through
		return value as Agent | Quest | Guild | BossBattle | Party;
	}

	switch (entityType) {
		case 'agent':
			return transformAgent(key, value);
		case 'quest':
			return transformQuest(key, value);
		case 'guild':
			return transformGuild(key, value);
		case 'battle':
			return transformBattle(key, value);
		case 'party':
			return transformParty(key, value);
		default:
			return null;
	}
}

// =============================================================================
// AGENT TRANSFORMER
// =============================================================================

function transformAgent(key: string, entity: GraphEntity): Agent {
	const m = tripleMap(entity.triples);

	const level = num(m.get('agent.progression.level'), 1);
	const tier = (m.has('agent.progression.tier')
		? num(m.get('agent.progression.tier'))
		: tierFromLevel(level)) as TrustTier;

	// Extract skill proficiencies from agent.skill.{tag}.level / agent.skill.{tag}.total_xp
	const skillProfs: Record<string, SkillProficiency> = {};
	for (const triple of entity.triples) {
		const match = triple.predicate.match(/^agent\.skill\.(.+)\.level$/);
		if (match) {
			const tag = match[1];
			const xpTriple = entity.triples.find((t) => t.predicate === `agent.skill.${tag}.total_xp`);
			skillProfs[tag] = {
				level: num(triple.object, 1) as ProficiencyLevel,
				progress: 50,
				total_xp: xpTriple ? num(xpTriple.object) : 0,
				quests_used: 0
			};
		}
	}

	// Extract guild memberships
	const guilds = tripleValues(entity.triples, 'agent.membership.guild').map((v) =>
		guildId(str(v))
	);

	const stats: AgentStats = {
		quests_completed: num(m.get('agent.stats.quests_completed')),
		quests_failed: num(m.get('agent.stats.quests_failed')),
		bosses_defeated: num(m.get('agent.stats.bosses_defeated')),
		bosses_failed: 0,
		total_xp_earned: num(m.get('agent.stats.total_xp_earned')),
		total_xp_spent: 0,
		avg_quality_score: 0,
		avg_efficiency: 0,
		parties_led: 0,
		quests_decomposed: 0,
		peer_review_avg: num(m.get('agent.stats.peer_review_avg')),
		peer_review_count: num(m.get('agent.stats.peer_review_count'))
	};

	return {
		id: agentId(key),
		name: str(m.get('agent.identity.name'), key),
		display_name: str(m.get('agent.identity.display_name')),
		status: str(m.get('agent.status.state'), 'idle') as AgentStatus,
		level,
		xp: num(m.get('agent.progression.xp.current')),
		xp_to_level: num(m.get('agent.progression.xp.to_level'), 100),
		death_count: num(m.get('agent.progression.death_count')),
		tier,
		equipment: [],
		skill_proficiencies: skillProfs as Record<SkillTag, SkillProficiency>,
		guilds,
		stats,
		config: { provider: '', model: '', system_prompt: '', temperature: 0, max_tokens: 0, metadata: {} },
		is_npc: m.get('agent.npc.flag') === true,
		total_spent: 0,
		created_at: str(m.get('agent.lifecycle.created_at')),
		updated_at: str(m.get('agent.lifecycle.updated_at'))
	};
}

// =============================================================================
// QUEST TRANSFORMER
// =============================================================================

function transformQuest(key: string, entity: GraphEntity): Quest {
	const m = tripleMap(entity.triples);

	const claimedByRaw = m.get('quest.assignment.agent');
	const claimedBy = typeof claimedByRaw === 'string' && claimedByRaw ? agentId(claimedByRaw) : undefined;

	return {
		id: questId(key),
		title: str(m.get('quest.identity.title'), 'Untitled Quest'),
		description: str(m.get('quest.identity.description')),
		status: str(m.get('quest.status.state'), 'posted') as QuestStatus,
		difficulty: num(m.get('quest.difficulty.level')) as QuestDifficulty,
		required_skills: [],
		required_tools: [],
		min_tier: num(m.get('quest.tier.minimum')) as TrustTier,
		party_required: m.get('quest.party.required') === true,
		min_party_size: 0,
		base_xp: num(m.get('quest.xp.base'), 100),
		bonus_xp: 0,
		guild_xp: 0,
		input: null,
		output: null,
		constraints: {
			max_duration: 0,
			max_cost: 0,
			max_tokens: 0,
			require_review: false,
			review_level: num(m.get('quest.review.level')) as ReviewLevel
		},
		claimed_by: claimedBy,
		claimed_at: str(m.get('quest.lifecycle.claimed_at')) || undefined,
		started_at: str(m.get('quest.lifecycle.started_at')) || undefined,
		completed_at: str(m.get('quest.lifecycle.completed_at')) || undefined,
		parent_quest: undefined,
		sub_quests: [],
		posted_at: str(m.get('quest.lifecycle.posted_at')),
		attempts: num(m.get('quest.attempts.current')),
		max_attempts: num(m.get('quest.attempts.max'), 3),
		escalated: false,
		loop_id: str(m.get('quest.execution.loop_id'))
	};
}

// =============================================================================
// GUILD TRANSFORMER
// =============================================================================

function transformGuild(key: string, entity: GraphEntity): Guild {
	const m = tripleMap(entity.triples);

	const questTypes = tripleValues(entity.triples, 'guild.routing.quest_type').map((v) => str(v));

	return {
		id: guildId(key),
		name: str(m.get('guild.identity.name'), 'Unknown Guild'),
		description: str(m.get('guild.identity.description')),
		status: str(m.get('guild.status.state'), 'active') as GuildStatus,
		members: [],
		max_members: num(m.get('guild.config.max_members'), 50),
		min_level: num(m.get('guild.config.min_level'), 1),
		founded: str(m.get('guild.founding.date')),
		founded_by: agentId(str(m.get('guild.founding.agent'))) as unknown as AgentID,
		culture: str(m.get('guild.identity.culture')),
		motto: str(m.get('guild.identity.motto')),
		reputation: num(m.get('guild.stats.reputation')),
		quests_handled: num(m.get('guild.stats.quests_handled')),
		success_rate: num(m.get('guild.stats.success_rate')),
		quests_failed: num(m.get('guild.stats.quests_failed')),
		shared_tools: [],
		quest_types: questTypes.length > 0 ? questTypes : undefined,
		created_at: str(m.get('guild.lifecycle.created_at'))
	};
}

// =============================================================================
// BATTLE TRANSFORMER (minimal — battles not yet in SSE seed data)
// =============================================================================

function transformBattle(key: string, entity: GraphEntity): BossBattle {
	const m = tripleMap(entity.triples);

	return {
		id: battleId(key),
		quest_id: questId(str(m.get('battle.assignment.quest'))),
		agent_id: agentId(str(m.get('battle.assignment.agent'))),
		level: num(m.get('battle.review.level')) as ReviewLevel,
		status: str(m.get('battle.status.state'), 'active') as BossBattle['status'],
		criteria: [],
		results: [],
		judges: [],
		started_at: str(m.get('battle.lifecycle.started_at'))
	};
}

// =============================================================================
// PARTY TRANSFORMER (minimal — parties not yet in SSE seed data)
// =============================================================================

function transformParty(key: string, entity: GraphEntity): Party {
	const m = tripleMap(entity.triples);

	return {
		id: partyId(key),
		name: str(m.get('party.identity.name'), 'Unknown Party'),
		status: str(m.get('party.status.state'), 'forming') as Party['status'],
		quest_id: questId(str(m.get('party.quest'))),
		lead: agentId(str(m.get('party.lead'))),
		members: [],
		strategy: (str(m.get('party.strategy'), 'balanced') as Party['strategy']),
		sub_quest_map: {},
		shared_context: [],
		sub_results: {},
		formed_at: str(m.get('party.lifecycle.formed_at'))
	};
}
