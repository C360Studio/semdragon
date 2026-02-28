/**
 * Unit tests for type utilities and branded IDs
 */

import { describe, it, expect } from 'vitest';
import {
	agentId,
	questId,
	partyId,
	guildId,
	battleId,
	TrustTierNames,
	QuestDifficultyNames,
	ReviewLevelNames,
	type AgentID,
	type QuestID,
	type TrustTier,
	type QuestDifficulty,
	type ReviewLevel
} from './index';

describe('Branded ID Functions', () => {
	describe('agentId', () => {
		it('creates a branded AgentID from string', () => {
			const id = agentId('agent-123');
			expect(id).toBe('agent-123');
			// The brand is compile-time only, but the value works as a string
			expect(typeof id).toBe('string');
		});

		it('preserves the original string value', () => {
			const original = 'agent-with-special-chars_123';
			const id = agentId(original);
			expect(id).toBe(original);
			expect(id.length).toBe(original.length);
		});
	});

	describe('questId', () => {
		it('creates a branded QuestID from string', () => {
			const id = questId('quest-456');
			expect(id).toBe('quest-456');
		});
	});

	describe('partyId', () => {
		it('creates a branded PartyID from string', () => {
			const id = partyId('party-789');
			expect(id).toBe('party-789');
		});
	});

	describe('guildId', () => {
		it('creates a branded GuildID from string', () => {
			const id = guildId('guild-abc');
			expect(id).toBe('guild-abc');
		});
	});

	describe('battleId', () => {
		it('creates a branded BattleID from string', () => {
			const id = battleId('battle-xyz');
			expect(id).toBe('battle-xyz');
		});
	});
});

describe('Trust Tier Names', () => {
	it('has all 5 tiers defined', () => {
		expect(Object.keys(TrustTierNames)).toHaveLength(5);
	});

	it('maps tier numbers to names', () => {
		expect(TrustTierNames[0]).toBe('Apprentice');
		expect(TrustTierNames[1]).toBe('Journeyman');
		expect(TrustTierNames[2]).toBe('Expert');
		expect(TrustTierNames[3]).toBe('Master');
		expect(TrustTierNames[4]).toBe('Grandmaster');
	});

	it('returns correct name for each TrustTier', () => {
		const tiers: TrustTier[] = [0, 1, 2, 3, 4];
		const expectedNames = ['Apprentice', 'Journeyman', 'Expert', 'Master', 'Grandmaster'];

		tiers.forEach((tier, index) => {
			expect(TrustTierNames[tier]).toBe(expectedNames[index]);
		});
	});
});

describe('Quest Difficulty Names', () => {
	it('has all 6 difficulties defined', () => {
		expect(Object.keys(QuestDifficultyNames)).toHaveLength(6);
	});

	it('maps difficulty numbers to names', () => {
		expect(QuestDifficultyNames[0]).toBe('Trivial');
		expect(QuestDifficultyNames[1]).toBe('Easy');
		expect(QuestDifficultyNames[2]).toBe('Moderate');
		expect(QuestDifficultyNames[3]).toBe('Hard');
		expect(QuestDifficultyNames[4]).toBe('Epic');
		expect(QuestDifficultyNames[5]).toBe('Legendary');
	});

	it('returns correct name for each QuestDifficulty', () => {
		const difficulties: QuestDifficulty[] = [0, 1, 2, 3, 4, 5];
		const expectedNames = ['Trivial', 'Easy', 'Moderate', 'Hard', 'Epic', 'Legendary'];

		difficulties.forEach((diff, index) => {
			expect(QuestDifficultyNames[diff]).toBe(expectedNames[index]);
		});
	});
});

describe('Review Level Names', () => {
	it('has all 4 review levels defined', () => {
		expect(Object.keys(ReviewLevelNames)).toHaveLength(4);
	});

	it('maps review level numbers to names', () => {
		expect(ReviewLevelNames[0]).toBe('Auto');
		expect(ReviewLevelNames[1]).toBe('Standard');
		expect(ReviewLevelNames[2]).toBe('Strict');
		expect(ReviewLevelNames[3]).toBe('Human');
	});

	it('returns correct name for each ReviewLevel', () => {
		const levels: ReviewLevel[] = [0, 1, 2, 3];
		const expectedNames = ['Auto', 'Standard', 'Strict', 'Human'];

		levels.forEach((level, index) => {
			expect(ReviewLevelNames[level]).toBe(expectedNames[index]);
		});
	});
});

describe('Type Safety', () => {
	it('branded IDs work with string operations', () => {
		const id = agentId('agent-test');

		// Should work with string methods
		expect(id.toUpperCase()).toBe('AGENT-TEST');
		expect(id.includes('test')).toBe(true);
		expect(id.split('-')).toEqual(['agent', 'test']);
	});

	it('branded IDs can be used as Map keys', () => {
		const map = new Map<AgentID, string>();
		const id1 = agentId('agent-1');
		const id2 = agentId('agent-2');

		map.set(id1, 'First Agent');
		map.set(id2, 'Second Agent');

		expect(map.get(id1)).toBe('First Agent');
		expect(map.get(id2)).toBe('Second Agent');
		expect(map.size).toBe(2);
	});

	it('branded IDs work with object property access', () => {
		const agents: Record<AgentID, string> = {} as Record<AgentID, string>;
		const id = agentId('agent-1');

		agents[id] = 'Test Agent';
		expect(agents[id]).toBe('Test Agent');
	});
});
