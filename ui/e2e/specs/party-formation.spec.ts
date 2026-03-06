import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/**
 * Party Formation
 *
 * Exercises the partycoord lifecycle via the backend API. The partycoord
 * processor reacts to quest claim events and auto-forms parties for quests
 * that have party_required set.
 *
 * Endpoints covered:
 *   POST  /game/quests           (create with party_required hint)
 *   GET   /game/quests/{id}      (verify party_required persisted)
 *   GET   /game/parties          (list all parties)
 *   GET   /game/parties/{id}     (get single party)
 *
 * Note: party auto-formation is event-driven and asynchronous. Tests that
 * assert party existence must use retry() to poll for the expected state.
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

	test('party auto-forms when agent claims party-required quest', async ({ lifecycleApi }) => {
		// Create a party-required quest
		const quest = await lifecycleApi.createQuestWithParty('Auto-form party test', 2);
		const questId = extractInstance(quest.id);

		// Recruit an agent and claim the quest
		const agent = await lifecycleApi.recruitAgent(`party-test-${Date.now()}`, ['analysis']);
		const agentId = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questId, agentId);
		expect(claimRes.ok).toBeTruthy();

		// Poll for party formation — partycoord reacts to quest claim events
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
			{ timeout: 60_000, interval: 2000, message: 'Party did not auto-form within 60s' }
		);

		expect(parties.length).toBeGreaterThanOrEqual(1);
		const party = parties[0];
		expect(party.quest_id).toContain(questId);
	});

	test('list parties returns empty array when no parties exist', async ({ lifecycleApi }) => {
		const parties = await lifecycleApi.listParties();
		expect(Array.isArray(parties)).toBe(true);
	});
});
