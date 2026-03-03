/**
 * Unit tests for worldStore
 */

import { describe, it, expect, beforeEach } from 'vitest';
import { createWorldStore } from './worldStore.svelte';
import {
	type Agent,
	type Quest,
	type BossBattle,
	type Party,
	type Guild,
	type GameEvent,
	type WorldStats,
	agentId,
	questId,
	battleId,
	partyId,
	guildId
} from '$types';

// Default stats for tests
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
	active_guilds: 0
};

// Helper to create a test agent
function createTestAgent(overrides: Partial<Agent> = {}): Agent {
	return {
		id: agentId('agent-1'),
		name: 'Test Agent',
		display_name: '',
		level: 5,
		xp: 100,
		xp_to_level: 500,
		tier: 1,
		status: 'idle',
		equipment: [],
		guilds: [],
		death_count: 0,
		skill_proficiencies: {} as Agent['skill_proficiencies'],
		stats: {
			quests_completed: 10,
			quests_failed: 1,
			bosses_defeated: 5,
			bosses_failed: 0,
			total_xp_earned: 1000,
			total_xp_spent: 0,
			avg_quality_score: 0.85,
			avg_efficiency: 0.9,
			parties_led: 2,
			quests_decomposed: 3,
			peer_review_avg: 0,
			peer_review_count: 0
		},
		config: {
			provider: 'openai',
			model: 'gpt-4',
			system_prompt: 'You are a helpful assistant.',
			temperature: 0.7,
			max_tokens: 4096,
			metadata: {}
		},
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
		total_spent: 0,
		...overrides
	};
}

// Helper to create a test quest
function createTestQuest(overrides: Partial<Quest> = {}): Quest {
	return {
		id: questId('quest-1'),
		title: 'Test Quest',
		description: 'A test quest',
		status: 'posted',
		difficulty: 2,
		base_xp: 100,
		bonus_xp: 0,
		guild_xp: 0,
		required_skills: ['code_generation'],
		required_tools: [],
		min_tier: 0,
		max_attempts: 3,
		attempts: 0,
		party_required: false,
		min_party_size: 1,
		sub_quests: [],
		escalated: false,
		input: null,
		output: null,
		constraints: {
			max_duration: 3600000000000,
			max_cost: 1.0,
			max_tokens: 100000,
			require_review: true,
			review_level: 1
		},
		posted_at: new Date().toISOString(),
		trajectory_id: 'traj-123',
		...overrides
	};
}

// Helper to create a test battle
function createTestBattle(overrides: Partial<BossBattle> = {}): BossBattle {
	return {
		id: battleId('battle-1'),
		quest_id: questId('quest-1'),
		agent_id: agentId('agent-1'),
		status: 'active',
		level: 1,
		criteria: [
			{
				name: 'Code Quality',
				description: 'Code follows best practices',
				weight: 1,
				threshold: 0.7
			}
		],
		results: [],
		judges: [{ type: 'llm', id: 'gpt-4', config: {} }],
		started_at: new Date().toISOString(),
		...overrides
	};
}

// Helper to create a test party
function createTestParty(overrides: Partial<Party> = {}): Party {
	return {
		id: partyId('party-1'),
		name: 'Alpha Squad',
		status: 'active',
		quest_id: questId('quest-1'),
		lead: agentId('agent-1'),
		members: [],
		strategy: 'balanced',
		sub_quest_map: {},
		shared_context: [],
		sub_results: {},
		formed_at: new Date().toISOString(),
		...overrides
	};
}

// Helper to create a test guild
function createTestGuild(overrides: Partial<Guild> = {}): Guild {
	return {
		id: guildId('guild-1'),
		name: 'Data Wranglers',
		description: 'A guild of data specialists',
		status: 'active',
		members: [],
		max_members: 20,
		min_level: 3,
		founded: new Date().toISOString(),
		founded_by: agentId('agent-1'),
		culture: 'data-first',
		reputation: 0.8,
		quests_handled: 50,
		success_rate: 0.9,
		quests_failed: 5,
		shared_tools: [],
		created_at: new Date().toISOString(),
		...overrides
	};
}

// Helper to create a test event
function createTestEvent(overrides: Partial<GameEvent> = {}): GameEvent {
	return {
		type: 'quest.lifecycle.posted',
		timestamp: Date.now(),
		session_id: 'session-1',
		trajectory_id: 'traj-1',
		span_id: 'span-1',
		data: {},
		...overrides
	};
}

describe('worldStore', () => {
	let store: ReturnType<typeof createWorldStore>;

	beforeEach(() => {
		store = createWorldStore();
	});

	describe('initial state', () => {
		it('starts with empty collections', () => {
			expect(store.agents.size).toBe(0);
			expect(store.quests.size).toBe(0);
			expect(store.battles.size).toBe(0);
			expect(store.parties.size).toBe(0);
			expect(store.guilds.size).toBe(0);
			expect(store.recentEvents).toHaveLength(0);
		});

		it('starts disconnected and loading', () => {
			expect(store.connected).toBe(false);
			expect(store.loading).toBe(true);
		});

		it('starts not synced', () => {
			expect(store.synced).toBe(false);
		});

		it('has no selected entities', () => {
			expect(store.selectedAgentId).toBeNull();
			expect(store.selectedQuestId).toBeNull();
			expect(store.selectedBattleId).toBeNull();
		});
	});

	describe('upsertAgent', () => {
		it('adds a new agent', () => {
			const agent = createTestAgent();
			store.upsertAgent(agent);

			expect(store.agents.size).toBe(1);
			expect(store.agents.get(agent.id)).toEqual(agent);
		});

		it('updates an existing agent', () => {
			const agent = createTestAgent();
			store.upsertAgent(agent);

			const updatedAgent = { ...agent, level: 6, xp: 200 };
			store.upsertAgent(updatedAgent);

			expect(store.agents.size).toBe(1);
			expect(store.agents.get(agent.id)?.level).toBe(6);
			expect(store.agents.get(agent.id)?.xp).toBe(200);
		});
	});

	describe('upsertQuest', () => {
		it('adds a new quest', () => {
			const quest = createTestQuest();
			store.upsertQuest(quest);

			expect(store.quests.size).toBe(1);
			expect(store.quests.get(quest.id)).toEqual(quest);
		});

		it('updates an existing quest', () => {
			const quest = createTestQuest();
			store.upsertQuest(quest);

			const updatedQuest = { ...quest, status: 'claimed' as const, claimed_by: agentId('agent-1') };
			store.upsertQuest(updatedQuest);

			expect(store.quests.size).toBe(1);
			expect(store.quests.get(quest.id)?.status).toBe('claimed');
		});
	});

	describe('upsertBattle', () => {
		it('adds a new battle', () => {
			const battle = createTestBattle();
			store.upsertBattle(battle);

			expect(store.battles.size).toBe(1);
			expect(store.battles.get(battle.id)).toEqual(battle);
		});

		it('updates an existing battle', () => {
			const battle = createTestBattle();
			store.upsertBattle(battle);

			const updatedBattle = {
				...battle,
				status: 'victory' as const,
				verdict: {
					passed: true,
					quality_score: 0.9,
					xp_awarded: 150,
					xp_penalty: 0,
					level_change: 0,
					feedback: 'Great work!'
				}
			};
			store.upsertBattle(updatedBattle);

			expect(store.battles.size).toBe(1);
			expect(store.battles.get(battle.id)?.status).toBe('victory');
			expect(store.battles.get(battle.id)?.verdict?.passed).toBe(true);
		});
	});

	describe('upsertParty', () => {
		it('adds a new party', () => {
			const party = createTestParty();
			store.upsertParty(party);

			expect(store.parties.size).toBe(1);
			expect(store.parties.get(party.id)).toEqual(party);
		});

		it('updates an existing party', () => {
			const party = createTestParty();
			store.upsertParty(party);

			const updated = { ...party, status: 'disbanded' as const };
			store.upsertParty(updated);

			expect(store.parties.size).toBe(1);
			expect(store.parties.get(party.id)?.status).toBe('disbanded');
		});
	});

	describe('upsertGuild', () => {
		it('adds a new guild', () => {
			const guild = createTestGuild();
			store.upsertGuild(guild);

			expect(store.guilds.size).toBe(1);
			expect(store.guilds.get(guild.id)).toEqual(guild);
		});

		it('updates an existing guild', () => {
			const guild = createTestGuild();
			store.upsertGuild(guild);

			const updated = { ...guild, reputation: 0.95 };
			store.upsertGuild(updated);

			expect(store.guilds.size).toBe(1);
			expect(store.guilds.get(guild.id)?.reputation).toBe(0.95);
		});
	});

	describe('removeAgent', () => {
		it('removes an agent by ID', () => {
			store.upsertAgent(createTestAgent({ id: agentId('agent-1') }));
			store.upsertAgent(createTestAgent({ id: agentId('agent-2') }));

			store.removeAgent(agentId('agent-1'));

			expect(store.agents.size).toBe(1);
			expect(store.agents.has(agentId('agent-1'))).toBe(false);
			expect(store.agents.has(agentId('agent-2'))).toBe(true);
		});

		it('does nothing when removing non-existent agent', () => {
			store.removeAgent(agentId('agent-nonexistent'));
			expect(store.agents.size).toBe(0);
		});
	});

	describe('removeQuest', () => {
		it('removes a quest by ID', () => {
			store.upsertQuest(createTestQuest({ id: questId('quest-1') }));
			store.removeQuest(questId('quest-1'));

			expect(store.quests.size).toBe(0);
		});
	});

	describe('removeBattle', () => {
		it('removes a battle by ID', () => {
			store.upsertBattle(createTestBattle({ id: battleId('battle-1') }));
			store.removeBattle(battleId('battle-1'));

			expect(store.battles.size).toBe(0);
		});
	});

	describe('removeParty', () => {
		it('removes a party by ID', () => {
			store.upsertParty(createTestParty({ id: partyId('party-1') }));
			store.removeParty(partyId('party-1'));

			expect(store.parties.size).toBe(0);
		});
	});

	describe('removeGuild', () => {
		it('removes a guild by ID', () => {
			store.upsertGuild(createTestGuild({ id: guildId('guild-1') }));
			store.removeGuild(guildId('guild-1'));

			expect(store.guilds.size).toBe(0);
		});
	});

	describe('synced state', () => {
		it('sets synced state', () => {
			expect(store.synced).toBe(false);

			store.setSynced(true);
			expect(store.synced).toBe(true);

			store.setSynced(false);
			expect(store.synced).toBe(false);
		});

		it('reset clears synced state', () => {
			store.setSynced(true);
			store.reset();
			expect(store.synced).toBe(false);
		});
	});

	describe('hydrateFromSnapshot', () => {
		it('populates all entity maps from snapshot', () => {
			const agents = [createTestAgent({ id: agentId('agent-1') })];
			const quests = [createTestQuest({ id: questId('quest-1') })];
			const parties = [createTestParty({ id: partyId('party-1') })];
			const guilds = [createTestGuild({ id: guildId('guild-1') })];
			const battles = [createTestBattle({ id: battleId('battle-1') })];

			store.hydrateFromSnapshot({ agents, quests, parties, guilds, battles });

			expect(store.agents.size).toBe(1);
			expect(store.quests.size).toBe(1);
			expect(store.parties.size).toBe(1);
			expect(store.guilds.size).toBe(1);
			expect(store.battles.size).toBe(1);
		});

		it('merges with existing data (does not clear)', () => {
			store.upsertAgent(createTestAgent({ id: agentId('agent-existing') }));

			store.hydrateFromSnapshot({
				agents: [createTestAgent({ id: agentId('agent-new') })],
				quests: [],
				parties: [],
				guilds: [],
				battles: []
			});

			expect(store.agents.size).toBe(2);
			expect(store.agents.has(agentId('agent-existing'))).toBe(true);
			expect(store.agents.has(agentId('agent-new'))).toBe(true);
		});
	});

	describe('setWorldState (legacy)', () => {
		it('replaces all agents, quests, and battles', () => {
			const agents = [createTestAgent({ id: agentId('agent-1') })];
			const quests = [createTestQuest({ id: questId('quest-1') })];
			const battles = [createTestBattle({ id: battleId('battle-1') })];

			store.setWorldState({
				agents,
				quests,
				parties: [],
				guilds: [],
				battles,
				stats: defaultStats
			});

			expect(store.agents.size).toBe(1);
			expect(store.quests.size).toBe(1);
			expect(store.battles.size).toBe(1);
			expect(store.loading).toBe(false);
		});

		it('clears previous state when setting new state', () => {
			store.setWorldState({
				agents: [createTestAgent({ id: agentId('agent-1') })],
				quests: [],
				parties: [],
				guilds: [],
				battles: [],
				stats: defaultStats
			});

			store.setWorldState({
				agents: [createTestAgent({ id: agentId('agent-2'), name: 'New Agent' })],
				quests: [],
				parties: [],
				guilds: [],
				battles: [],
				stats: defaultStats
			});

			expect(store.agents.size).toBe(1);
			expect(store.agents.has(agentId('agent-1'))).toBe(false);
			expect(store.agents.has(agentId('agent-2'))).toBe(true);
		});
	});

	describe('legacy update aliases', () => {
		it('updateAgent works as upsert', () => {
			const agent = createTestAgent();
			store.updateAgent(agent);
			expect(store.agents.size).toBe(1);
			expect(store.agents.get(agent.id)).toEqual(agent);
		});

		it('updateQuest works as upsert', () => {
			const quest = createTestQuest();
			store.updateQuest(quest);
			expect(store.quests.size).toBe(1);
		});

		it('updateBattle works as upsert', () => {
			const battle = createTestBattle();
			store.updateBattle(battle);
			expect(store.battles.size).toBe(1);
		});
	});

	describe('selection', () => {
		beforeEach(() => {
			store.upsertAgent(createTestAgent({ id: agentId('agent-1') }));
			store.upsertQuest(createTestQuest({ id: questId('quest-1') }));
			store.upsertBattle(createTestBattle({ id: battleId('battle-1') }));
		});

		it('selects an agent', () => {
			store.selectAgent(agentId('agent-1'));
			expect(store.selectedAgentId).toBe('agent-1');
			expect(store.selectedAgent).toBeDefined();
			expect(store.selectedAgent?.name).toBe('Test Agent');
		});

		it('selects a quest', () => {
			store.selectQuest(questId('quest-1'));
			expect(store.selectedQuestId).toBe('quest-1');
			expect(store.selectedQuest).toBeDefined();
			expect(store.selectedQuest?.title).toBe('Test Quest');
		});

		it('selects a battle', () => {
			store.selectBattle(battleId('battle-1'));
			expect(store.selectedBattleId).toBe('battle-1');
			expect(store.selectedBattle).toBeDefined();
			expect(store.selectedBattle?.status).toBe('active');
		});

		it('clears selection with null', () => {
			store.selectAgent(agentId('agent-1'));
			expect(store.selectedAgentId).not.toBeNull();

			store.selectAgent(null);
			expect(store.selectedAgentId).toBeNull();
			expect(store.selectedAgent).toBeNull();
		});
	});

	describe('addEvent', () => {
		it('adds events to the event stream', () => {
			const event = createTestEvent({ data: { quest: createTestQuest() } });

			store.addEvent(event);
			expect(store.recentEvents).toHaveLength(1);
			expect(store.recentEvents[0].type).toBe('quest.lifecycle.posted');
		});

		it('limits the event stream to 100 events', () => {
			for (let i = 0; i < 105; i++) {
				store.addEvent(createTestEvent({
					type: 'quest.lifecycle.claimed',
					timestamp: Date.now() + i
				}));
			}

			expect(store.recentEvents).toHaveLength(100);
		});
	});

	describe('connection state', () => {
		it('sets connected state', () => {
			store.setConnected(true);
			expect(store.connected).toBe(true);

			store.setConnected(false);
			expect(store.connected).toBe(false);
		});

		it('sets error state', () => {
			store.setError('Connection failed');
			expect(store.error).toBe('Connection failed');

			store.setError(null);
			expect(store.error).toBeNull();
		});
	});

	describe('computed stats', () => {
		it('computes agent stats from entity maps', () => {
			store.upsertAgent(createTestAgent({ id: agentId('a1'), status: 'idle' }));
			store.upsertAgent(createTestAgent({ id: agentId('a2'), status: 'on_quest' }));
			store.upsertAgent(createTestAgent({ id: agentId('a3'), status: 'retired' }));

			const stats = store.stats;
			expect(stats.idle_agents).toBe(1);
			expect(stats.active_agents).toBe(1);
			expect(stats.retired_agents).toBe(1);
		});

		it('computes quest stats', () => {
			store.upsertQuest(createTestQuest({ id: questId('q1'), status: 'posted' }));
			store.upsertQuest(createTestQuest({ id: questId('q2'), status: 'in_progress' }));
			store.upsertQuest(createTestQuest({ id: questId('q3'), status: 'completed' }));
			store.upsertQuest(createTestQuest({ id: questId('q4'), status: 'failed' }));

			const stats = store.stats;
			expect(stats.open_quests).toBe(1);
			expect(stats.active_quests).toBe(1);
			expect(stats.completion_rate).toBe(0.5); // 1 completed / 2 (completed + failed)
		});
	});

	describe('dashboard derived states', () => {
		describe('tierDistribution', () => {
			it('calculates tier distribution with correct percentages', () => {
				store.setWorldState({
					agents: [
						createTestAgent({ id: agentId('agent-1'), level: 3, tier: 0 }),
						createTestAgent({ id: agentId('agent-2'), level: 7, tier: 1 }),
						createTestAgent({ id: agentId('agent-3'), level: 8, tier: 1 }),
						createTestAgent({ id: agentId('agent-4'), level: 12, tier: 2 })
					],
					quests: [],
					parties: [],
					guilds: [],
					battles: [],
					stats: defaultStats
				});

				const distribution = store.tierDistribution;
				expect(distribution).toHaveLength(5);
				expect(distribution[0].count).toBe(1);
				expect(distribution[0].name).toBe('Apprentice');
				expect(distribution[1].count).toBe(2);
				expect(distribution[1].name).toBe('Journeyman');
				expect(distribution[2].count).toBe(1);
				expect(distribution[0].percentage).toBe(25);
				expect(distribution[1].percentage).toBe(50);
				expect(distribution[2].percentage).toBe(25);
			});

			it('handles empty agent list without division by zero', () => {
				const distribution = store.tierDistribution;
				expect(distribution).toHaveLength(5);
				distribution.forEach((tier) => {
					expect(tier.count).toBe(0);
					expect(tier.percentage).toBe(0);
				});
			});

			it('includes all five tiers even when some are empty', () => {
				store.upsertAgent(createTestAgent({ id: agentId('agent-1'), level: 19, tier: 4 }));

				const distribution = store.tierDistribution;
				expect(distribution).toHaveLength(5);
				expect(distribution[4].name).toBe('Grandmaster');
				expect(distribution[4].count).toBe(1);
				expect(distribution[4].percentage).toBe(100);
				expect(distribution[0].count).toBe(0);
			});
		});

		describe('totalXpEarned', () => {
			it('sums total XP across all agents', () => {
				store.upsertAgent(createTestAgent({
					id: agentId('agent-1'),
					stats: {
						quests_completed: 10, quests_failed: 1, bosses_defeated: 5,
						bosses_failed: 0, total_xp_earned: 1000, total_xp_spent: 0,
						avg_quality_score: 0.85, avg_efficiency: 0.9, parties_led: 2, quests_decomposed: 3,
						peer_review_avg: 0, peer_review_count: 0
					}
				}));
				store.upsertAgent(createTestAgent({
					id: agentId('agent-2'),
					stats: {
						quests_completed: 20, quests_failed: 2, bosses_defeated: 10,
						bosses_failed: 1, total_xp_earned: 2500, total_xp_spent: 100,
						avg_quality_score: 0.9, avg_efficiency: 0.85, parties_led: 5, quests_decomposed: 7,
						peer_review_avg: 0, peer_review_count: 0
					}
				}));

				expect(store.totalXpEarned).toBe(3500);
			});

			it('returns 0 for empty agent list', () => {
				expect(store.totalXpEarned).toBe(0);
			});
		});

		describe('battleStats', () => {
			it('calculates win rate correctly', () => {
				store.upsertBattle(createTestBattle({ id: battleId('b1'), status: 'victory' }));
				store.upsertBattle(createTestBattle({ id: battleId('b2'), status: 'victory' }));
				store.upsertBattle(createTestBattle({ id: battleId('b3'), status: 'victory' }));
				store.upsertBattle(createTestBattle({ id: battleId('b4'), status: 'defeat' }));

				const stats = store.battleStats;
				expect(stats.won).toBe(3);
				expect(stats.lost).toBe(1);
				expect(stats.winRate).toBe(75);
			});

			it('handles no battles without division by zero', () => {
				const stats = store.battleStats;
				expect(stats.won).toBe(0);
				expect(stats.lost).toBe(0);
				expect(stats.winRate).toBe(0);
			});

			it('ignores active and retreat battles when calculating win rate', () => {
				store.upsertBattle(createTestBattle({ id: battleId('b1'), status: 'active' }));
				store.upsertBattle(createTestBattle({ id: battleId('b2'), status: 'retreat' }));
				store.upsertBattle(createTestBattle({ id: battleId('b3'), status: 'victory' }));

				const stats = store.battleStats;
				expect(stats.won).toBe(1);
				expect(stats.lost).toBe(0);
				expect(stats.winRate).toBe(100);
			});

			it('calculates 50% win rate correctly', () => {
				store.upsertBattle(createTestBattle({ id: battleId('b1'), status: 'victory' }));
				store.upsertBattle(createTestBattle({ id: battleId('b2'), status: 'defeat' }));

				const stats = store.battleStats;
				expect(stats.won).toBe(1);
				expect(stats.lost).toBe(1);
				expect(stats.winRate).toBe(50);
			});
		});
	});

	describe('derived lists', () => {
		it('provides sorted agent list', () => {
			store.upsertAgent(createTestAgent({ id: agentId('agent-3'), level: 10 }));
			store.upsertAgent(createTestAgent({ id: agentId('agent-1'), level: 5 }));
			store.upsertAgent(createTestAgent({ id: agentId('agent-2'), level: 15 }));

			const list = store.agentList;
			expect(list).toHaveLength(3);
			expect(list[0].level).toBe(15);
			expect(list[1].level).toBe(10);
			expect(list[2].level).toBe(5);
		});

		it('provides quest list', () => {
			store.upsertQuest(createTestQuest({ id: questId('quest-1') }));
			store.upsertQuest(createTestQuest({ id: questId('quest-2') }));

			expect(store.questList).toHaveLength(2);
		});

		it('provides battle list', () => {
			store.upsertBattle(createTestBattle({ id: battleId('battle-1') }));
			store.upsertBattle(createTestBattle({ id: battleId('battle-2') }));

			expect(store.battleList).toHaveLength(2);
		});

		it('provides party list', () => {
			store.upsertParty(createTestParty({ id: partyId('party-1') }));
			store.upsertParty(createTestParty({ id: partyId('party-2') }));

			expect(store.partyList).toHaveLength(2);
		});

		it('provides guild list', () => {
			store.upsertGuild(createTestGuild({ id: guildId('guild-1') }));
			store.upsertGuild(createTestGuild({ id: guildId('guild-2') }));

			expect(store.guildList).toHaveLength(2);
		});
	});

	describe('reset', () => {
		it('clears all state', () => {
			store.upsertAgent(createTestAgent());
			store.upsertQuest(createTestQuest());
			store.upsertBattle(createTestBattle());
			store.upsertParty(createTestParty());
			store.upsertGuild(createTestGuild());
			store.addEvent(createTestEvent());
			store.setSynced(true);
			store.setError('some error');

			store.reset();

			expect(store.agents.size).toBe(0);
			expect(store.quests.size).toBe(0);
			expect(store.battles.size).toBe(0);
			expect(store.parties.size).toBe(0);
			expect(store.guilds.size).toBe(0);
			expect(store.recentEvents).toHaveLength(0);
			expect(store.synced).toBe(false);
			expect(store.error).toBeNull();
		});
	});
});
