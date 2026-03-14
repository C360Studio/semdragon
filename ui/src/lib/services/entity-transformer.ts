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
	GuildMember,
	PartyMember,
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

function asStringArray(v: unknown): string[] | undefined {
	if (!Array.isArray(v)) return undefined;
	return v.filter((item): item is string => typeof item === 'string');
}

/**
 * Collect indexed triples like "prefix.N.field" into an ordered array.
 * The apply callback populates fields on each item by index.
 */
function collectIndexed<T extends Record<string, unknown>>(
	triples: Triple[],
	prefix: string,
	apply: (parts: string[], obj: unknown, item: T) => void
): T[] {
	const byIndex = new Map<number, T>();
	for (const t of triples) {
		if (!t.predicate.startsWith(prefix)) continue;
		const parts = t.predicate.split('.');
		if (parts.length !== 4) continue;
		const idx = parseInt(parts[2], 10);
		if (isNaN(idx)) continue;
		if (!byIndex.has(idx)) byIndex.set(idx, {} as T);
		apply(parts, t.object, byIndex.get(idx)!);
	}
	if (byIndex.size === 0) return [];
	const maxIdx = Math.max(...byIndex.keys());
	const result: T[] = [];
	for (let i = 0; i <= maxIdx; i++) {
		if (byIndex.has(i)) result.push(byIndex.get(i)!);
	}
	return result;
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

	// Extract single guild membership
	const guildRaw = m.get('agent.membership.guild');
	const guild_id = guildRaw ? guildId(str(guildRaw)) : null;

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
		peer_review_count: num(m.get('agent.stats.peer_review_count')),
		peer_review_q1_avg: 0,
		peer_review_q2_avg: 0,
		peer_review_q3_avg: 0
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
		guild_id,
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
		parent_quest: m.has('quest.parent.quest') ? questId(str(m.get('quest.parent.quest'))) : undefined,
		sub_quests: [],
		decomposed_by: m.has('quest.decomposed_by') ? agentId(str(m.get('quest.decomposed_by'))) : undefined,
		party_id: m.has('quest.assignment.party') ? partyId(str(m.get('quest.assignment.party'))) : undefined,
		posted_at: str(m.get('quest.lifecycle.posted_at')),
		attempts: num(m.get('quest.attempts.current')),
		max_attempts: num(m.get('quest.attempts.max'), 3),
		escalated: false,
		loop_id: str(m.get('quest.execution.loop_id')),
		context_token_count: m.has('quest.context.token_count')
			? num(m.get('quest.context.token_count'))
			: undefined,
		context_sources: asStringArray(m.get('quest.context.sources')),
		context_entities: asStringArray(m.get('quest.context.entities')),
		verdict: m.has('quest.verdict.passed')
			? {
					passed: m.get('quest.verdict.passed') === true || m.get('quest.verdict.passed') === 'true',
					quality_score: num(m.get('quest.verdict.score')),
					xp_awarded: num(m.get('quest.verdict.xp_awarded')),
					xp_penalty: 0,
					level_change: 0,
					feedback: str(m.get('quest.verdict.feedback'))
				}
			: undefined
	};
}

// =============================================================================
// GUILD TRANSFORMER
// =============================================================================

function transformGuild(key: string, entity: GraphEntity): Guild {
	const m = tripleMap(entity.triples);

	const questTypes = tripleValues(entity.triples, 'guild.routing.quest_type').map((v) => str(v));
	const sharedTools = tripleValues(entity.triples, 'guild.resource.tool').map((v) => str(v));

	// Extract members from guild.membership.agent triples
	const memberAgentIds = tripleValues(entity.triples, 'guild.membership.agent').map((v) => str(v));
	const members: GuildMember[] = memberAgentIds.map((aid) => ({
		agent_id: agentId(aid) as unknown as AgentID,
		rank: (str(m.get(`guild.member.${aid}.rank`), 'member') as GuildMember['rank']),
		contribution: num(m.get(`guild.member.${aid}.contribution`)),
		joined_at: str(m.get(`guild.member.${aid}.joined_at`))
	}));

	return {
		id: guildId(key),
		name: str(m.get('guild.identity.name'), 'Unknown Guild'),
		description: str(m.get('guild.identity.description')),
		status: str(m.get('guild.status.state'), 'active') as GuildStatus,
		members,
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
		shared_tools: sharedTools,
		quest_types: questTypes.length > 0 ? questTypes : undefined,
		quorum_size: num(m.get('guild.config.quorum_size'), 3),
		created_at: str(m.get('guild.lifecycle.created_at'))
	};
}

// =============================================================================
// BATTLE TRANSFORMER
// =============================================================================

function transformBattle(key: string, entity: GraphEntity): BossBattle {
	const m = tripleMap(entity.triples);

	const loopIdRaw = str(m.get('battle.execution.loop_id'));
	const completedAt = str(m.get('battle.lifecycle.completed_at'));

	// Extract indexed criteria: battle.criteria.N.{name,description,weight,threshold}
	const criteria = collectIndexed<BossBattle['criteria'][0]>(entity.triples, 'battle.criteria.', (parts, obj, item) => {
		switch (parts[3]) {
			case 'name': item.name = str(obj); break;
			case 'description': item.description = str(obj); break;
			case 'weight': item.weight = num(obj); break;
			case 'threshold': item.threshold = num(obj); break;
		}
	});

	// Extract indexed results: battle.result.N.{criterion_name,judge_id,score,passed,reasoning}
	const results = collectIndexed<NonNullable<BossBattle['results']>[0]>(entity.triples, 'battle.result.', (parts, obj, item) => {
		switch (parts[3]) {
			case 'criterion_name': item.criterion_name = str(obj); break;
			case 'judge_id': item.judge_id = str(obj); break;
			case 'score': item.score = num(obj); break;
			case 'passed': item.passed = obj === true || obj === 'true'; break;
			case 'reasoning': item.reasoning = str(obj); break;
		}
	});

	// Extract indexed judges: battle.judge.N.{id,type}
	const judges = collectIndexed<BossBattle['judges'][0]>(entity.triples, 'battle.judge.', (parts, obj, item) => {
		switch (parts[3]) {
			case 'id': item.id = str(obj); break;
			case 'type': item.type = str(obj); break;
		}
	});

	// Extract verdict if any verdict predicates exist
	let verdict: BossBattle['verdict'] = undefined;
	if (m.has('battle.verdict.passed')) {
		const passed = m.get('battle.verdict.passed');
		verdict = {
			passed: passed === true || passed === 'true',
			quality_score: num(m.get('battle.verdict.quality_score')),
			xp_awarded: num(m.get('battle.verdict.xp_awarded')),
			xp_penalty: num(m.get('battle.verdict.xp_penalty')),
			level_change: num(m.get('battle.verdict.level_change')),
			feedback: str(m.get('battle.verdict.feedback'))
		};
	}

	return {
		id: battleId(key),
		quest_id: questId(str(m.get('battle.assignment.quest'))),
		agent_id: agentId(str(m.get('battle.assignment.agent'))),
		level: num(m.get('battle.review.level')) as ReviewLevel,
		status: str(m.get('battle.status.state'), 'active') as BossBattle['status'],
		criteria,
		results: results.length > 0 ? results : undefined,
		judges,
		verdict,
		started_at: str(m.get('battle.lifecycle.started_at')),
		completed_at: completedAt || undefined,
		loop_id: loopIdRaw || undefined
	};
}

// =============================================================================
// PARTY TRANSFORMER (minimal — parties not yet in SSE seed data)
// =============================================================================

function transformParty(key: string, entity: GraphEntity): Party {
	const m = tripleMap(entity.triples);

	// Extract members from party.membership.member triples
	const memberAgentIds = tripleValues(entity.triples, 'party.membership.member').map((v) => str(v));
	const members: PartyMember[] = memberAgentIds.map((aid) => ({
		agent_id: agentId(aid) as unknown as AgentID,
		role: str(m.get(`party.member.${aid}.role`), 'executor') as PartyMember['role'],
		skills: [],
		joined_at: ''
	}));

	// Extract sub-quest assignments from party.assignment.{questID} triples
	const subQuestMap: Record<string, string> = {};
	for (const triple of entity.triples) {
		const match = triple.predicate.match(/^party\.assignment\.(.+)$/);
		if (match) {
			subQuestMap[match[1]] = str(triple.object);
		}
	}

	return {
		id: partyId(key),
		name: str(m.get('party.identity.name'), 'Unknown Party'),
		status: str(m.get('party.status.state'), 'forming') as Party['status'],
		quest_id: questId(str(m.get('party.quest'))),
		lead: agentId(str(m.get('party.lead'))),
		members,
		strategy: (str(m.get('party.strategy'), 'balanced') as Party['strategy']),
		sub_quest_map: subQuestMap,
		shared_context: [],
		sub_results: {},
		formed_at: str(m.get('party.lifecycle.formed_at'))
	};
}
