/**
 * Unit tests for API service
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
	api,
	setApiUrl,
	healthCheck,
	getQuest,
	createQuest,
	getAgent,
	recruitAgent,
	retireAgent,
	getBattle,
	getTrajectory,
	sendDMChat,
	intervene,
	getStoreItems,
	getStoreItem,
	getInventory,
	purchase,
	useConsumable,
	getActiveEffects,
	getWorldState
} from './api';
import type { Quest, Agent, BossBattle, AgentStats, QuestConstraints, AgentConfig } from '$types';
import { agentId, questId, battleId } from '$types';

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Helper for complete agent stats
function createAgentStats(): AgentStats {
	return {
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
	};
}

// Helper for agent config
function createAgentConfig(): AgentConfig {
	return {
		provider: 'openai',
		model: 'gpt-4',
		system_prompt: 'You are a helpful assistant.',
		temperature: 0.7,
		max_tokens: 4096,
		metadata: {}
	};
}

// Helper for quest constraints
function createQuestConstraints(): QuestConstraints {
	return {
		max_duration: 3600000000000,
		max_cost: 1.0,
		max_tokens: 100000,
		require_review: true,
		review_level: 1
	};
}

describe('API Service', () => {
	beforeEach(() => {
		mockFetch.mockReset();
		setApiUrl('http://test-api.local');
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	describe('setApiUrl', () => {
		it('changes the base URL for API calls', async () => {
			setApiUrl('http://custom-api.local:9000');

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ status: 'ok' })
			});

			await healthCheck();

			expect(mockFetch).toHaveBeenCalledWith(
				'http://custom-api.local:9000/health',
				expect.any(Object)
			);
		});
	});

	describe('healthCheck', () => {
		it('returns health status on success', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ status: 'healthy' })
			});

			const result = await healthCheck();

			expect(result).toEqual({ status: 'healthy' });
			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/health',
				expect.objectContaining({
					headers: expect.objectContaining({
						'Content-Type': 'application/json'
					})
				})
			);
		});

		it('throws error on failure', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 500,
				text: async () => 'Internal Server Error'
			});

			await expect(healthCheck()).rejects.toThrow('API Error 500: Internal Server Error');
		});
	});

	describe('getWorldState (fallback)', () => {
		it('fetches world state from /game/world', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({
					agents: [],
					quests: [],
					parties: [],
					guilds: [],
					battles: [],
					stats: {}
				})
			});

			await getWorldState();

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/world',
				expect.any(Object)
			);
		});
	});

	describe('getQuest', () => {
		it('fetches a quest by ID from /game/quests/{id}', async () => {
			const mockQuest: Quest = {
				id: questId('quest-1'),
				title: 'Test Quest',
				description: 'A test quest',
				status: 'posted',
				difficulty: 2,
				base_xp: 100,
				bonus_xp: 0,
				guild_xp: 0,
				required_skills: [],
				required_tools: [],
				min_tier: 0,
				max_attempts: 3,
				attempts: 0,
				party_required: false,
				min_party_size: 1,
				sub_quests: [],
				escalated: false,
				input: null,
				constraints: createQuestConstraints(),
				posted_at: '2024-01-01T00:00:00Z',
				trajectory_id: 'traj-1'
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => mockQuest
			});

			const result = await getQuest(questId('quest-1'));

			expect(result).toEqual(mockQuest);
			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/quests/quest-1',
				expect.any(Object)
			);
		});
	});

	describe('createQuest', () => {
		it('posts a new quest to /game/quests', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ id: 'quest-new', title: 'New Quest' })
			});

			await createQuest('Build a dashboard');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/quests',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify({ objective: 'Build a dashboard' })
				})
			);
		});

		it('includes hints when provided', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ id: 'quest-new' })
			});

			await createQuest('Build a dashboard', {
				suggested_difficulty: 3,
				require_human_review: true,
				budget: 100
			});

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/quests',
				expect.objectContaining({
					body: JSON.stringify({
						objective: 'Build a dashboard',
						hints: {
							suggested_difficulty: 3,
							require_human_review: true,
							budget: 100
						}
					})
				})
			);
		});
	});

	describe('getAgent', () => {
		it('fetches an agent by ID from /game/agents/{id}', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ id: 'agent-1', name: 'Test Agent' })
			});

			await getAgent(agentId('agent-1'));

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/agents/agent-1',
				expect.any(Object)
			);
		});
	});

	describe('recruitAgent', () => {
		it('posts to /game/agents', async () => {
			const config: AgentConfig = createAgentConfig();

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ id: 'agent-new', name: 'New Agent' })
			});

			await recruitAgent(config);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/agents',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify(config)
				})
			);
		});
	});

	describe('retireAgent', () => {
		it('posts to /game/agents/{id}/retire', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({})
			});

			await retireAgent(agentId('agent-1'), 'No longer needed');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/agents/agent-1/retire',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify({ reason: 'No longer needed' })
				})
			);
		});
	});

	describe('getBattle', () => {
		it('fetches a battle by ID from /game/battles/{id}', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ id: 'battle-1', status: 'active' })
			});

			await getBattle(battleId('battle-1'));

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/battles/battle-1',
				expect.any(Object)
			);
		});
	});

	describe('getTrajectory', () => {
		it('fetches trajectory events from /game/trajectories/{id}', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => [{ timestamp: 123, type: 'event', data: {} }]
			});

			const result = await getTrajectory('traj-1');

			expect(result).toHaveLength(1);
			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/trajectories/traj-1',
				expect.any(Object)
			);
		});
	});

	describe('sendDMChat', () => {
		it('posts to /game/dm/chat', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ message: 'Hello!' })
			});

			await sendDMChat('Hello');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/dm/chat',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify({ message: 'Hello' })
				})
			);
		});
	});

	describe('intervene', () => {
		it('posts to /game/dm/intervene/{questId}', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({})
			});

			await intervene(questId('quest-1'), { type: 'assist', reason: 'Needs help' });

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/dm/intervene/quest-1',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify({ type: 'assist', reason: 'Needs help' })
				})
			);
		});
	});

	describe('store endpoints', () => {
		it('fetches store items from /game/store', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => [{ id: 'item-1', name: 'Sword' }]
			});

			await getStoreItems(agentId('agent-1'));

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/store?agent_id=agent-1',
				expect.any(Object)
			);
		});

		it('fetches a store item by ID from /game/store/{id}', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ id: 'item-1', name: 'Sword' })
			});

			await getStoreItem('item-1');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/store/item-1',
				expect.any(Object)
			);
		});

		it('fetches inventory from /game/agents/{id}/inventory', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ agent_id: 'agent-1', owned_tools: {}, consumables: {}, total_spent: 0 })
			});

			await getInventory(agentId('agent-1'));

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/agents/agent-1/inventory',
				expect.any(Object)
			);
		});

		it('posts purchase to /game/store/purchase', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ success: true })
			});

			await purchase({ agent_id: agentId('agent-1'), item_id: 'item-1' });

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/store/purchase',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify({ agent_id: 'agent-1', item_id: 'item-1' })
				})
			);
		});

		it('posts consumable use to /game/agents/{id}/inventory/use', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ success: true, remaining: 2, active_effects: [] })
			});

			await useConsumable({
				agent_id: agentId('agent-1'),
				consumable_id: 'potion-1',
				quest_id: questId('quest-1')
			});

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/agents/agent-1/inventory/use',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify({ consumable_id: 'potion-1', quest_id: 'quest-1' })
				})
			);
		});

		it('fetches active effects from /game/agents/{id}/effects', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => []
			});

			await getActiveEffects(agentId('agent-1'));

			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/game/agents/agent-1/effects',
				expect.any(Object)
			);
		});
	});

	describe('error handling', () => {
		it('includes status code in error message', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 404,
				text: async () => 'Not Found'
			});

			await expect(getAgent(agentId('agent-1'))).rejects.toThrow('API Error 404: Not Found');
		});

		it('handles empty error body', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 503,
				text: async () => ''
			});

			await expect(healthCheck()).rejects.toThrow('API Error 503: ');
		});

		it('handles JSON parse errors gracefully', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => {
					throw new SyntaxError('Unexpected token');
				}
			});

			await expect(healthCheck()).rejects.toThrow('Unexpected token');
		});

		it('throws on network error', async () => {
			mockFetch.mockRejectedValueOnce(new Error('Network error'));

			await expect(getAgent(agentId('agent-1'))).rejects.toThrow('Network error');
		});
	});
});

describe('API Service Object', () => {
	beforeEach(() => {
		mockFetch.mockReset();
		setApiUrl('http://test-api.local');
	});

	it('exports all API methods', () => {
		expect(api.setApiUrl).toBeDefined();
		expect(api.getWorldState).toBeDefined();
		expect(api.getQuest).toBeDefined();
		expect(api.createQuest).toBeDefined();
		expect(api.getAgent).toBeDefined();
		expect(api.recruitAgent).toBeDefined();
		expect(api.retireAgent).toBeDefined();
		expect(api.getBattle).toBeDefined();
		expect(api.getTrajectory).toBeDefined();
		expect(api.sendDMChat).toBeDefined();
		expect(api.intervene).toBeDefined();
		expect(api.getStoreItems).toBeDefined();
		expect(api.getStoreItem).toBeDefined();
		expect(api.getInventory).toBeDefined();
		expect(api.purchase).toBeDefined();
		expect(api.useConsumable).toBeDefined();
		expect(api.getActiveEffects).toBeDefined();
		expect(api.healthCheck).toBeDefined();
	});

	it('api methods work via exported object', async () => {
		mockFetch.mockResolvedValueOnce({
			ok: true,
			json: async () => ({ status: 'ok' })
		});

		const result = await api.healthCheck();

		expect(result).toEqual({ status: 'ok' });
	});
});
