/**
 * Unit tests for API service
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { api, setApiUrl, getQuests, getAgents, getBattles, healthCheck } from './api';
import type { Quest, Agent, BossBattle, AgentStats, QuestConstraints, AgentConfig } from '$types';

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

	describe('getAgents', () => {
		it('returns list of agents', async () => {
			const mockAgents: Agent[] = [
				{
					id: 'agent-1' as Agent['id'],
					name: 'Test Agent',
					level: 5,
					xp: 100,
					xp_to_level: 500,
					tier: 1,
					status: 'idle',
					skills: ['code_generation'],
					equipment: [],
					guilds: [],
					death_count: 0,
					stats: createAgentStats(),
					config: createAgentConfig(),
					created_at: '2024-01-01T00:00:00Z',
					updated_at: '2024-01-01T00:00:00Z'
				}
			];

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => mockAgents
			});

			const result = await getAgents();

			expect(result).toEqual(mockAgents);
			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/agents',
				expect.any(Object)
			);
		});

		it('throws on network error', async () => {
			mockFetch.mockRejectedValueOnce(new Error('Network error'));

			await expect(getAgents()).rejects.toThrow('Network error');
		});
	});

	describe('getQuests', () => {
		it('returns list of quests without filters', async () => {
			const mockQuests: Quest[] = [
				{
					id: 'quest-1' as Quest['id'],
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
				}
			];

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => mockQuests
			});

			const result = await getQuests();

			expect(result).toEqual(mockQuests);
			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/quests',
				expect.any(Object)
			);
		});

		it('appends filters as query params', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => []
			});

			await getQuests({ status: 'posted', difficulty: 2 });

			expect(mockFetch).toHaveBeenCalledWith(
				expect.stringContaining('http://test-api.local/quests?'),
				expect.any(Object)
			);
			expect(mockFetch).toHaveBeenCalledWith(
				expect.stringContaining('status=posted'),
				expect.any(Object)
			);
			expect(mockFetch).toHaveBeenCalledWith(
				expect.stringContaining('difficulty=2'),
				expect.any(Object)
			);
		});
	});

	describe('getBattles', () => {
		it('returns list of battles', async () => {
			const mockBattles: BossBattle[] = [
				{
					id: 'battle-1' as BossBattle['id'],
					quest_id: 'quest-1' as Quest['id'],
					agent_id: 'agent-1' as Agent['id'],
					status: 'active',
					level: 1,
					criteria: [],
					results: [],
					judges: [{ type: 'llm', id: 'gpt-4', config: {} }],
					started_at: '2024-01-01T00:00:00Z'
				}
			];

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: async () => mockBattles
			});

			const result = await getBattles();

			expect(result).toEqual(mockBattles);
			expect(mockFetch).toHaveBeenCalledWith(
				'http://test-api.local/battles',
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

			await expect(getAgents()).rejects.toThrow('API Error 404: Not Found');
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
		expect(api.getQuests).toBeDefined();
		expect(api.getQuest).toBeDefined();
		expect(api.createQuest).toBeDefined();
		expect(api.getAgents).toBeDefined();
		expect(api.getAgent).toBeDefined();
		expect(api.recruitAgent).toBeDefined();
		expect(api.retireAgent).toBeDefined();
		expect(api.getBattles).toBeDefined();
		expect(api.getBattle).toBeDefined();
		expect(api.getTrajectory).toBeDefined();
		expect(api.sendDMChat).toBeDefined();
		expect(api.intervene).toBeDefined();
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
