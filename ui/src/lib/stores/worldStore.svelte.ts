/**
 * World Store - Central state management for Semdragons
 *
 * Uses Svelte 5 runes for reactive state management.
 * Maintains the complete world state populated via SSE.
 */

import type {
	Agent,
	AgentID,
	Quest,
	QuestID,
	Party,
	PartyID,
	Guild,
	GuildID,
	BossBattle,
	BattleID,
	WorldStats,
	GameEvent,
	StoreItem,
	AgentInventory,
	ActiveEffect,
	TokenBreakerState
} from '$types';

// =============================================================================
// STORE STATE
// =============================================================================

const MAX_RECENT_EVENTS = 100;

const defaultStats: WorldStats = {
	active_agents: 0,
	idle_agents: 0,
	cooldown_agents: 0,
	retired_agents: 0,
	open_quests: 0,
	active_quests: 0,
	completion_rate: 0,
	avg_quality: 0,
	active_parties: 0,
	active_guilds: 0,
	tokens_used_hourly: 0,
	tokens_limit_hourly: 0,
	token_budget_pct: 0,
	token_breaker: 'ok',
	cost_used_hourly_usd: 0,
	cost_total_usd: 0
};

// =============================================================================
// REACTIVE STATE
// =============================================================================

let agents = $state<Map<AgentID, Agent>>(new Map());
let quests = $state<Map<QuestID, Quest>>(new Map());
let parties = $state<Map<PartyID, Party>>(new Map());
let guilds = $state<Map<GuildID, Guild>>(new Map());
let battles = $state<Map<BattleID, BossBattle>>(new Map());
let recentEvents = $state<GameEvent[]>([]);
let connected = $state(false);
let synced = $state(false);
let loading = $state(false);
let error = $state<string | null>(null);

// Board control state
let boardPaused = $state(false);
let boardPausedAt = $state<string | null>(null);
let boardPausedBy = $state<string | null>(null);

// Token tracking state (updated from backend poll or world state snapshot)
let tokensUsedHourly = $state(0);
let tokensLimitHourly = $state(0);
let tokenBudgetPct = $state(0);
let tokenBreaker = $state<TokenBreakerState>('ok');
let costUsedHourlyUSD = $state(0);
let costTotalUSD = $state(0);

// Selected entities
let selectedAgentId = $state<AgentID | null>(null);
let selectedQuestId = $state<QuestID | null>(null);
let selectedBattleId = $state<BattleID | null>(null);

// Store state
let storeItems = $state<Map<string, StoreItem>>(new Map());
let inventories = $state<Map<AgentID, AgentInventory>>(new Map());
let activeEffects = $state<Map<AgentID, ActiveEffect[]>>(new Map());
let selectedStoreItemId = $state<string | null>(null);

// =============================================================================
// DERIVED STATE
// =============================================================================

const agentList = $derived(Array.from(agents.values()));
const questList = $derived(Array.from(quests.values()));
const partyList = $derived(Array.from(parties.values()));
const guildList = $derived(Array.from(guilds.values()));
const battleList = $derived(Array.from(battles.values()));

const selectedAgent = $derived(selectedAgentId ? agents.get(selectedAgentId) : null);
const selectedQuest = $derived(selectedQuestId ? quests.get(selectedQuestId) : null);
const selectedBattle = $derived(selectedBattleId ? battles.get(selectedBattleId) : null);

// Quest filtering
const openQuests = $derived(questList.filter((q) => q.status === 'posted'));
const activeQuests = $derived(
	questList.filter((q) => ['claimed', 'in_progress', 'in_review'].includes(q.status))
);
const completedQuests = $derived(questList.filter((q) => q.status === 'completed'));
const failedQuests = $derived(questList.filter((q) => q.status === 'failed'));

// Agent filtering
const idleAgents = $derived(agentList.filter((a) => a.status === 'idle'));
const busyAgents = $derived(agentList.filter((a) => a.status !== 'idle' && a.status !== 'retired'));
const retiredAgents = $derived(agentList.filter((a) => a.status === 'retired'));

// Active battles
const activeBattles = $derived(battleList.filter((b) => b.status === 'active'));

// Client-computed stats from entity Maps, with token fields from backend
const computedStats = $derived<WorldStats>({
	active_agents: agentList.filter((a) => a.status === 'on_quest' || a.status === 'in_battle').length,
	idle_agents: idleAgents.length,
	cooldown_agents: agentList.filter((a) => a.status === 'cooldown').length,
	retired_agents: retiredAgents.length,
	open_quests: openQuests.length,
	active_quests: activeQuests.length,
	completion_rate: (() => {
		const done = questList.filter((q) => q.status === 'completed').length;
		const failed = questList.filter((q) => q.status === 'failed').length;
		const total = done + failed;
		return total > 0 ? done / total : 0;
	})(),
	avg_quality: (() => {
		const scores = agentList.map((a) => a.stats.avg_quality_score).filter((s) => s > 0);
		return scores.length > 0 ? scores.reduce((a, b) => a + b, 0) / scores.length : 0;
	})(),
	active_parties: partyList.filter((p) => p.status === 'active').length,
	active_guilds: guildList.filter((g) => g.status === 'active').length,
	tokens_used_hourly: tokensUsedHourly,
	tokens_limit_hourly: tokensLimitHourly,
	token_budget_pct: tokenBudgetPct,
	token_breaker: tokenBreaker,
	cost_used_hourly_usd: costUsedHourlyUSD,
	cost_total_usd: costTotalUSD
});

// Tier distribution for agent breakdown
const tierDistribution = $derived.by(() => {
	const tiers = [
		{ tier: 0 as const, name: 'Apprentice', count: 0 },
		{ tier: 1 as const, name: 'Journeyman', count: 0 },
		{ tier: 2 as const, name: 'Expert', count: 0 },
		{ tier: 3 as const, name: 'Master', count: 0 },
		{ tier: 4 as const, name: 'Grandmaster', count: 0 }
	];

	for (const agent of agentList) {
		if (agent.tier >= 0 && agent.tier <= 4) {
			tiers[agent.tier].count++;
		}
	}

	// Avoid division by zero when no agents exist
	const total = agentList.length || 1;
	return tiers.map(t => ({
		...t,
		percentage: (t.count / total) * 100
	}));
});

// Total XP earned across all agents
const totalXpEarned = $derived(
	agentList.reduce((sum, agent) => sum + agent.stats.total_xp_earned, 0)
);

// Boss battle win/loss stats
const battleStats = $derived.by(() => {
	const won = battleList.filter(b => b.status === 'victory').length;
	const lost = battleList.filter(b => b.status === 'defeat').length;
	const total = won + lost;
	return {
		won,
		lost,
		winRate: total > 0 ? (won / total) * 100 : 0
	};
});

// Store derived state
const storeItemList = $derived(Array.from(storeItems.values()));
const selectedStoreItem = $derived(selectedStoreItemId ? storeItems.get(selectedStoreItemId) : null);
const toolItems = $derived(storeItemList.filter((item) => item.item_type === 'tool'));
const consumableItems = $derived(storeItemList.filter((item) => item.item_type === 'consumable'));

// =============================================================================
// ACTIONS
// =============================================================================

function setLoading(value: boolean) {
	loading = value;
}

function setError(message: string | null) {
	error = message;
}

function setConnected(value: boolean) {
	connected = value;
}

function setSynced(value: boolean) {
	synced = value;
}

function setBoardPaused(paused: boolean, pausedAt: string | null = null, pausedBy: string | null = null) {
	boardPaused = paused;
	boardPausedAt = pausedAt;
	boardPausedBy = pausedBy;
}

function setTokenStats(usedHourly: number, limitHourly: number, budgetPct: number, breaker: TokenBreakerState, hourlyCost = 0, totalCost = 0) {
	tokensUsedHourly = usedHourly;
	tokensLimitHourly = limitHourly;
	tokenBudgetPct = budgetPct;
	tokenBreaker = breaker;
	costUsedHourlyUSD = hourlyCost;
	costTotalUSD = totalCost;
}

function selectAgent(id: AgentID | null) {
	selectedAgentId = id;
}

function selectQuest(id: QuestID | null) {
	selectedQuestId = id;
}

function selectBattle(id: BattleID | null) {
	selectedBattleId = id;
}

// Upsert individual entities (insert or update)
function upsertAgent(agent: Agent) {
	agents = new Map(agents).set(agent.id, agent);
}

function upsertQuest(quest: Quest) {
	quests = new Map(quests).set(quest.id, quest);
}

function upsertParty(party: Party) {
	parties = new Map(parties).set(party.id, party);
}

function upsertGuild(guild: Guild) {
	guilds = new Map(guilds).set(guild.id, guild);
}

function upsertBattle(battle: BossBattle) {
	battles = new Map(battles).set(battle.id, battle);
}

// Remove individual entities
function removeAgent(id: AgentID) {
	const newMap = new Map(agents);
	newMap.delete(id);
	agents = newMap;
}

function removeQuest(id: QuestID) {
	const newMap = new Map(quests);
	newMap.delete(id);
	quests = newMap;
}

function removeParty(id: PartyID) {
	const newMap = new Map(parties);
	newMap.delete(id);
	parties = newMap;
}

function removeGuild(id: GuildID) {
	const newMap = new Map(guilds);
	newMap.delete(id);
	guilds = newMap;
}

function removeBattle(id: BattleID) {
	const newMap = new Map(battles);
	newMap.delete(id);
	battles = newMap;
}

// Store actions
function selectStoreItem(id: string | null) {
	selectedStoreItemId = id;
}

function setStoreItems(items: StoreItem[]) {
	storeItems = new Map(items.map((item) => [item.id, item]));
}

function updateStoreItem(item: StoreItem) {
	storeItems = new Map(storeItems).set(item.id, item);
}

function setInventory(inventory: AgentInventory) {
	inventories = new Map(inventories).set(inventory.agent_id, inventory);
}

function getInventoryFn(agentId: AgentID): AgentInventory | undefined {
	return inventories.get(agentId);
}

function setActiveEffectsFn(agentId: AgentID, effects: ActiveEffect[]) {
	activeEffects = new Map(activeEffects).set(agentId, effects);
}

function getActiveEffectsFn(agentId: AgentID): ActiveEffect[] {
	return activeEffects.get(agentId) ?? [];
}

// Add a new event to the recent events list
function addEvent(event: GameEvent) {
	recentEvents = [event, ...recentEvents].slice(0, MAX_RECENT_EVENTS);
}

// Bulk hydrate from world state snapshot (fallback for GET /game/world)
// Builds Maps once instead of creating N intermediate copies via individual upserts
function hydrateFromSnapshot(snapshot: {
	agents: Agent[];
	quests: Quest[];
	parties: Party[];
	guilds: Guild[];
	battles: BossBattle[];
}) {
	agents = new Map([...agents, ...snapshot.agents.map(a => [a.id, a] as const)]);
	quests = new Map([...quests, ...snapshot.quests.map(q => [q.id, q] as const)]);
	parties = new Map([...parties, ...snapshot.parties.map(p => [p.id, p] as const)]);
	guilds = new Map([...guilds, ...snapshot.guilds.map(g => [g.id, g] as const)]);
	battles = new Map([...battles, ...snapshot.battles.map(b => [b.id, b] as const)]);
}

// Clear all state
function reset() {
	agents = new Map();
	quests = new Map();
	parties = new Map();
	guilds = new Map();
	battles = new Map();
	recentEvents = [];
	selectedAgentId = null;
	selectedQuestId = null;
	selectedBattleId = null;
	storeItems = new Map();
	inventories = new Map();
	activeEffects = new Map();
	selectedStoreItemId = null;
	synced = false;
	error = null;
}

// =============================================================================
// FACTORY FOR TESTING
// =============================================================================

/**
 * Create a new world store instance for testing
 * Uses plain objects instead of runes for testability
 */
export function createWorldStore() {
	const state = {
		agents: new Map<AgentID, Agent>(),
		quests: new Map<QuestID, Quest>(),
		parties: new Map<PartyID, Party>(),
		guilds: new Map<GuildID, Guild>(),
		battles: new Map<BattleID, BossBattle>(),
		recentEvents: [] as GameEvent[],
		connected: false,
		synced: false,
		loading: true,
		error: null as string | null,
		selectedAgentId: null as AgentID | null,
		selectedQuestId: null as QuestID | null,
		selectedBattleId: null as BattleID | null,
		boardPaused: false,
		boardPausedAt: null as string | null,
		boardPausedBy: null as string | null,
		storeItems: new Map<string, StoreItem>(),
		inventories: new Map<AgentID, AgentInventory>(),
		activeEffects: new Map<AgentID, ActiveEffect[]>(),
		selectedStoreItemId: null as string | null
	};

	function computeStats(): WorldStats {
		const agentList = Array.from(state.agents.values());
		const questList = Array.from(state.quests.values());
		const partyList = Array.from(state.parties.values());
		const guildList = Array.from(state.guilds.values());
		const done = questList.filter((q) => q.status === 'completed').length;
		const failed = questList.filter((q) => q.status === 'failed').length;
		const total = done + failed;
		const scores = agentList.map((a) => a.stats.avg_quality_score).filter((s) => s > 0);
		return {
			active_agents: agentList.filter((a) => a.status === 'on_quest' || a.status === 'in_battle').length,
			idle_agents: agentList.filter((a) => a.status === 'idle').length,
			cooldown_agents: agentList.filter((a) => a.status === 'cooldown').length,
			retired_agents: agentList.filter((a) => a.status === 'retired').length,
			open_quests: questList.filter((q) => q.status === 'posted').length,
			active_quests: questList.filter((q) => ['claimed', 'in_progress', 'in_review'].includes(q.status)).length,
			completion_rate: total > 0 ? done / total : 0,
			avg_quality: scores.length > 0 ? scores.reduce((a, b) => a + b, 0) / scores.length : 0,
			active_parties: partyList.filter((p) => p.status === 'active').length,
			active_guilds: guildList.filter((g) => g.status === 'active').length,
			tokens_used_hourly: 0,
			tokens_limit_hourly: 0,
			token_budget_pct: 0,
			token_breaker: 'ok' as TokenBreakerState,
			cost_used_hourly_usd: 0,
			cost_total_usd: 0
		};
	}

	return {
		get agents() { return state.agents; },
		get quests() { return state.quests; },
		get parties() { return state.parties; },
		get guilds() { return state.guilds; },
		get battles() { return state.battles; },
		get stats() { return computeStats(); },
		get recentEvents() { return state.recentEvents; },
		get connected() { return state.connected; },
		get synced() { return state.synced; },
		get loading() { return state.loading; },
		get error() { return state.error; },
		get selectedAgentId() { return state.selectedAgentId; },
		get selectedQuestId() { return state.selectedQuestId; },
		get selectedBattleId() { return state.selectedBattleId; },
		get selectedAgent() { return state.selectedAgentId ? state.agents.get(state.selectedAgentId) ?? null : null; },
		get selectedQuest() { return state.selectedQuestId ? state.quests.get(state.selectedQuestId) ?? null : null; },
		get selectedBattle() { return state.selectedBattleId ? state.battles.get(state.selectedBattleId) ?? null : null; },
		get agentList() { return Array.from(state.agents.values()).sort((a, b) => b.level - a.level); },
		get questList() { return Array.from(state.quests.values()); },
		get partyList() { return Array.from(state.parties.values()); },
		get guildList() { return Array.from(state.guilds.values()); },
		get battleList() { return Array.from(state.battles.values()); },
		get tierDistribution() {
			const agentList = Array.from(state.agents.values());
			const tiers = [
				{ tier: 0 as const, name: 'Apprentice', count: 0 },
				{ tier: 1 as const, name: 'Journeyman', count: 0 },
				{ tier: 2 as const, name: 'Expert', count: 0 },
				{ tier: 3 as const, name: 'Master', count: 0 },
				{ tier: 4 as const, name: 'Grandmaster', count: 0 }
			];
			for (const agent of agentList) {
				if (agent.tier >= 0 && agent.tier <= 4) {
					tiers[agent.tier].count++;
				}
			}
			const total = agentList.length || 1;
			return tiers.map(t => ({ ...t, percentage: (t.count / total) * 100 }));
		},
		get totalXpEarned() {
			return Array.from(state.agents.values()).reduce((sum, agent) => sum + agent.stats.total_xp_earned, 0);
		},
		get battleStats() {
			const battles = Array.from(state.battles.values());
			const won = battles.filter(b => b.status === 'victory').length;
			const lost = battles.filter(b => b.status === 'defeat').length;
			const total = won + lost;
			return { won, lost, winRate: total > 0 ? (won / total) * 100 : 0 };
		},

		get boardPaused() { return state.boardPaused; },
		get boardPausedAt() { return state.boardPausedAt; },
		get boardPausedBy() { return state.boardPausedBy; },

		setLoading(value: boolean) { state.loading = value; },
		setError(message: string | null) { state.error = message; },
		setConnected(value: boolean) { state.connected = value; },
		setSynced(value: boolean) { state.synced = value; },
		setBoardPaused(paused: boolean, pausedAt: string | null = null, pausedBy: string | null = null) {
			state.boardPaused = paused;
			state.boardPausedAt = pausedAt;
			state.boardPausedBy = pausedBy;
		},
		selectAgent(id: AgentID | null) { state.selectedAgentId = id; },
		selectQuest(id: QuestID | null) { state.selectedQuestId = id; },
		selectBattle(id: BattleID | null) { state.selectedBattleId = id; },

		// Upsert methods (called by SSE service)
		upsertAgent(agent: Agent) { state.agents.set(agent.id, agent); },
		upsertQuest(quest: Quest) { state.quests.set(quest.id, quest); },
		upsertParty(party: Party) { state.parties.set(party.id, party); },
		upsertGuild(guild: Guild) { state.guilds.set(guild.id, guild); },
		upsertBattle(battle: BossBattle) { state.battles.set(battle.id, battle); },

		/** @deprecated Use upsertAgent instead */
		updateAgent(agent: Agent) { state.agents.set(agent.id, agent); },
		/** @deprecated Use upsertQuest instead */
		updateQuest(quest: Quest) { state.quests.set(quest.id, quest); },
		/** @deprecated Use upsertParty instead */
		updateParty(party: Party) { state.parties.set(party.id, party); },
		/** @deprecated Use upsertGuild instead */
		updateGuild(guild: Guild) { state.guilds.set(guild.id, guild); },
		/** @deprecated Use upsertBattle instead */
		updateBattle(battle: BossBattle) { state.battles.set(battle.id, battle); },

		// Remove methods
		removeAgent(id: AgentID) { state.agents.delete(id); },
		removeQuest(id: QuestID) { state.quests.delete(id); },
		removeParty(id: PartyID) { state.parties.delete(id); },
		removeGuild(id: GuildID) { state.guilds.delete(id); },
		removeBattle(id: BattleID) { state.battles.delete(id); },

		addEvent(event: GameEvent) {
			state.recentEvents = [event, ...state.recentEvents].slice(0, MAX_RECENT_EVENTS);
		},

		hydrateFromSnapshot(newState: {
			agents: Agent[];
			quests: Quest[];
			parties: Party[];
			guilds: Guild[];
			battles: BossBattle[];
		}) {
			for (const a of newState.agents) state.agents.set(a.id, a);
			for (const q of newState.quests) state.quests.set(q.id, q);
			for (const p of newState.parties) state.parties.set(p.id, p);
			for (const g of newState.guilds) state.guilds.set(g.id, g);
			for (const b of newState.battles) state.battles.set(b.id, b);
		},

		// Legacy setWorldState for backward compatibility
		setWorldState(newState: {
			agents: Agent[];
			quests: Quest[];
			parties: Party[];
			guilds: Guild[];
			battles: BossBattle[];
			stats: WorldStats;
		}) {
			state.agents = new Map(newState.agents.map((a) => [a.id, a]));
			state.quests = new Map(newState.quests.map((q) => [q.id, q]));
			state.parties = new Map(newState.parties.map((p) => [p.id, p]));
			state.guilds = new Map(newState.guilds.map((g) => [g.id, g]));
			state.battles = new Map(newState.battles.map((b) => [b.id, b]));
			state.loading = false;
		},

		reset() {
			state.agents.clear();
			state.quests.clear();
			state.parties.clear();
			state.guilds.clear();
			state.battles.clear();
			state.recentEvents = [];
			state.selectedAgentId = null;
			state.selectedQuestId = null;
			state.selectedBattleId = null;
			state.storeItems.clear();
			state.inventories.clear();
			state.activeEffects.clear();
			state.selectedStoreItemId = null;
			state.synced = false;
			state.error = null;
		},

		// Store methods
		get storeItems() { return state.storeItems; },
		get storeItemList() { return Array.from(state.storeItems.values()); },
		get selectedStoreItemId() { return state.selectedStoreItemId; },
		get selectedStoreItem() { return state.selectedStoreItemId ? state.storeItems.get(state.selectedStoreItemId) : undefined; },
		get inventories() { return state.inventories; },
		get activeEffects() { return state.activeEffects; },

		selectStoreItem(id: string | null) { state.selectedStoreItemId = id; },
		setStoreItems(items: StoreItem[]) {
			state.storeItems = new Map(items.map((item) => [item.id, item]));
		},
		updateStoreItem(item: StoreItem) { state.storeItems.set(item.id, item); },
		setInventory(inventory: AgentInventory) { state.inventories.set(inventory.agent_id, inventory); },
		getInventory(agentId: AgentID) { return state.inventories.get(agentId); },
		setActiveEffects(agentId: AgentID, effects: ActiveEffect[]) { state.activeEffects.set(agentId, effects); },
		getActiveEffects(agentId: AgentID) { return state.activeEffects.get(agentId) ?? []; }
	};
}

// =============================================================================
// EXPORT
// =============================================================================

export const worldStore = {
	// Getters for reactive state
	get agents() { return agents; },
	get quests() { return quests; },
	get parties() { return parties; },
	get guilds() { return guilds; },
	get battles() { return battles; },
	get stats() { return computedStats; },
	get recentEvents() { return recentEvents; },
	get connected() { return connected; },
	get synced() { return synced; },
	get loading() { return loading; },
	get error() { return error; },
	get boardPaused() { return boardPaused; },
	get boardPausedAt() { return boardPausedAt; },
	get boardPausedBy() { return boardPausedBy; },

	// Derived lists
	get agentList() { return agentList; },
	get questList() { return questList; },
	get partyList() { return partyList; },
	get guildList() { return guildList; },
	get battleList() { return battleList; },

	// Selections
	get selectedAgentId() { return selectedAgentId; },
	get selectedQuestId() { return selectedQuestId; },
	get selectedBattleId() { return selectedBattleId; },
	get selectedAgent() { return selectedAgent; },
	get selectedQuest() { return selectedQuest; },
	get selectedBattle() { return selectedBattle; },

	// Filtered lists
	get openQuests() { return openQuests; },
	get activeQuests() { return activeQuests; },
	get completedQuests() { return completedQuests; },
	get failedQuests() { return failedQuests; },
	get idleAgents() { return idleAgents; },
	get busyAgents() { return busyAgents; },
	get retiredAgents() { return retiredAgents; },
	get activeBattles() { return activeBattles; },

	// Dashboard derived state
	get tierDistribution() { return tierDistribution; },
	get totalXpEarned() { return totalXpEarned; },
	get battleStats() { return battleStats; },

	// Store state
	get storeItems() { return storeItems; },
	get storeItemList() { return storeItemList; },
	get toolItems() { return toolItems; },
	get consumableItems() { return consumableItems; },
	get inventories() { return inventories; },
	get activeEffects() { return activeEffects; },
	get selectedStoreItemId() { return selectedStoreItemId; },
	get selectedStoreItem() { return selectedStoreItem; },

	// Actions
	setLoading,
	setError,
	setConnected,
	setSynced,
	setBoardPaused,
	setTokenStats,
	selectAgent,
	selectQuest,
	selectBattle,

	// Upsert (SSE-driven)
	upsertAgent,
	upsertQuest,
	upsertParty,
	upsertGuild,
	upsertBattle,

	/** @deprecated Use upsertAgent instead */
	updateAgent: upsertAgent,
	/** @deprecated Use upsertQuest instead */
	updateQuest: upsertQuest,
	/** @deprecated Use upsertParty instead */
	updateParty: upsertParty,
	/** @deprecated Use upsertGuild instead */
	updateGuild: upsertGuild,
	/** @deprecated Use upsertBattle instead */
	updateBattle: upsertBattle,

	// Remove
	removeAgent,
	removeQuest,
	removeParty,
	removeGuild,
	removeBattle,

	addEvent,
	hydrateFromSnapshot,
	reset,

	// Store actions
	selectStoreItem,
	setStoreItems,
	updateStoreItem,
	setInventory,
	getInventory: getInventoryFn,
	setActiveEffects: setActiveEffectsFn,
	getActiveEffects: getActiveEffectsFn
};
