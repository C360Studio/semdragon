import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';
import type { QuestResponse, AgentResponse } from '../fixtures/test-base';

/**
 * Party Quest DAG Execution E2E Tests
 *
 * Tests the full party quest pipeline: post a party-required quest,
 * observe autonomous DAG decomposition, member recruitment, sub-quest
 * assignment, lead review, rollup, and completion.
 *
 * Environment requirements:
 *   - E2E_OLLAMA=true (gate variable)
 *   - Ollama running with code-capable model
 *   - Backend running with questdagexec, partycoord, questbridge, autonomy enabled
 *
 * @integration @ollama
 */

// =============================================================================
// ENVIRONMENT GUARD
// =============================================================================

function hasOllama(): boolean {
	return process.env.E2E_OLLAMA === 'true';
}

// =============================================================================
// POLLING CONFIGURATION
// =============================================================================

const POLL = {
	questTerminal: { timeout: 540_000, interval: 5000 },
	agentIdle: { timeout: 60_000, interval: 2000 }
} as const;

const TERMINAL_STATUSES = new Set(['completed', 'failed']);

// =============================================================================
// GROUP: Party Quest DAG
// =============================================================================

test.describe('Party Quest DAG @integration @ollama', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasOllama(),
			'Requires running backend and Ollama (E2E_OLLAMA=true)'
		);
	});

	test('party quest tree completes autonomously via DAG', async ({ lifecycleApi }) => {
		test.setTimeout(600_000);

		// Seed: 1 Master (level 16), 2 Journeyman (level 8) with code_generation skills.
		// The lead agent must be level 16+ to hold the party-lead role and trigger DAG
		// decomposition. Executor agents only need Journeyman tier for code tasks.
		const lead = await lifecycleApi.recruitAgentAtLevel('dag-lead', 16, ['code_generation']);
		const exec1 = await lifecycleApi.recruitAgentAtLevel('dag-exec1', 8, ['code_generation']);
		const exec2 = await lifecycleApi.recruitAgentAtLevel('dag-exec2', 8, ['code_generation']);

		console.log(
			`[DAG E2E] Seeded agents: lead=${lead.id}, exec1=${exec1.id}, exec2=${exec2.id}`
		);

		// Post the parent quest marked as party_required with min 3 members.
		// The objective is split into two parallel sub-tasks so the DAG executor
		// can assign each leaf node to a separate agent before the lead rolls up.
		const parent = await lifecycleApi.createQuestWithParty(
			'Build a math module: implement add(a,b) and subtract(a,b) as '
				+ 'separate Go functions that take two integers and return an integer, '
				+ 'then combine them into a single math.go module.',
			3
		);
		expect(parent.id).toBeTruthy();
		console.log(`[DAG E2E] Posted parent quest: ${parent.id}`);

		// Wait for the parent quest to reach a terminal state. The full DAG
		// pipeline — decompose, recruit, execute sub-quests, lead review, rollup —
		// is autonomous. We only observe the final outcome.
		const finalParent = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(extractInstance(parent.id));
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Parent quest still ${q.status}`);
				}
				return q;
			},
			{
				...POLL.questTerminal,
				message: 'Parent quest did not reach a terminal state within 9 minutes'
			}
		);

		expect(finalParent.status).toBe('completed');
		console.log(`[DAG E2E] Parent quest completed`);

		// Verify every seeded agent returned to idle once all work is done.
		// An agent stuck in_progress or claiming indicates a pipeline leak.
		const agents: Array<AgentResponse> = [lead, exec1, exec2];
		for (const agent of agents) {
			const idle = await retry(
				async () => {
					const a = await lifecycleApi.getAgent(extractInstance(agent.id));
					if (a.status !== 'idle') {
						throw new Error(`Agent ${agent.id} still ${a.status}`);
					}
					return a;
				},
				{
					...POLL.agentIdle,
					message: `Agent ${agent.id} did not return to idle after DAG completion`
				}
			);
			expect(idle.status).toBe('idle');
		}
		console.log(`[DAG E2E] All agents returned to idle`);
	});
});
