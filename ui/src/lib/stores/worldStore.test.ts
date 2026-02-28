/**
 * Unit tests for worldStore
 */

import { describe, it, expect, beforeEach } from 'vitest';
import { createWorldStore } from './worldStore.svelte';
import {
	type Agent,
	type Quest,
	type BossBattle,
	type GameEvent,
	type WorldStats,
	agentId,
	questId,
	battleId
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
		level: 5,
		xp: 100,
		xp_to_level: 500,
		tier: 1,
		status: 'idle',
		skills: ['code_generation', 'code_review'],
		equipment: [],
		guilds: [],
		death_count: 0,
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
			quests_decomposed: 3
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

// Helper to create a test event
function createTestEvent(overrides: Partial<GameEvent> = {}): GameEvent {
	return {
		type: 'quest.posted',
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
			expect(store.recentEvents).toHaveLength(0);
		});

		it('starts disconnected and loading', () => {
			expect(store.connected).toBe(false);
			expect(store.loading).toBe(true);
		});

		it('has no selected entities', () => {
			expect(store.selectedAgentId).toBeNull();
			expect(store.selectedQuestId).toBeNull();
			expect(store.selectedBattleId).toBeNull();
		});
	});

	describe('setWorldState', () => {
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
			// Set initial state
			store.setWorldState({
				agents: [createTestAgent({ id: agentId('agent-1') })],
				quests: [],
				parties: [],
				guilds: [],
				battles: [],
				stats: defaultStats
			});

			// Set new state with different agent
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

	describe('updateAgent', () => {
		it('adds a new agent', () => {
			const agent = createTestAgent();
			store.updateAgent(agent);

			expect(store.agents.size).toBe(1);
			expect(store.agents.get(agent.id)).toEqual(agent);
		});

		it('updates an existing agent', () => {
			const agent = createTestAgent();
			store.updateAgent(agent);

			const updatedAgent = { ...agent, level: 6, xp: 200 };
			store.updateAgent(updatedAgent);

			expect(store.agents.size).toBe(1);
			expect(store.agents.get(agent.id)?.level).toBe(6);
			expect(store.agents.get(agent.id)?.xp).toBe(200);
		});
	});

	describe('updateQuest', () => {
		it('adds a new quest', () => {
			const quest = createTestQuest();
			store.updateQuest(quest);

			expect(store.quests.size).toBe(1);
			expect(store.quests.get(quest.id)).toEqual(quest);
		});

		it('updates an existing quest', () => {
			const quest = createTestQuest();
			store.updateQuest(quest);

			const updatedQuest = { ...quest, status: 'claimed' as const, claimed_by: agentId('agent-1') };
			store.updateQuest(updatedQuest);

			expect(store.quests.size).toBe(1);
			expect(store.quests.get(quest.id)?.status).toBe('claimed');
			expect(store.quests.get(quest.id)?.claimed_by).toBe('agent-1');
		});
	});

	describe('updateBattle', () => {
		it('adds a new battle', () => {
			const battle = createTestBattle();
			store.updateBattle(battle);

			expect(store.battles.size).toBe(1);
			expect(store.battles.get(battle.id)).toEqual(battle);
		});

		it('updates an existing battle', () => {
			const battle = createTestBattle();
			store.updateBattle(battle);

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
			store.updateBattle(updatedBattle);

			expect(store.battles.size).toBe(1);
			expect(store.battles.get(battle.id)?.status).toBe('victory');
			expect(store.battles.get(battle.id)?.verdict?.passed).toBe(true);
		});
	});

	describe('selection', () => {
		beforeEach(() => {
			store.setWorldState({
				agents: [createTestAgent({ id: agentId('agent-1') })],
				quests: [createTestQuest({ id: questId('quest-1') })],
				parties: [],
				guilds: [],
				battles: [createTestBattle({ id: battleId('battle-1') })],
				stats: defaultStats
			});
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
			expect(store.selectedAgent).toBeUndefined();
		});
	});

	describe('addEvent', () => {
		it('adds events to the event stream', () => {
			const event = createTestEvent({ data: { quest: createTestQuest() } });

			store.addEvent(event);
			expect(store.recentEvents).toHaveLength(1);
			expect(store.recentEvents[0].type).toBe('quest.posted');
		});

		it('limits the event stream to 100 events', () => {
			// Add 105 events
			for (let i = 0; i < 105; i++) {
				store.addEvent(createTestEvent({
					type: 'quest.claimed',
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

	describe('dashboard derived states', () => {
		describe('tierDistribution', () => {
			it('calculates tier distribution with correct percentages', () => {
				store.setWorldState({
					agents: [
						createTestAgent({ id: agentId('agent-1'), level: 3, tier: 0 }), // Apprentice
						createTestAgent({ id: agentId('agent-2'), level: 7, tier: 1 }), // Journeyman
						createTestAgent({ id: agentId('agent-3'), level: 8, tier: 1 }), // Journeyman
						createTestAgent({ id: agentId('agent-4'), level: 12, tier: 2 }) // Expert
					],
					quests: [],
					parties: [],
					guilds: [],
					battles: [],
					stats: defaultStats
				});

				const distribution = store.tierDistribution;
				expect(distribution).toHaveLength(5);
				expect(distribution[0].count).toBe(1); // Apprentice: 1
				expect(distribution[0].name).toBe('Apprentice');
				expect(distribution[1].count).toBe(2); // Journeyman: 2
				expect(distribution[1].name).toBe('Journeyman');
				expect(distribution[2].count).toBe(1); // Expert: 1
				expect(distribution[0].percentage).toBe(25);
				expect(distribution[1].percentage).toBe(50);
				expect(distribution[2].percentage).toBe(25);
			});

			it('handles empty agent list without division by zero', () => {
				store.setWorldState({
					agents: [],
					quests: [],
					parties: [],
					guilds: [],
					battles: [],
					stats: defaultStats
				});

				const distribution = store.tierDistribution;
				expect(distribution).toHaveLength(5);
				distribution.forEach((tier) => {
					expect(tier.count).toBe(0);
					expect(tier.percentage).toBe(0);
				});
			});

			it('includes all five tiers even when some are empty', () => {
				store.setWorldState({
					agents: [createTestAgent({ id: agentId('agent-1'), level: 19, tier: 4 })], // Grandmaster only
					quests: [],
					parties: [],
					guilds: [],
					battles: [],
					stats: defaultStats
				});

				const distribution = store.tierDistribution;
				expect(distribution).toHaveLength(5);
				expect(distribution[4].name).toBe('Grandmaster');
				expect(distribution[4].count).toBe(1);
				expect(distribution[4].percentage).toBe(100);
				// Other tiers should be 0
				expect(distribution[0].count).toBe(0);
				expect(distribution[1].count).toBe(0);
				expect(distribution[2].count).toBe(0);
				expect(distribution[3].count).toBe(0);
			});
		});

		describe('totalXpEarned', () => {
			it('sums total XP across all agents', () => {
				store.setWorldState({
					agents: [
						createTestAgent({
							id: agentId('agent-1'),
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
								quests_decomposed: 3
							}
						}),
						createTestAgent({
							id: agentId('agent-2'),
							stats: {
								quests_completed: 20,
								quests_failed: 2,
								bosses_defeated: 10,
								bosses_failed: 1,
								total_xp_earned: 2500,
								total_xp_spent: 100,
								avg_quality_score: 0.9,
								avg_efficiency: 0.85,
								parties_led: 5,
								quests_decomposed: 7
							}
						})
					],
					quests: [],
					parties: [],
					guilds: [],
					battles: [],
					stats: defaultStats
				});

				expect(store.totalXpEarned).toBe(3500);
			});

			it('returns 0 for empty agent list', () => {
				store.setWorldState({
					agents: [],
					quests: [],
					parties: [],
					guilds: [],
					battles: [],
					stats: defaultStats
				});

				expect(store.totalXpEarned).toBe(0);
			});
		});

		describe('battleStats', () => {
			it('calculates win rate correctly', () => {
				store.setWorldState({
					agents: [],
					quests: [],
					parties: [],
					guilds: [],
					battles: [
						createTestBattle({ id: battleId('b1'), status: 'victory' }),
						createTestBattle({ id: battleId('b2'), status: 'victory' }),
						createTestBattle({ id: battleId('b3'), status: 'victory' }),
						createTestBattle({ id: battleId('b4'), status: 'defeat' })
					],
					stats: defaultStats
				});

				const stats = store.battleStats;
				expect(stats.won).toBe(3);
				expect(stats.lost).toBe(1);
				expect(stats.winRate).toBe(75);
			});

			it('handles no battles without division by zero', () => {
				store.setWorldState({
					agents: [],
					quests: [],
					parties: [],
					guilds: [],
					battles: [],
					stats: defaultStats
				});

				const stats = store.battleStats;
				expect(stats.won).toBe(0);
				expect(stats.lost).toBe(0);
				expect(stats.winRate).toBe(0);
			});

			it('ignores active and retreat battles when calculating win rate', () => {
				store.setWorldState({
					agents: [],
					quests: [],
					parties: [],
					guilds: [],
					battles: [
						createTestBattle({ id: battleId('b1'), status: 'active' }),
						createTestBattle({ id: battleId('b2'), status: 'retreat' }),
						createTestBattle({ id: battleId('b3'), status: 'victory' })
					],
					stats: defaultStats
				});

				const stats = store.battleStats;
				expect(stats.won).toBe(1);
				expect(stats.lost).toBe(0);
				expect(stats.winRate).toBe(100);
			});

			it('calculates 50% win rate correctly', () => {
				store.setWorldState({
					agents: [],
					quests: [],
					parties: [],
					guilds: [],
					battles: [
						createTestBattle({ id: battleId('b1'), status: 'victory' }),
						createTestBattle({ id: battleId('b2'), status: 'defeat' })
					],
					stats: defaultStats
				});

				const stats = store.battleStats;
				expect(stats.won).toBe(1);
				expect(stats.lost).toBe(1);
				expect(stats.winRate).toBe(50);
			});
		});
	});

	describe('derived lists', () => {
		it('provides sorted agent list', () => {
			store.setWorldState({
				agents: [
					createTestAgent({ id: agentId('agent-3'), level: 10 }),
					createTestAgent({ id: agentId('agent-1'), level: 5 }),
					createTestAgent({ id: agentId('agent-2'), level: 15 })
				],
				quests: [],
				parties: [],
				guilds: [],
				battles: [],
				stats: defaultStats
			});

			const list = store.agentList;
			expect(list).toHaveLength(3);
			// List should be sorted by level descending
			expect(list[0].level).toBe(15);
			expect(list[1].level).toBe(10);
			expect(list[2].level).toBe(5);
		});

		it('provides quest list', () => {
			store.setWorldState({
				agents: [],
				quests: [
					createTestQuest({ id: questId('quest-1') }),
					createTestQuest({ id: questId('quest-2') })
				],
				parties: [],
				guilds: [],
				battles: [],
				stats: defaultStats
			});

			expect(store.questList).toHaveLength(2);
		});

		it('provides battle list', () => {
			store.setWorldState({
				agents: [],
				quests: [],
				parties: [],
				guilds: [],
				battles: [
					createTestBattle({ id: battleId('battle-1') }),
					createTestBattle({ id: battleId('battle-2') })
				],
				stats: defaultStats
			});

			expect(store.battleList).toHaveLength(2);
		});
	});
});
