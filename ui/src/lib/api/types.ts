/**
 * Semdragons TypeScript Types — Generated + Branded
 *
 * Entity types are derived from Go structs via OpenAPI → openapi-typescript.
 * This wrapper adds branded ID types, display constants, and SSE-only types.
 *
 * Regenerate with: make openapi && cd ui && npm run gen:api
 */

import type { components } from './generated';

// =============================================================================
// BRANDED ID TYPES
// =============================================================================
// TypeScript-only nominal types for compile-time ID safety.

declare const __brand: unique symbol;
type Brand<K, T> = K & { [__brand]: T };

export type AgentID = Brand<string, 'AgentID'>;
export type QuestID = Brand<string, 'QuestID'>;
export type PartyID = Brand<string, 'PartyID'>;
export type GuildID = Brand<string, 'GuildID'>;
export type BattleID = Brand<string, 'BattleID'>;
export type PeerReviewID = Brand<string, 'PeerReviewID'>;

export const agentId = (id: string): AgentID => id as AgentID;
export const questId = (id: string): QuestID => id as QuestID;
export const partyId = (id: string): PartyID => id as PartyID;
export const guildId = (id: string): GuildID => id as GuildID;
export const battleId = (id: string): BattleID => id as BattleID;
export const peerReviewId = (id: string): PeerReviewID => id as PeerReviewID;

// =============================================================================
// RAW GENERATED TYPES — Before branded ID overrides
// =============================================================================

type RawQuest = components['schemas']['Quest'];
type RawAgent = components['schemas']['Agent'];
type RawBossBattle = components['schemas']['BossBattle'];
type RawParty = components['schemas']['Party'];
type RawGuild = components['schemas']['Guild'];
type RawPeerReview = components['schemas']['PeerReview'];
type RawPartyMember = components['schemas']['PartyMember'];
type RawGuildMember = components['schemas']['GuildMember'];
type RawReviewSubmission = components['schemas']['ReviewSubmission'];
type RawActiveEffect = components['schemas']['ActiveEffect'];
type RawAgentInventory = components['schemas']['AgentInventory'];

// =============================================================================
// ENTITY TYPES — Generated schemas with branded ID overrides
// =============================================================================
// ID fields are overridden with branded types for compile-time safety.
// The generated schema uses plain `string`; brands exist only in TypeScript.

export type Quest = Omit<
	RawQuest,
	'id' | 'claimed_by' | 'party_id' | 'guild_priority' | 'parent_quest' | 'sub_quests' | 'decomposed_by'
> & {
	id: QuestID;
	claimed_by?: AgentID | null;
	party_id?: PartyID | null;
	guild_priority?: GuildID | null;
	parent_quest?: QuestID | null;
	sub_quests?: QuestID[];
	decomposed_by?: AgentID | null;
	// Context metadata — populated by questbridge after prompt assembly.
	context_token_count?: number;
	context_sources?: string[];
	context_entities?: string[];
};

export type Agent = Omit<RawAgent, 'id' | 'current_quest' | 'current_party' | 'guilds'> & {
	id: AgentID;
	current_quest?: QuestID | null;
	current_party?: PartyID | null;
	guild_id?: GuildID | null;
};

export type BossBattle = Omit<RawBossBattle, 'id' | 'quest_id' | 'agent_id'> & {
	id: BattleID;
	quest_id: QuestID;
	agent_id: AgentID;
	loop_id?: string;
};

export type Party = Omit<RawParty, 'id' | 'quest_id' | 'lead' | 'members'> & {
	id: PartyID;
	quest_id: QuestID;
	lead: AgentID;
	members: PartyMember[];
};

export type Guild = Omit<RawGuild, 'id' | 'founded_by' | 'members'> & {
	id: GuildID;
	founded_by: AgentID;
	members: GuildMember[];
};

export type PeerReview = Omit<
	RawPeerReview,
	'id' | 'quest_id' | 'party_id' | 'leader_id' | 'member_id'
> & {
	id: PeerReviewID;
	quest_id: QuestID;
	party_id?: PartyID | null;
	leader_id: AgentID;
	member_id: AgentID;
};

export type PartyMember = Omit<RawPartyMember, 'agent_id'> & {
	agent_id: AgentID;
};

export type GuildMember = Omit<RawGuildMember, 'agent_id'> & {
	agent_id: AgentID;
};

export type ReviewSubmission = Omit<RawReviewSubmission, 'reviewer_id' | 'reviewee_id'> & {
	reviewer_id: AgentID;
	reviewee_id: AgentID;
};

export type ActiveEffect = Omit<RawActiveEffect, 'quest_id'> & {
	quest_id?: QuestID | null;
};

export type AgentInventory = Omit<RawAgentInventory, 'agent_id'> & {
	agent_id: AgentID;
};

// Quest subtypes (no ID overrides needed)
export type QuestConstraints = components['schemas']['QuestConstraints'];
export type BattleVerdict = components['schemas']['BattleVerdict'];
export type QuestBrief = components['schemas']['QuestBrief'];
export type QuestChainBrief = components['schemas']['QuestChainBrief'];
export type QuestChainEntry = components['schemas']['QuestChainEntry'];
export type QuestHints = components['schemas']['QuestHints'];

// Agent subtypes
export type AgentStats = components['schemas']['AgentStats'];
export type AgentConfig = components['schemas']['AgentConfig'];
export type AgentPersona = components['schemas']['AgentPersona'];

// Battle subtypes
export type Judge = components['schemas']['Judge'];
export type ReviewCriterion = components['schemas']['ReviewCriterion'];
export type ReviewResult = components['schemas']['ReviewResult'];

// Peer Review subtypes
export type ReviewRatings = components['schemas']['ReviewRatings'];

// Store types
export type StoreItem = components['schemas']['StoreItem'];
export type OwnedItem = components['schemas']['OwnedItem'];
export type ConsumableEffect = components['schemas']['ConsumableEffect'];

// Character sheet
export type CharacterSheet = components['schemas']['CharacterSheet'];
export type SkillBar = components['schemas']['SkillBar'];
export type DerivedStats = components['schemas']['DerivedStats'];
export type GuildMembership = components['schemas']['GuildMembership'];
export type EquippedItem = components['schemas']['EquippedItem'];

// DM types
export type DMChatSession = components['schemas']['DMChatSession'];
export type DMChatTurn = components['schemas']['DMChatTurn'];
export type DMChatResponse = components['schemas']['DMChatResponse'];
export type TraceInfoResponse = components['schemas']['TraceInfoResponse'];

// Token breaker circuit-breaker state
export type TokenBreakerState = 'ok' | 'warning' | 'tripped';

// API response types
export type WorldStateResponse = components['schemas']['WorldStateResponse'];
export type WorldStats = components['schemas']['WorldStats'] & {
	tokens_used_hourly?: number;
	tokens_limit_hourly?: number;
	token_budget_pct?: number;
	token_breaker?: TokenBreakerState;
	cost_used_hourly_usd?: number;
	cost_total_usd?: number;
};
export type PurchaseResponse = components['schemas']['PurchaseResponse'];
export type UseConsumableResponse = components['schemas']['UseConsumableResponse'];

// Request types (for API client typing)
export type CreateQuestRequest = components['schemas']['CreateQuestRequest'];
export type CreateQuestHints = components['schemas']['CreateQuestHints'];
export type ClaimQuestRequest = components['schemas']['ClaimQuestRequest'];
export type SubmitQuestRequest = components['schemas']['SubmitQuestRequest'];
export type FailQuestRequest = components['schemas']['FailQuestRequest'];
export type AbandonQuestRequest = components['schemas']['AbandonQuestRequest'];
export type RecruitAgentRequest = components['schemas']['RecruitAgentRequest'];
export type PurchaseItemRequest = components['schemas']['PurchaseItemRequest'];
export type CreateReviewRequest = components['schemas']['CreateReviewRequest'];
export type SubmitReviewRequest = components['schemas']['SubmitReviewRequest'];
export type DMChatRequest = components['schemas']['DMChatRequest'];
export type DMChatContextRef = components['schemas']['DMChatContextRef'];
export type DMChatHistoryItem = components['schemas']['DMChatHistoryItem'];

// UseConsumableRequest: agent_id is a path param (not in Go body struct),
// but the UI API client needs it for URL construction.
export interface UseConsumableRequest {
	agent_id: AgentID;
	consumable_id: string;
	quest_id?: string;
}

// =============================================================================
// ENUM TYPES — Extracted from generated literal unions for convenience
// =============================================================================

export type AgentStatus = Agent['status'];
export type QuestStatus = Quest['status'];
export type QuestDifficulty = Quest['difficulty'];
export type BattleStatus = BossBattle['status'];
export type PartyStatus = Party['status'];
export type PartyRole = PartyMember['role'];
export type GuildStatus = Guild['status'];
export type GuildRank = GuildMember['rank'];
export type PeerReviewStatus = PeerReview['status'];
export type ReviewDirection = ReviewSubmission['direction'];
export type JudgeType = Judge['type'];
export type FailureType = NonNullable<Quest['failure_type']>;

export type TrustTier = 0 | 1 | 2 | 3 | 4;
export type ReviewLevel = 0 | 1 | 2 | 3;
export type ProficiencyLevel = 1 | 2 | 3 | 4 | 5;

export type SkillTag =
	| 'code_generation'
	| 'code_review'
	| 'data_transformation'
	| 'summarization'
	| 'research'
	| 'planning'
	| 'customer_communications'
	| 'analysis'
	| 'training';

export type ItemType = 'tool' | 'consumable';
export type PurchaseType = 'permanent' | 'rental';

export type ConsumableType =
	| 'retry_token'
	| 'cooldown_skip'
	| 'xp_boost'
	| 'quality_shield'
	| 'insight_scroll';

export type DMMode = 'full_auto' | 'assisted' | 'supervised' | 'manual';
export type PartyStrategy = 'balanced' | 'specialist' | 'mentor' | 'minimal';
export type InterventionType = 'assist' | 'redirect' | 'takeover' | 'abort' | 'augment';

// =============================================================================
// DISPLAY CONSTANTS
// =============================================================================

export const TrustTierNames: Record<number, string> = {
	0: 'Apprentice',
	1: 'Journeyman',
	2: 'Expert',
	3: 'Master',
	4: 'Grandmaster'
};

export const TrustTierLevelRanges: Record<number, [number, number]> = {
	0: [1, 5],
	1: [6, 10],
	2: [11, 15],
	3: [16, 18],
	4: [19, 20]
};

export const QuestDifficultyNames: Record<QuestDifficulty, string> = {
	0: 'Trivial',
	1: 'Easy',
	2: 'Moderate',
	3: 'Hard',
	4: 'Epic',
	5: 'Legendary'
};

export const ReviewLevelNames: Record<number, string> = {
	0: 'Auto',
	1: 'Standard',
	2: 'Strict',
	3: 'Human'
};

export const ProficiencyLevelNames: Record<ProficiencyLevel, string> = {
	1: 'Novice',
	2: 'Apprentice',
	3: 'Journeyman',
	4: 'Expert',
	5: 'Master'
};

export const ConsumableTypeNames: Record<ConsumableType, string> = {
	retry_token: 'Retry Token',
	cooldown_skip: 'Cooldown Skip',
	xp_boost: 'XP Boost',
	quality_shield: 'Quality Shield',
	insight_scroll: 'Insight Scroll'
};

export const ConsumableTypeDescriptions: Record<ConsumableType, string> = {
	retry_token: 'Retry a failed quest without penalty',
	cooldown_skip: 'Clear cooldown immediately',
	xp_boost: 'Earn 2x XP on your next quest',
	quality_shield: 'Ignore one failed review criterion',
	insight_scroll: 'See difficulty hints before claiming'
};

// =============================================================================
// UTILITY FUNCTIONS
// =============================================================================

export function tierFromLevel(level: number): TrustTier {
	if (level <= 5) return 0;
	if (level <= 10) return 1;
	if (level <= 15) return 2;
	if (level <= 18) return 3;
	return 4;
}

// =============================================================================
// TOOL REGISTRY — mirrors processor/executor/tools.go RegisterBuiltins
// =============================================================================

export interface ToolInfo {
	name: string;
	description: string;
	min_tier: TrustTier;
	category: string;
}

/** Static tool definitions matching the Go ToolRegistry.RegisterBuiltins(). */
export const BuiltinTools: ToolInfo[] = [
	{ name: 'read_file', description: 'Read the contents of a file', min_tier: 0, category: 'filesystem' },
	{ name: 'list_directory', description: 'List the contents of a directory', min_tier: 0, category: 'filesystem' },
	{ name: 'search_text', description: 'Search for text patterns in files', min_tier: 0, category: 'filesystem' },
	{ name: 'patch_file', description: 'Apply a targeted find-and-replace edit', min_tier: 1, category: 'filesystem' },
	{ name: 'http_request', description: 'Make an HTTP request to a URL', min_tier: 1, category: 'network' },
	{ name: 'write_file', description: 'Write content to a file', min_tier: 2, category: 'filesystem' },
	{ name: 'run_tests', description: 'Run a test command in the workspace', min_tier: 2, category: 'execution' },
	{ name: 'run_command', description: 'Run an arbitrary shell command', min_tier: 3, category: 'execution' },
	{ name: 'decompose_quest', description: 'Break a quest into a DAG of sub-quests', min_tier: 3, category: 'coordination' },
];

/** Returns tools available at a given trust tier. */
export function toolsForTier(tier: number): ToolInfo[] {
	return BuiltinTools.filter((t) => t.min_tier <= tier);
}

// =============================================================================
// COMPOSITE TYPES — Used in the UI but not directly in the OpenAPI spec
// =============================================================================

/** WorldState is a typed view used by the SSE store and world endpoint. */
export interface WorldState {
	agents: Agent[];
	quests: Quest[];
	parties: Party[];
	guilds: Guild[];
	battles: BossBattle[];
	stats: WorldStats;
}

// =============================================================================
// SSE TYPES — Not in Go, only used by the frontend SSE client
// =============================================================================

export type KVOperation = 'create' | 'update' | 'delete' | 'initial_sync_complete';

export interface KVWatchConnectedEvent {
	bucket: string;
	pattern: string;
	message: string;
}

export interface KVChangeEvent {
	bucket: string;
	key: string;
	operation: KVOperation;
	value?: unknown;
	revision: number;
	timestamp: string;
}

export type EntityType = 'quest' | 'agent' | 'battle' | 'party' | 'guild';

// =============================================================================
// GAME EVENTS — SSE event types for real-time UI updates
// =============================================================================

export type GameEventType =
	| 'quest.lifecycle.posted'
	| 'quest.lifecycle.claimed'
	| 'quest.lifecycle.started'
	| 'quest.lifecycle.submitted'
	| 'quest.lifecycle.completed'
	| 'quest.lifecycle.failed'
	| 'quest.lifecycle.escalated'
	| 'quest.lifecycle.abandoned'
	| 'agent.progression.xp'
	| 'agent.progression.levelup'
	| 'agent.progression.leveldown'
	| 'agent.progression.death'
	| 'agent.status.idle'
	| 'agent.status.active'
	| 'agent.autonomy.cooldown'
	| 'battle.review.started'
	| 'battle.review.verdict'
	| 'battle.review.victory'
	| 'battle.review.defeat'
	| 'party.formation.created'
	| 'party.formation.disbanded'
	| 'guild.membership.joined'
	| 'guild.membership.promoted'
	| 'dm.intervention'
	| 'dm.escalation'
	| 'dm.session_start'
	| 'dm.session_end'
	| 'store.item.purchased'
	| 'store.consumable.used'
	| 'agent.inventory.updated'
	| 'review.lifecycle.pending'
	| 'review.lifecycle.submitted'
	| 'review.lifecycle.completed';

export interface GameEvent {
	type: GameEventType;
	timestamp: number;
	session_id: string;
	data: unknown;
	quest_id?: QuestID;
	agent_id?: AgentID;
	party_id?: PartyID;
	guild_id?: GuildID;
	battle_id?: BattleID;
	span_id: string;
}

export interface EventFilter {
	types?: GameEventType[];
	quest_id?: QuestID;
	agent_id?: AgentID;
	guild_id?: GuildID;
}

// =============================================================================
// API-ONLY TYPES — Used in api.ts but not part of entity schemas
// =============================================================================

export interface Intervention {
	type: InterventionType;
	reason: string;
	payload?: unknown;
}

export interface TokenStats {
	hourly_usage: {
		prompt_tokens: number;
		completion_tokens: number;
		total_tokens: number;
		estimated_cost_usd: number;
	};
	total_usage: {
		prompt_tokens: number;
		completion_tokens: number;
		total_tokens: number;
		estimated_cost_usd: number;
	};
	hourly_limit: number;
	budget_pct: number;
	breaker: TokenBreakerState;
	hourly_epoch: number;
	hourly_cost_usd: number;
	total_cost_usd: number;
}

export interface Trajectory {
	loop_id: string;
	start_time: string;
	end_time?: string;
	steps: TrajectoryStep[];
	outcome?: string;
	total_tokens_in: number;
	total_tokens_out: number;
	duration: number;
}

export interface TrajectoryStep {
	timestamp: string;
	step_type: 'model_call' | 'tool_call';
	request_id?: string;
	prompt?: string;
	response?: string;
	tokens_in?: number;
	tokens_out?: number;
	tool_name?: string;
	tool_arguments?: Record<string, unknown>;
	tool_result?: string;
	duration: number;
	messages?: ChatMessage[];
	tool_calls?: ToolCallRef[];
	model?: string;
}

export interface ChatMessage {
	role: string;
	content?: string;
	name?: string;
	reasoning_content?: string;
	tool_calls?: ToolCallRef[];
	tool_call_id?: string;
}

export interface ToolCallRef {
	id: string;
	name: string;
	arguments?: Record<string, unknown>;
	metadata?: Record<string, unknown>;
	loop_id?: string;
	trace_id?: string;
}

export interface SkillProficiency {
	level: ProficiencyLevel;
	progress: number;
	total_xp: number;
	quests_used: number;
	last_used?: string;
}

/** Legacy alias for CharacterSheetMembership in some components */
export type CharacterSheetMembership = GuildMembership;

/** Tool interface used by Agent.equipment */
export interface Tool {
	id: string;
	name: string;
	description: string;
	min_tier: TrustTier;
	category: string;
	dangerous: boolean;
	config: Record<string, unknown>;
}

export interface ContextItem {
	key: string;
	value: unknown;
	added_by: AgentID;
	added_at: string;
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

// Chat types used by api.ts and chatStore
export interface ChatHistoryMessage {
	role: 'user' | 'dm';
	content: string;
}

export interface ChatContextRef {
	type: string;
	id: string;
}

export interface TraceInfo {
	trace_id?: string;
	span_id?: string;
	parent_span_id?: string;
}

export type ChatMode = 'converse' | 'quest';

export interface ChatResponseHints {
	party_required?: boolean;
	min_party_size?: number;
	require_human_review?: boolean;
	review_level?: number;
	prefer_guild?: string;
	budget?: number;
	deadline?: string;
}

export interface ChatResponse {
	message: string;
	mode: ChatMode;
	quest_brief?: {
		title: string;
		description?: string;
		difficulty?: number;
		skills?: string[];
		acceptance?: string[];
		hints?: ChatResponseHints;
	};
	quest_chain?: {
		quests: Array<{
			title: string;
			description?: string;
			difficulty?: number;
			skills?: string[];
			acceptance?: string[];
			depends_on?: number[];
			hints?: ChatResponseHints;
		}>;
	};
	session_id?: string;
	trace_info?: TraceInfo;
}

export interface PurchaseRequest {
	agent_id: AgentID;
	item_id: string;
}

/** Subset of SkillImprovementResult for event display */
export interface SkillImprovementResult {
	skill: SkillTag;
	points_earned: number;
	old_level: ProficiencyLevel;
	new_level: ProficiencyLevel;
	old_progress: number;
	new_progress: number;
	leveled_up: boolean;
	at_max_level: boolean;
}

/** Library entry in a guild's knowledge base */
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

/** Agent evaluation result from DM */
export interface AgentEvaluation {
	agent_id: AgentID;
	current_level: number;
	recommended_level: number;
	strengths: string[];
	weaknesses: string[];
	recommendation: 'promote' | 'maintain' | 'demote' | 'retire';
}

/** Escalation result from DM */
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

/** Board play/pause status */
export interface BoardStatus {
	paused: boolean;
	paused_at: string | null;
	paused_by: string | null;
}
