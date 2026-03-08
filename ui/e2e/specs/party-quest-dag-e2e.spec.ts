import {
	test,
	expect,
	hasBackend,
	hasLLM,
	isRealLLM,
	isMockLLM,
	extractInstance,
	retry
} from '../fixtures/test-base';
import type { QuestResponse, AgentResponse, ReviewResponse } from '../fixtures/test-base';

/**
 * Party Quest DAG Execution E2E Tests
 *
 * Tests the full party quest pipeline: post a party-required quest,
 * observe autonomous DAG decomposition, member recruitment, sub-quest
 * assignment, lead review, rollup, and completion.
 *
 * Pipeline under test:
 *   POST quest (party_required) → partycoord forms party (lead + members)
 *     → lead claims quest → agentic loop decomposes into DAG
 *     → questdagexec creates sub-quests → assigns to members
 *     → each member executes sub-quest via agentic loop
 *     → lead reviews each sub-quest output (peer review)
 *     → lead produces rollup → parent quest completed
 *
 * Environment requirements:
 *   - E2E_BACKEND_AVAILABLE=true (set by global-setup.ts)
 *   - E2E_LLM_MODE=mock|gemini|openai|anthropic|ollama (mock or real LLM)
 *   - Backend running with questdagexec, partycoord, questbridge, autonomy enabled
 *
 * @integration
 */

// =============================================================================
// POLLING CONFIGURATION
// =============================================================================

/**
 * Poll settings scaled by provider. DAG execution involves multiple sequential
 * LLM calls: decomposition + N sub-quest executions + N lead reviews + rollup.
 * Budget conservatively for 6-8 LLM round-trips.
 * Mock LLM responds instantly, so use tighter timeouts.
 */
const POLL = isMockLLM()
	? {
			partyFormation: { timeout: 30_000, interval: 1000 },
			dagDecomposition: { timeout: 60_000, interval: 1000 },
			subQuestExecution: { timeout: 90_000, interval: 2000 },
			parentCompletion: { timeout: 120_000, interval: 2000 },
			agentIdle: { timeout: 30_000, interval: 1000 },
			reviewCreation: { timeout: 30_000, interval: 1000 }
		}
	: {
			partyFormation: { timeout: 30_000, interval: 2000 },
			dagDecomposition: { timeout: 120_000, interval: 3000 },
			subQuestExecution: { timeout: 180_000, interval: 5000 },
			parentCompletion: { timeout: 600_000, interval: 5000 },
			agentIdle: { timeout: 60_000, interval: 2000 },
			reviewCreation: { timeout: 30_000, interval: 2000 }
		};

const TERMINAL_STATUSES = new Set(['completed', 'failed', 'escalated']);

/**
 * Self-contained parent quest that decomposes cleanly into 2 parallel sub-tasks.
 * The description explicitly tells the LLM to split into sub-quests, giving it
 * enough structure to produce a DAG rather than requesting clarification.
 */
const DAG_PARENT_QUEST =
	'Build a utility library with two independent functions. ' +
	'Sub-task 1: Write a Python function `celsius_to_fahrenheit(c)` that converts Celsius to Fahrenheit (formula: F = C * 9/5 + 32), with a docstring and 2 test cases. ' +
	'Sub-task 2: Write a Python function `kilometers_to_miles(km)` that converts kilometers to miles (formula: miles = km * 0.621371), with a docstring and 2 test cases. ' +
	'Each sub-task should be completed independently, then combined into a single utils.py module.';

// =============================================================================
// PARTY QUEST DAG TESTS
// =============================================================================

test.describe('Party Quest DAG @integration', () => {
	// Serialize to avoid concurrent LLM contention.
	test.describe.configure({ mode: 'serial' });

	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend with LLM (real or mock)'
		);
	});

	test('party quest decomposes, executes sub-quests, and completes via DAG', async ({
		lifecycleApi
	}) => {
		test.setTimeout(isMockLLM() ? 180_000 : 600_000);

		// -----------------------------------------------------------------------
		// 1. Seed agents: 1 Master lead + 2 Journeyman executors
		// -----------------------------------------------------------------------

		const lead = await lifecycleApi.recruitAgentAtLevel('dag-lead', 16, ['code_generation']);
		const exec1 = await lifecycleApi.recruitAgentAtLevel('dag-exec1', 8, ['code_generation']);
		const exec2 = await lifecycleApi.recruitAgentAtLevel('dag-exec2', 8, ['code_generation']);

		const leadInstance = extractInstance(lead.id);
		const exec1Instance = extractInstance(exec1.id);
		const exec2Instance = extractInstance(exec2.id);
		const agentInstances = [leadInstance, exec1Instance, exec2Instance];

		console.log(`[DAG E2E] Seeded agents: lead=${leadInstance}, exec1=${exec1Instance}, exec2=${exec2Instance}`);

		// -----------------------------------------------------------------------
		// 2. Post party-required quest
		// -----------------------------------------------------------------------

		const parent = await lifecycleApi.createQuestWithParty(DAG_PARENT_QUEST, 3);
		expect(parent.id).toBeTruthy();
		const parentInstance = extractInstance(parent.id);
		console.log(`[DAG E2E] Posted parent quest: ${parentInstance}`);

		// -----------------------------------------------------------------------
		// 3. Verify party formation
		// -----------------------------------------------------------------------

		const party = await retry(
			async () => {
				const parties = await lifecycleApi.listParties();
				const match = parties.find(
					(p) => p.quest_id === parent.id || extractInstance(p.quest_id ?? '') === parentInstance
				);
				if (!match) throw new Error('No party formed yet');
				return match;
			},
			{
				...POLL.partyFormation,
				message: 'Party was not auto-formed for party-required quest'
			}
		);

		expect(party.id, 'party must have an ID').toBeTruthy();
		expect(party.lead, 'party must have a lead').toBeTruthy();
		console.log(`[DAG E2E] Party formed: ${extractInstance(party.id)}, lead=${extractInstance(party.lead)}`);

		// Party should have recruited members
		if (party.members) {
			console.log(`[DAG E2E] Party members: ${party.members.length}`);
			expect(party.members.length).toBeGreaterThanOrEqual(2);
		}

		// -----------------------------------------------------------------------
		// 4. Wait for parent quest to reach terminal state
		// -----------------------------------------------------------------------

		const finalParent = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(parentInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Parent quest still ${q.status}`);
				}
				return q;
			},
			{
				...POLL.parentCompletion,
				message: 'Parent quest did not reach terminal state within 10 minutes'
			}
		);

		console.log(`[DAG E2E] Parent quest reached: ${finalParent.status}`);

		if (finalParent.status === 'escalated') {
			test.skip(true, 'Parent quest escalated (LLM requested clarification) — DAG not triggered');
			return;
		}

		// -----------------------------------------------------------------------
		// 5. Verify sub-quests were created (DAG decomposition happened)
		// -----------------------------------------------------------------------

		const allQuests = await lifecycleApi.listQuests();

		// Find sub-quests: they reference the parent quest or the party
		const subQuests = allQuests.filter(
			(q) =>
				(q.parent_quest && extractInstance(q.parent_quest) === parentInstance) ||
				(q.party_id && party.id && extractInstance(q.party_id) === extractInstance(party.id) && q.id !== parent.id)
		);

		console.log(`[DAG E2E] Sub-quests found: ${subQuests.length}`);
		expect(subQuests.length, 'DAG should have decomposed into at least 2 sub-quests').toBeGreaterThanOrEqual(2);

		// All sub-quests should have reached terminal state
		for (const sq of subQuests) {
			const sqInstance = extractInstance(sq.id);
			expect(
				TERMINAL_STATUSES.has(sq.status),
				`Sub-quest ${sqInstance} should be terminal, got: ${sq.status}`
			).toBe(true);
			console.log(`[DAG E2E] Sub-quest ${sqInstance}: ${sq.status}`);
		}

		// -----------------------------------------------------------------------
		// 6. Verify parent completed (not just any terminal state)
		// -----------------------------------------------------------------------

		expect(
			finalParent.status,
			`Parent quest should be completed after successful DAG execution, got: ${finalParent.status}`
		).toBe('completed');

		// -----------------------------------------------------------------------
		// 7. Verify all agents returned to idle
		// -----------------------------------------------------------------------

		for (const agentId of agentInstances) {
			const idleAgent = await retry(
				async () => {
					const a = await lifecycleApi.getAgent(agentId);
					if (a.status !== 'idle') {
						throw new Error(`Agent ${agentId} still ${a.status}`);
					}
					return a;
				},
				{
					...POLL.agentIdle,
					message: `Agent ${agentId} did not return to idle after DAG completion`
				}
			);
			expect(idleAgent.status).toBe('idle');
		}
		console.log('[DAG E2E] All agents returned to idle');

		// -----------------------------------------------------------------------
		// 8. Verify peer reviews exist for all party members
		// -----------------------------------------------------------------------
		// The lead reviews each member's sub-quest output. questdagexec creates
		// PeerReview entities on both accept and reject. Every agent who worked
		// a sub-quest should have at least one review referencing them.

		// Get the actual party members (non-lead) to check reviews.
		// The party may contain seeded agents, not necessarily our test agents.
		const partyLeadInstance = extractInstance(party.lead);
		const partyMembers = (party.members ?? [])
			.map((m: any) => extractInstance(m.agent_id ?? m.id ?? ''))
			.filter((id: string) => id && id !== partyLeadInstance);

		console.log(`[DAG E2E] Checking reviews for party members: ${partyMembers.join(', ')}`);

		for (const agentId of partyMembers) {
			const reviews = await retry(
				async () => {
					const r = await lifecycleApi.getAgentReviews(agentId);
					if (!r || r.length === 0) {
						throw new Error(`No reviews found for agent ${agentId}`);
					}
					return r;
				},
				{
					...POLL.reviewCreation,
					message: `Agent ${agentId} has no peer review entities after DAG completion`
				}
			);

			expect(reviews.length, `Agent ${agentId} should have at least 1 peer review`).toBeGreaterThanOrEqual(1);

			for (const review of reviews) {
				console.log(
					`[DAG E2E] Review for ${agentId}: id=${extractInstance(review.id)}, ` +
					`status=${review.status}, leader_avg=${review.leader_avg_rating ?? 'pending'}`
				);
			}
		}

		// Lead agent reviews — informational only, don't hard-fail.
		const leadReviews = await lifecycleApi.getAgentReviews(partyLeadInstance);
		console.log(`[DAG E2E] Lead reviews: ${leadReviews.length}`);

		console.log('[DAG E2E] All peer reviews verified');
	});
});
