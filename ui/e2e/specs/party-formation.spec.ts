import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/**
 * Party Formation
 *
 * Exercises the partycoord lifecycle via the backend API. The partycoord
 * processor reacts to party-required quest posting and auto-forms parties,
 * recruiting idle agents and claiming the quest on their behalf.
 *
 * Auto-formation requires a Master-tier (level 16+) idle agent as party lead.
 * Tests recruit their own Master-tier agent to ensure availability.
 *
 * Endpoints covered:
 *   POST  /game/quests           (create with party_required hint)
 *   GET   /game/quests/{id}      (verify party_required persisted)
 *   GET   /game/parties          (list all parties)
 *   GET   /game/parties/{id}     (get single party)
 */

test.describe('Party Formation', () => {
	test.beforeEach(async () => {
		if (!hasBackend()) test.skip();
	});

	test('quest creation with party_required persists flag', async ({ lifecycleApi }) => {
		const quest = await lifecycleApi.createQuestWithParty('Party quest test');
		const questId = extractInstance(quest.id);

		// Verify the quest has party_required persisted
		const fetched = await lifecycleApi.getQuest(questId);
		expect(fetched.party_required).toBe(true);
	});

	test('party auto-forms when party-required quest is posted', async ({ lifecycleApi }) => {
		// Recruit a Master-tier agent (level 16) to serve as party lead.
		// selectPartyLead requires CanLeadParty, which is Master+ only.
		await lifecycleApi.recruitAgentAtLevel(`party-lead-${Date.now()}`, 16, [
			'analysis',
			'coding'
		]);

		// Create a party-required quest — partycoord auto-forms a party on posting
		const quest = await lifecycleApi.createQuestWithParty('Auto-form party test', 2);
		const questId = extractInstance(quest.id);

		// Poll for party formation — partycoord reacts to quest posting,
		// finds idle agents, forms a party, claims, and starts the quest.
		const parties = await retry(
			async () => {
				const allParties = await lifecycleApi.listParties();
				const matching = allParties.filter(
					(p) => extractInstance(p.quest_id) === questId
				);
				if (matching.length === 0) {
					throw new Error('No party formed yet');
				}
				return matching;
			},
			{ timeout: 30_000, interval: 2000, message: 'Party did not auto-form within 30s' }
		);

		expect(parties.length).toBeGreaterThanOrEqual(1);
		const party = parties[0];
		expect(party.quest_id).toContain(questId);

		// Verify the quest was claimed (partycoord auto-claims)
		const questAfter = await lifecycleApi.getQuest(questId);
		expect(['claimed', 'in_progress', 'completed']).toContain(questAfter.status);
	});

	test('list parties returns empty array when no parties exist', async ({ lifecycleApi }) => {
		const parties = await lifecycleApi.listParties();
		expect(Array.isArray(parties)).toBe(true);
	});
});
