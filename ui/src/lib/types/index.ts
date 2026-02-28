/**
 * Semdragons TypeScript Types
 *
 * Mirrors Go domain types from semdragons package.
 * Uses branded types for type-safe IDs.
 */

// =============================================================================
// BRANDED ID TYPES
// =============================================================================

declare const __brand: unique symbol;
type Brand<K, T> = K & { [__brand]: T };

export type AgentID = Brand<string, 'AgentID'>;
export type QuestID = Brand<string, 'QuestID'>;
export type PartyID = Brand<string, 'PartyID'>;
export type GuildID = Brand<string, 'GuildID'>;
export type BattleID = Brand<string, 'BattleID'>;

// Helper functions to create branded IDs
export const agentId = (id: string): AgentID => id as AgentID;
export const questId = (id: string): QuestID => id as QuestID;
export const partyId = (id: string): PartyID => id as PartyID;
export const guildId = (id: string): GuildID => id as GuildID;
export const battleId = (id: string): BattleID => id as BattleID;

// =============================================================================
// AGENT
// =============================================================================

export type AgentStatus = 'idle' | 'on_quest' | 'in_battle' | 'cooldown' | 'retired';

export type TrustTier = 0 | 1 | 2 | 3 | 4;

export const TrustTierNames: Record<TrustTier, string> = {
	0: 'Apprentice',
	1: 'Journeyman',
	2: 'Expert',
	3: 'Master',
	4: 'Grandmaster'
};

export const TrustTierLevelRanges: Record<TrustTier, [number, number]> = {
	0: [1, 5],
	1: [6, 10],
	2: [11, 15],
	3: [16, 18],
	4: [19, 20]
};

export function tierFromLevel(level: number): TrustTier {
	if (level <= 5) return 0;
	if (level <= 10) return 1;
	if (level <= 15) return 2;
	if (level <= 18) return 3;
	return 4;
}

export type SkillTag =
	| 'code_generation'
	| 'code_review'
	| 'data_transformation'
	| 'summarization'
	| 'research'
	| 'planning'
	| 'customer_communications'
	| 'analysis';

export interface AgentStats {
	quests_completed: number;
	quests_failed: number;
	bosses_defeated: number;
	bosses_failed: number;
	total_xp_earned: number;
	avg_quality_score: number;
	avg_efficiency: number;
	parties_led: number;
	quests_decomposed: number;
}

export interface AgentConfig {
	provider: string;
	model: string;
	system_prompt: string;
	temperature: number;
	max_tokens: number;
	metadata: Record<string, string>;
}

export interface Tool {
	id: string;
	name: string;
	description: string;
	min_tier: TrustTier;
	category: string;
	dangerous: boolean;
	config: Record<string, unknown>;
}

export interface Agent {
	id: AgentID;
	name: string;
	status: AgentStatus;
	level: number;
	xp: number;
	xp_to_level: number;
	death_count: number;
	tier: TrustTier;
	equipment: Tool[];
	skills: SkillTag[];
	guilds: GuildID[];
	current_quest?: QuestID;
	current_party?: PartyID;
	cooldown_until?: string; // ISO date string
	stats: AgentStats;
	config: AgentConfig;
	created_at: string;
	updated_at: string;
}

// =============================================================================
// QUEST
// =============================================================================

export type QuestStatus =
	| 'posted'
	| 'claimed'
	| 'in_progress'
	| 'in_review'
	| 'completed'
	| 'failed'
	| 'escalated'
	| 'cancelled';

export type QuestDifficulty = 0 | 1 | 2 | 3 | 4 | 5;

export const QuestDifficultyNames: Record<QuestDifficulty, string> = {
	0: 'Trivial',
	1: 'Easy',
	2: 'Moderate',
	3: 'Hard',
	4: 'Epic',
	5: 'Legendary'
};

export type ReviewLevel = 0 | 1 | 2 | 3;

export const ReviewLevelNames: Record<ReviewLevel, string> = {
	0: 'Auto',
	1: 'Standard',
	2: 'Strict',
	3: 'Human'
};

export interface QuestConstraints {
	max_duration: number; // Duration in nanoseconds (Go time.Duration)
	max_cost: number;
	max_tokens: number;
	require_review: boolean;
	review_level: ReviewLevel;
}

export interface Quest {
	id: QuestID;
	title: string;
	description: string;
	status: QuestStatus;
	difficulty: QuestDifficulty;

	// Requirements
	required_skills: SkillTag[];
	required_tools: string[];
	min_tier: TrustTier;
	party_required: boolean;
	min_party_size: number;

	// Rewards
	base_xp: number;
	bonus_xp: number;
	guild_xp: number;

	// Execution context
	input: unknown;
	output?: unknown;
	constraints: QuestConstraints;

	// Quest chain / decomposition
	parent_quest?: QuestID;
	sub_quests: QuestID[];
	decomposed_by?: AgentID;

	// Assignment
	claimed_by?: AgentID;
	party_id?: PartyID;
	guild_priority?: GuildID;

	// Lifecycle
	posted_at: string;
	claimed_at?: string;
	started_at?: string;
	completed_at?: string;
	deadline?: string;

	// Failure tracking
	attempts: number;
	max_attempts: number;
	escalated: boolean;

	// Observability
	trajectory_id: string;
}

// =============================================================================
// BOSS BATTLE
// =============================================================================

export type BattleStatus = 'active' | 'victory' | 'defeat' | 'retreat';

export type JudgeType = 'automated' | 'llm' | 'human';

export interface Judge {
	id: string;
	type: JudgeType;
	config: Record<string, unknown>;
}

export interface ReviewCriterion {
	name: string;
	description: string;
	weight: number;
	threshold: number;
}

export interface ReviewResult {
	criterion_name: string;
	score: number;
	passed: boolean;
	reasoning: string;
	judge_id: string;
}

export interface BattleVerdict {
	passed: boolean;
	quality_score: number;
	xp_awarded: number;
	xp_penalty: number;
	feedback: string;
	level_change: number;
}

export interface BossBattle {
	id: BattleID;
	quest_id: QuestID;
	agent_id: AgentID;
	level: ReviewLevel;
	status: BattleStatus;
	criteria: ReviewCriterion[];
	results: ReviewResult[];
	verdict?: BattleVerdict;
	judges: Judge[];
	started_at: string;
	completed_at?: string;
}

// =============================================================================
// PARTY
// =============================================================================

export type PartyStatus = 'forming' | 'active' | 'disbanded';

export type PartyRole = 'lead' | 'executor' | 'reviewer' | 'scout';

export interface PartyMember {
	agent_id: AgentID;
	role: PartyRole;
	skills: SkillTag[];
	joined_at: string;
}

export interface ContextItem {
	key: string;
	value: unknown;
	added_by: AgentID;
	added_at: string;
}

export interface Party {
	id: PartyID;
	name: string;
	status: PartyStatus;
	quest_id: QuestID;
	lead: AgentID;
	members: PartyMember[];
	strategy: string;
	sub_quest_map: Record<string, AgentID>;
	shared_context: ContextItem[];
	sub_results: Record<string, unknown>;
	rollup_result?: unknown;
	formed_at: string;
	disbanded_at?: string;
}

// =============================================================================
// GUILD
// =============================================================================

export type GuildStatus = 'active' | 'inactive';

export type GuildRank = 'initiate' | 'member' | 'veteran' | 'officer' | 'guildmaster';

export interface GuildMember {
	agent_id: AgentID;
	rank: GuildRank;
	guild_xp: number;
	joined_at: string;
}

export interface LibraryEntry {
	id: string;
	title: string;
	content: unknown;
	category: string;
	added_by: AgentID;
	use_count: number;
	effectiveness: number;
	added_at: string;
}

export interface Guild {
	id: GuildID;
	name: string;
	description: string;
	status: GuildStatus;
	domain: string;
	skills: SkillTag[];
	quest_types: string[];
	members: GuildMember[];
	max_members: number;
	reputation: number;
	quests_handled: number;
	success_rate: number;
	library: LibraryEntry[];
	shared_tools: string[];
	min_level_to_join: number;
	required_skills: SkillTag[];
	auto_recruit: boolean;
	created_at: string;
}

// =============================================================================
// DUNGEON MASTER & WORLD STATE
// =============================================================================

export type DMMode = 'full_auto' | 'assisted' | 'supervised' | 'manual';

export type PartyStrategy = 'balanced' | 'specialist' | 'mentor' | 'minimal';

export type InterventionType = 'assist' | 'redirect' | 'takeover' | 'abort' | 'augment';

export interface WorldStats {
	active_agents: number;
	idle_agents: number;
	cooldown_agents: number;
	retired_agents: number;
	open_quests: number;
	active_quests: number;
	completion_rate: number;
	avg_quality: number;
	active_parties: number;
	active_guilds: number;
}

export interface WorldState {
	agents: Agent[];
	quests: Quest[];
	parties: Party[];
	guilds: Guild[];
	battles: BossBattle[];
	stats: WorldStats;
}

export interface SessionConfig {
	mode: DMMode;
	name: string;
	description: string;
	dm_model: string;
	max_concurrent: number;
	auto_escalate: boolean;
	trajectory_mode: string;
	metadata: Record<string, string>;
}

export interface Session {
	id: string;
	config: SessionConfig;
	world_state: WorldState;
	active: boolean;
}

// =============================================================================
// GAME EVENTS
// =============================================================================

export type GameEventType =
	| 'quest.posted'
	| 'quest.claimed'
	| 'quest.started'
	| 'quest.completed'
	| 'quest.failed'
	| 'quest.escalated'
	| 'agent.recruited'
	| 'agent.level_up'
	| 'agent.level_down'
	| 'agent.death'
	| 'agent.permadeath'
	| 'agent.revived'
	| 'battle.started'
	| 'battle.victory'
	| 'battle.defeat'
	| 'party.formed'
	| 'party.disbanded'
	| 'guild.created'
	| 'guild.joined'
	| 'dm.intervention'
	| 'dm.escalation'
	| 'dm.session_start'
	| 'dm.session_end';

export interface GameEvent {
	type: GameEventType;
	timestamp: number; // Unix millis
	session_id: string;
	data: unknown;
	quest_id?: QuestID;
	agent_id?: AgentID;
	party_id?: PartyID;
	guild_id?: GuildID;
	battle_id?: BattleID;
	trajectory_id: string;
	span_id: string;
}

export interface EventFilter {
	types?: GameEventType[];
	quest_id?: QuestID;
	agent_id?: AgentID;
	guild_id?: GuildID;
}

// =============================================================================
// API TYPES
// =============================================================================

export interface Intervention {
	type: InterventionType;
	reason: string;
	payload?: unknown;
}

export interface QuestHints {
	suggested_difficulty?: QuestDifficulty;
	suggested_skills?: SkillTag[];
	prefer_guild?: GuildID;
	require_human_review: boolean;
	budget: number;
	deadline?: string;
}

export interface AgentEvaluation {
	agent_id: AgentID;
	current_level: number;
	recommended_level: number;
	strengths: string[];
	weaknesses: string[];
	recommendation: 'promote' | 'maintain' | 'demote' | 'retire';
}

export interface EscalationResult {
	quest_id: QuestID;
	resolution: 'reassigned' | 'completed_by_dm' | 'cancelled';
	new_party_id?: PartyID;
	dm_completed: boolean;
}

export interface SessionSummary {
	session_id: string;
	quests_completed: number;
	quests_failed: number;
	quests_escalated: number;
	agents_active: number;
	total_xp_awarded: number;
	avg_quality: number;
	level_ups: number;
	level_downs: number;
	deaths: number;
}
