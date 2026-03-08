import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';
import type {
	QuestResponse,
	AgentResponse,
	WorldStateResponse,
	TrajectoryResponse,
	BattleResponse
} from '../fixtures/test-base';

/**
 * Ollama E2E Integration Tests
 *
 * These tests exercise the full agentic pipeline against a real LLM
 * (Ollama with qwen2.5-coder:7b) running on the host machine.
 *
 * Group A: Basic validation — verify the pipeline works with a real LLM.
 * Group B: Semspec comparison — replicate semspec's hello-world scenario
 *          (3 quests, 3 seeded agents) for direct approach comparison.
 *
 * Environment requirements:
 *   - E2E_OLLAMA=true (gate variable)
 *   - Ollama running on host with qwen2.5-coder:7b pulled
 *   - Backend configured with semdragons-e2e-ollama.json
 *   - SEED_E2E=true for Group B (seeded agents)
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
// POLLING CONFIGURATION — generous timeouts for real LLM inference
// =============================================================================

const OLLAMA_POLL = {
	// All quests go through boss battle review (LLM judge), adding ~2 min per quest.
	questExecution: { timeout: 420_000, interval: 3000 },
	agentIdle: { timeout: 60_000, interval: 2000 }
} as const;

// in_review is NOT terminal — boss battle must resolve before quest completes.
// escalated IS terminal — quest needs DM intervention (e.g., clarification output).
const TERMINAL_STATUSES = new Set(['completed', 'failed', 'escalated']);

/**
 * Compute step-level token sums from a trajectory.
 *
 * Note: trajectory.total_tokens_in/out are currently 0 due to a semstreams
 * aggregation bug in Trajectory.Complete(). Use step sums instead.
 */
function trajectoryTokenSums(trajectory: TrajectoryResponse) {
	const tokensIn = trajectory.steps.reduce((sum, s) => sum + (s.tokens_in ?? 0), 0);
	const tokensOut = trajectory.steps.reduce((sum, s) => sum + (s.tokens_out ?? 0), 0);
	return { tokensIn, tokensOut };
}

/**
 * Check if a trajectory has a model_call step with a non-empty response.
 *
 * Note: agentic-loop splits model calls into two steps with the same request_id:
 * one with `prompt` (request) and one with `response` + tokens (completion).
 * We look for any model_call step with a response.
 */
function hasModelResponse(trajectory: TrajectoryResponse): boolean {
	return trajectory.steps.some(
		(s) => s.step_type === 'model_call' && s.response
	);
}

// =============================================================================
// HELPERS
// =============================================================================

const API_URL = process.env.API_URL || 'http://localhost:8080';

/**
 * Find a seeded agent by display name from the world state.
 * Seeded agents have random instance IDs, so we look them up by name.
 */
async function findAgentByName(
	getWorldState: () => Promise<WorldStateResponse>,
	name: string
): Promise<AgentResponse> {
	const world = await getWorldState();
	const agent = world.agents?.find((a) => a.name === name);
	if (!agent) {
		throw new Error(
			`Seeded agent "${name}" not found in world state — is SEED_E2E=true? ` +
				`Available agents: ${world.agents?.map((a) => a.name).join(', ') ?? 'none'}`
		);
	}
	return agent;
}

/**
 * Create a quest with specific skills via direct API call.
 * Extends the standard createQuest with suggested_skills support.
 */
async function createQuestWithSkills(
	objective: string,
	difficulty: number,
	skills: string[]
): Promise<QuestResponse> {
	const res = await fetch(`${API_URL}/game/quests`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			objective,
			hints: {
				suggested_difficulty: difficulty,
				suggested_skills: skills,
				require_human_review: false,
				budget: 200
			}
		})
	});
	if (!res.ok) {
		throw new Error(`createQuestWithSkills failed: ${res.status} ${await res.text()}`);
	}
	return res.json();
}

// =============================================================================
// CODEBASE CONTEXT — embedded in quest objectives (matches semspec fixture)
// =============================================================================

const CODEBASE_CONTEXT = `
Existing codebase for context:

\`\`\`python
# api/app.py
from flask import Flask, jsonify
app = Flask(__name__)

@app.route("/hello")
def hello():
    return jsonify({"message": "Hello World"})

if __name__ == "__main__":
    app.run(port=5000)
\`\`\`

\`\`\`javascript
// ui/app.js
async function loadGreeting() {
  const response = await fetch("/hello");
  const data = await response.json();
  document.getElementById("greeting").textContent = data.message;
}
loadGreeting();
\`\`\`
`;

// =============================================================================
// GROUP A: Basic Ollama Validation
// =============================================================================

// Run Ollama tests serially — a single Ollama instance cannot handle parallel
// agentic loops + boss battle LLM judges without severe contention.
test.describe.configure({ mode: 'serial' });

test.describe('Ollama Basic Validation @integration @ollama', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasOllama(),
			'Requires running backend (E2E_BACKEND_AVAILABLE=true) and Ollama (E2E_OLLAMA=true)'
		);
	});

	test('apprentice quest completes with real LLM', async ({ lifecycleApi }) => {
		// Fresh level-1 agent, no tools, single-shot text question.
		// Validates the full pipeline works with a real LLM response.
		test.setTimeout(480_000);

		const quest = await lifecycleApi.createQuest(
			'Summarize in one sentence: what is the purpose of unit testing?',
			1
		);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('ollama-apprentice-test');
		expect(agent.id).toBeTruthy();
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status} — waiting for Ollama completion`);
				}
				return q;
			},
			{
				...OLLAMA_POLL.questExecution,
				message:
					'Quest did not reach terminal status within 5 minutes. ' +
					'Check that Ollama is running on the host and qwen2.5-coder:7b is available.'
			}
		);

		// Quest goes through boss battle review — may complete, fail, or escalate
		// (escalation happens when the LLM outputs clarification instead of work product).
		expect(
			TERMINAL_STATUSES.has(finalQuest.status),
			`Expected terminal status, got ${finalQuest.status}`
		).toBe(true);

		if (finalQuest.status === 'completed') {
			expect(finalQuest.completed_at).toBeTruthy();
			const output = (finalQuest as QuestResponse & { output?: string }).output;
			expect(output, 'quest output must be populated from real LLM').toBeTruthy();
			expect(
				typeof output === 'string' && output.length > 20,
				'output should be substantive (>20 chars)'
			).toBe(true);
			console.log('[Ollama] Apprentice quest output:', output);
		} else {
			console.log('[Ollama] Apprentice quest failed boss battle (expected with 7B model)');
		}

		// Trajectory validation via loop_id
		expect(finalQuest.loop_id, 'quest must have loop_id after terminal state').toBeTruthy();

		const trajectory = await lifecycleApi.getTrajectory(finalQuest.loop_id!);
		expect(
			trajectory.outcome === 'complete' || trajectory.outcome === 'failed',
			`Expected trajectory outcome complete or failed, got ${trajectory.outcome}`
		).toBe(true);
		expect(trajectory.steps.length).toBeGreaterThanOrEqual(1);
		expect(
			trajectory.steps.some((s) => s.step_type === 'model_call'),
			'trajectory must have at least one model_call step'
		).toBe(true);

		// Step-level token sums (trajectory-level totals have a semstreams aggregation bug)
		const { tokensIn, tokensOut } = trajectoryTokenSums(trajectory);
		expect(tokensIn, 'step-level tokens_in sum').toBeGreaterThan(0);
		expect(tokensOut, 'step-level tokens_out sum').toBeGreaterThan(0);
		expect(trajectory.duration).toBeGreaterThan(0);
		expect(trajectory.start_time).toBeTruthy();
		expect(trajectory.end_time).toBeTruthy();

		// Model call response step should have content (split into request/response steps)
		expect(hasModelResponse(trajectory), 'trajectory must have a model response step').toBe(true);

		console.log(
			`[Ollama] Trajectory: ${trajectory.steps.length} steps, ` +
				`${tokensIn}+${tokensOut} tokens, ` +
				`${trajectory.duration}ms`
		);
	});

	test('agent returns to idle after Ollama quest completes', async ({ lifecycleApi }) => {
		// Same pipeline, verify agent lifecycle reset.
		test.setTimeout(480_000);

		const quest = await lifecycleApi.createQuest(
			'What is the difference between a list and a tuple in Python?',
			1
		);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('ollama-idle-return-test');
		const agentInstance = extractInstance(agent.id);

		const initialAgent = await lifecycleApi.getAgent(agentInstance);
		expect(initialAgent.status).toBe('idle');

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok).toBeTruthy();

		// Wait for quest completion
		await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest status: ${q.status}`);
				}
			},
			{
				...OLLAMA_POLL.questExecution,
				message: 'Quest did not complete via Ollama'
			}
		);

		// Verify quest reached terminal state (completed, failed, or escalated after boss battle)
		const finalQuest = await lifecycleApi.getQuest(questInstance);
		expect(
			TERMINAL_STATUSES.has(finalQuest.status),
			`Expected terminal status, got ${finalQuest.status}`
		).toBe(true);

		if (finalQuest.status === 'completed') {
			expect(finalQuest.completed_at).toBeTruthy();
			const completedDate = new Date(finalQuest.completed_at!);
			expect(completedDate.getTime()).toBeGreaterThan(0);
		}

		// Verify agent lifecycle after quest terminal state.
		// - completed/failed: agent returns to idle or cooldown
		// - escalated: agent stays on_quest (awaiting DM clarification)
		const finalAgent = await retry(
			async () => {
				const a = await lifecycleApi.getAgent(agentInstance);
				if (finalQuest.status === 'escalated') {
					// Escalated quests keep agent assigned — not a failure
					if (a.status !== 'on_quest') {
						throw new Error(`Escalated quest but agent is ${a.status}, expected on_quest`);
					}
				} else {
					if (a.status !== 'idle' && a.status !== 'cooldown') {
						throw new Error(`Agent still ${a.status}`);
					}
				}
				return a;
			},
			{
				...OLLAMA_POLL.agentIdle,
				message: 'Agent did not reach expected status after quest completion'
			}
		);

		if (finalQuest.status === 'escalated') {
			expect(finalAgent.status).toBe('on_quest');
			console.log('[Ollama] Agent stays on_quest (escalation — awaiting DM)');
		} else {
			expect(
				finalAgent.status === 'idle' || finalAgent.status === 'cooldown',
				`agent should be idle or cooldown, got ${finalAgent.status}`
			).toBe(true);
		}
		// XP may or may not trigger a level-up depending on difficulty/base XP
		expect(
			finalAgent.level,
			'agent level should not decrease after quest'
		).toBeGreaterThanOrEqual(initialAgent.level);
	});

	test('skilled apprentice gets no tools (tier gating with real LLM)', async ({
		lifecycleApi
	}) => {
		// Agent with skills but Apprentice tier (level 1-5).
		// Should complete without tool access — validates tier gating works with real LLM.
		test.setTimeout(480_000);

		const quest = await lifecycleApi.createQuest(
			'Explain the concept of dependency injection in 2-3 sentences.',
			1
		);
		const questInstance = extractInstance(quest.id);

		// Recruit with skills — but level 1 means Apprentice tier = no tools
		const agent = await lifecycleApi.recruitAgent('ollama-skilled-apprentice', [
			'code_gen',
			'analysis'
		]);
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok).toBeTruthy();

		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status}`);
				}
				return q;
			},
			{
				...OLLAMA_POLL.questExecution,
				message: 'Skilled apprentice quest did not complete'
			}
		);

		// Quest should reach terminal state after boss battle — not hang
		expect(
			TERMINAL_STATUSES.has(finalQuest.status),
			`Expected terminal status after boss battle, got ${finalQuest.status}`
		).toBe(true);

		if (finalQuest.status === 'completed') {
			const output = (finalQuest as QuestResponse & { output?: string }).output;
			expect(output).toBeTruthy();
			console.log('[Ollama] Skilled apprentice output:', output);
		}

		// Trajectory should have no tool_call steps — apprentice tier has no tool access
		if (finalQuest.loop_id) {
			const trajectory = await lifecycleApi.getTrajectory(finalQuest.loop_id);
			const toolCalls = trajectory.steps.filter((s) => s.step_type === 'tool_call');
			expect(
				toolCalls.length,
				'apprentice-tier agent should have zero tool_call steps'
			).toBe(0);

			const modelCalls = trajectory.steps.filter((s) => s.step_type === 'model_call');
			expect(modelCalls.length, 'should have at least one model_call').toBeGreaterThanOrEqual(1);
		}
	});
});

// =============================================================================
// GROUP B: Semspec Hello-World Comparison
// =============================================================================

test.describe('Semspec Hello-World Comparison @integration @ollama', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasOllama(),
			'Requires running backend (E2E_BACKEND_AVAILABLE=true) and Ollama (E2E_OLLAMA=true)'
		);
	});

	test('3 quests complete with seeded agents (goodbye endpoint scenario)', async ({
		lifecycleApi
	}) => {
		// Replicate semspec's hello-world scenario:
		//   "add a /goodbye endpoint that returns a goodbye message and display it in the UI"
		//
		// Semspec decomposes this into 3 tasks via its planner. We manually create
		// 3 quests matching that decomposition and assign them to seeded agents
		// at appropriate tier/skill levels.
		//
		// This enables direct comparison: semspec's structured OODA workflow vs
		// semdragons' gaming/quest approach — same objective, same model.
		test.setTimeout(900_000); // 15 minutes for 3 concurrent LLM calls + boss battle judges

		// ---------------------------------------------------------------
		// 1. Find seeded agents by name
		// ---------------------------------------------------------------

		const [coderAgent, analystAgent, seniorAgent] = await Promise.all([
			findAgentByName(
				() => lifecycleApi.getWorldState(),
				'coder-journeyman'
			),
			findAgentByName(
				() => lifecycleApi.getWorldState(),
				'analyst-1'
			),
			findAgentByName(
				() => lifecycleApi.getWorldState(),
				'senior-dev'
			)
		]);

		console.log('[Semspec Comparison] Agents found:');
		console.log(`  coder-journeyman: id=${coderAgent.id}, level=${coderAgent.level}, tier=${coderAgent.tier}`);
		console.log(`  analyst-1: id=${analystAgent.id}, level=${analystAgent.level}, tier=${analystAgent.tier}`);
		console.log(`  senior-dev: id=${seniorAgent.id}, level=${seniorAgent.level}, tier=${seniorAgent.tier}`);

		const coderInstance = extractInstance(coderAgent.id);
		const analystInstance = extractInstance(analystAgent.id);
		const seniorInstance = extractInstance(seniorAgent.id);

		// ---------------------------------------------------------------
		// 1b. Wait for all seeded agents to be idle
		// ---------------------------------------------------------------
		// Prior tests or autonomy may have assigned quests to these agents.
		// We must wait for them to finish before we can claim new quests.

		for (const [name, instance, fullId] of [
			['coder-journeyman', coderInstance, coderAgent.id],
			['analyst-1', analystInstance, analystAgent.id],
			['senior-dev', seniorInstance, seniorAgent.id]
		] as const) {
			await retry(
				async () => {
					const a = await lifecycleApi.getAgent(instance);
					if (a.status === 'idle' || a.status === 'cooldown') return;
					// Defense: if agent is stuck on_quest, abandon the stale quest.
					if (a.status === 'on_quest') {
						const world = await lifecycleApi.getWorldState();
						const staleQuest = (world.quests ?? []).find(
							(q: Record<string, unknown>) =>
								q.claimed_by === fullId &&
								(q.status === 'escalated' || q.status === 'in_progress' ||
								 q.status === 'in_review' || q.status === 'claimed')
						);
						if (staleQuest) {
							console.log(`[Semspec] Abandoning stale quest ${staleQuest.id} (${staleQuest.status}) for ${name}`);
							await lifecycleApi.abandonQuest(
								extractInstance(staleQuest.id),
								'Freed by E2E test setup'
							);
						}
					}
					throw new Error(`${name} is ${a.status}, waiting for idle/cooldown`);
				},
				{
					timeout: 120_000,
					interval: 5000,
					message: `${name} did not return to idle — may be stuck on a quest`
				}
			);
		}

		console.log('[Semspec Comparison] All agents idle, creating quests...');

		// ---------------------------------------------------------------
		// 2. Create 3 quests matching semspec's task decomposition
		// ---------------------------------------------------------------

		const backendObjective =
			'Add a /goodbye endpoint to api/app.py that returns JSON {"message": "Goodbye World"} ' +
			'following the existing /hello pattern. Return only the complete updated file content.' +
			'\n\n' +
			CODEBASE_CONTEXT;

		const frontendObjective =
			'Update ui/app.js to also fetch the goodbye message from /goodbye and display it ' +
			'alongside the hello message. Add a new function loadGoodbye() following the loadGreeting() pattern, ' +
			'and add a new element display. Return only the complete updated file content.' +
			'\n\n' +
			CODEBASE_CONTEXT;

		const testingObjective =
			'Write pytest tests for the /goodbye endpoint. The tests should verify: ' +
			'1) The endpoint returns HTTP 200, ' +
			'2) The response is valid JSON, ' +
			'3) The JSON contains {"message": "Goodbye World"}. ' +
			'Follow standard pytest conventions with a test client fixture. ' +
			'Return only the complete test file content.' +
			'\n\n' +
			CODEBASE_CONTEXT;

		// Create each quest and claim it immediately. If autonomy auto-claims
		// the quest before us (409), create a new quest and retry.
		async function createAndClaim(
			objective: string,
			difficulty: number,
			skills: string[],
			agentInstance: string,
			label: string
		): Promise<{ quest: QuestResponse; questInstance: string }> {
			for (let attempt = 0; attempt < 5; attempt++) {
				const quest = await createQuestWithSkills(objective, difficulty, skills);
				const questInstance = extractInstance(quest.id);
				const claim = await lifecycleApi.claimQuest(questInstance, agentInstance);
				if (claim.ok) return { quest, questInstance };
				console.log(`[Semspec Comparison] ${label} claim attempt ${attempt + 1} got 409, retrying with new quest`);
			}
			throw new Error(`${label} claim failed after 5 attempts — autonomy keeps auto-claiming`);
		}

		const { quest: backendQuest, questInstance: backendInstance } = await createAndClaim(
			backendObjective, 2, ['code_generation'], coderInstance, 'Backend'
		);
		const { quest: frontendQuest, questInstance: frontendInstance } = await createAndClaim(
			frontendObjective, 2, ['analysis'], analystInstance, 'Frontend'
		);
		const { quest: testingQuest, questInstance: testingInstance } = await createAndClaim(
			testingObjective, 3, ['code_generation', 'code_review'], seniorInstance, 'Testing'
		);

		console.log('[Semspec Comparison] Quests created and claimed:');
		console.log(`  Backend: ${backendQuest.id}`);
		console.log(`  Frontend: ${frontendQuest.id}`);
		console.log(`  Testing: ${testingQuest.id}`);

		// ---------------------------------------------------------------
		// 4. Start all 3 quests (triggers 3 concurrent agentic loops)
		// ---------------------------------------------------------------

		const [start1, start2, start3] = await Promise.all([
			lifecycleApi.startQuest(backendInstance),
			lifecycleApi.startQuest(frontendInstance),
			lifecycleApi.startQuest(testingInstance)
		]);

		expect(start1.ok, `Backend start failed: ${start1.status}`).toBeTruthy();
		expect(start2.ok, `Frontend start failed: ${start2.status}`).toBeTruthy();
		expect(start3.ok, `Testing start failed: ${start3.status}`).toBeTruthy();

		console.log('[Semspec Comparison] All 3 quests started — waiting for Ollama completions...');

		// ---------------------------------------------------------------
		// 5. Poll all 3 until terminal state
		// ---------------------------------------------------------------

		const [finalBackend, finalFrontend, finalTesting] = await Promise.all([
			retry(
				async () => {
					const q = await lifecycleApi.getQuest(backendInstance);
					if (!TERMINAL_STATUSES.has(q.status)) {
						throw new Error(`Backend quest: ${q.status}`);
					}
					return q;
				},
				{
					...OLLAMA_POLL.questExecution,
					message: 'Backend quest did not reach terminal status'
				}
			),
			retry(
				async () => {
					const q = await lifecycleApi.getQuest(frontendInstance);
					if (!TERMINAL_STATUSES.has(q.status)) {
						throw new Error(`Frontend quest: ${q.status}`);
					}
					return q;
				},
				{
					...OLLAMA_POLL.questExecution,
					message: 'Frontend quest did not reach terminal status'
				}
			),
			retry(
				async () => {
					const q = await lifecycleApi.getQuest(testingInstance);
					if (!TERMINAL_STATUSES.has(q.status)) {
						throw new Error(`Testing quest: ${q.status}`);
					}
					return q;
				},
				{
					...OLLAMA_POLL.questExecution,
					message: 'Testing quest did not reach terminal status'
				}
			)
		]);

		// ---------------------------------------------------------------
		// 6. Assert and log results for manual comparison
		// ---------------------------------------------------------------

		console.log('\n========================================');
		console.log('SEMSPEC vs SEMDRAGONS COMPARISON RESULTS');
		console.log('========================================');

		const results = [
			{ label: 'Backend (/goodbye endpoint)', quest: finalBackend },
			{ label: 'Frontend (UI update)', quest: finalFrontend },
			{ label: 'Testing (pytest)', quest: finalTesting }
		];

		for (const { label, quest } of results) {
			const output = (quest as QuestResponse & { output?: string }).output;
			console.log(`\n--- ${label} ---`);
			console.log(`Status: ${quest.status}`);
			console.log(`Output:\n${output ?? '(no output)'}`);
		}

		console.log('\n========================================\n');

		// Assertions are intentionally loose — we're comparing approaches,
		// not expecting deterministic output from a 7B model.
		for (const { label, quest } of results) {
			// Quest reached a terminal status (completed, failed, or escalated are all valid)
			expect(
				TERMINAL_STATUSES.has(quest.status),
				`${label}: expected terminal status, got ${quest.status}`
			).toBe(true);

			// Output should be non-empty if completed
			if (quest.status === 'completed') {
				const output = (quest as QuestResponse & { output?: string }).output;
				expect(output, `${label}: completed quest must have output`).toBeTruthy();
			}
		}

		// Verify all 3 quests executed independently (different IDs, no cross-contamination)
		expect(finalBackend.id).not.toBe(finalFrontend.id);
		expect(finalFrontend.id).not.toBe(finalTesting.id);
		expect(finalBackend.id).not.toBe(finalTesting.id);

		// ---------------------------------------------------------------
		// 7. Trajectory validation for all 3 quests
		// ---------------------------------------------------------------

		const trajectories: Record<string, TrajectoryResponse> = {};
		for (const { label, quest } of results) {
			if (quest.loop_id) {
				const traj = await lifecycleApi.getTrajectory(quest.loop_id);
				trajectories[label] = traj;

				// Outcome should match quest status
				// Trajectory outcome should match quest status
				if (quest.status === 'completed') {
					expect(traj.outcome, `${label}: trajectory outcome`).toBe('complete');
				} else if (quest.status === 'failed') {
					// Trajectory still recorded the agentic loop (which completed),
					// but the boss battle may have failed the quest afterwards.
					expect(
						traj.outcome === 'complete' || traj.outcome === 'failed',
						`${label}: trajectory outcome should be complete or failed`
					).toBe(true);
				}

				// Each trajectory should have at least one model_call step
				expect(
					traj.steps.some((s) => s.step_type === 'model_call'),
					`${label}: trajectory must have model_call steps`
				).toBe(true);

				// Step-level token sums (trajectory-level totals have semstreams aggregation bug)
				const { tokensIn, tokensOut } = trajectoryTokenSums(traj);
				expect(tokensIn, `${label}: step tokens_in`).toBeGreaterThan(0);
				expect(tokensOut, `${label}: step tokens_out`).toBeGreaterThan(0);

				console.log(
					`[Semspec Comparison] ${label} trajectory: ` +
						`${traj.steps.length} steps, ` +
						`${tokensIn}+${tokensOut} tokens, ` +
						`${traj.duration}ms`
				);
			}
		}

		// Loop IDs must be unique across all 3 quests (no cross-contamination)
		const loopIds = results.map((r) => r.quest.loop_id).filter(Boolean);
		expect(new Set(loopIds).size, 'loop IDs must be unique').toBe(loopIds.length);

		// ---------------------------------------------------------------
		// 8. Agent lifecycle — all 3 agents return to idle
		// ---------------------------------------------------------------

		const agentChecks = [
			{ instance: coderInstance, name: 'coder-journeyman', initial: coderAgent },
			{ instance: analystInstance, name: 'analyst-1', initial: analystAgent },
			{ instance: seniorInstance, name: 'senior-dev', initial: seniorAgent }
		];

		for (const { instance, name, initial } of agentChecks) {
			const finalAgent = await retry(
				async () => {
					const a = await lifecycleApi.getAgent(instance);
					// Accept idle, cooldown (defeat), or on_quest (escalation awaiting DM)
					const acceptable = new Set(['idle', 'cooldown', 'on_quest']);
					if (!acceptable.has(a.status)) {
						throw new Error(`${name} still ${a.status}`);
					}
					return a;
				},
				{
					...OLLAMA_POLL.agentIdle,
					message: `${name} did not settle after quest`
				}
			);
			// Escalated agents stay on_quest awaiting DM clarification
			expect(
				finalAgent.status === 'idle' || finalAgent.status === 'cooldown' || finalAgent.status === 'on_quest',
				`${name}: expected idle, cooldown, or on_quest but got ${finalAgent.status}`
			).toBe(true);
			// XP is consumed by level-ups; verify progression via level or stats
			expect(
				finalAgent.level >= initial.level,
				`${name}: level should not decrease`
			).toBe(true);
		}

		// ---------------------------------------------------------------
		// 9. Content quality checks (loose — 7B model output is variable)
		// ---------------------------------------------------------------

		const backendOutput = String(
			(finalBackend as QuestResponse & { output?: string }).output ?? ''
		);
		const frontendOutput = String(
			(finalFrontend as QuestResponse & { output?: string }).output ?? ''
		);
		const testingOutput = String(
			(finalTesting as QuestResponse & { output?: string }).output ?? ''
		);

		if (finalBackend.status === 'completed') {
			expect(
				backendOutput.toLowerCase().includes('goodbye'),
				'backend output should mention "goodbye"'
			).toBe(true);
		}
		if (finalFrontend.status === 'completed') {
			expect(
				frontendOutput.toLowerCase().includes('goodbye'),
				'frontend output should mention "goodbye"'
			).toBe(true);
		}
		if (finalTesting.status === 'completed') {
			expect(
				testingOutput.toLowerCase().includes('test'),
				'testing output should mention "test"'
			).toBe(true);
		}
	});
});

// =============================================================================
// GROUP C: Trajectory Validation
// =============================================================================

test.describe('Trajectory Validation @integration @ollama', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasOllama(),
			'Requires running backend (E2E_BACKEND_AVAILABLE=true) and Ollama (E2E_OLLAMA=true)'
		);
	});

	test('trajectory records full agentic execution', async ({ lifecycleApi }) => {
		// Dedicated trajectory validation: quest completion then full trajectory check.
		test.setTimeout(480_000);

		const quest = await lifecycleApi.createQuest(
			'List three benefits of code review in a numbered list.',
			1
		);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('ollama-trajectory-test');
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok).toBeTruthy();

		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status}`);
				}
				return q;
			},
			{
				...OLLAMA_POLL.questExecution,
				message: 'Trajectory test quest did not complete'
			}
		);

		expect(
			finalQuest.status === 'completed' || finalQuest.status === 'failed',
			`Expected terminal status after boss battle, got ${finalQuest.status}`
		).toBe(true);
		expect(finalQuest.loop_id, 'quest must have loop_id').toBeTruthy();

		// Fetch and validate trajectory
		const trajectory = await lifecycleApi.getTrajectory(finalQuest.loop_id!);

		// loop_id should match
		expect(trajectory.loop_id).toBe(finalQuest.loop_id);

		// Chronological ordering
		expect(trajectory.start_time).toBeTruthy();
		expect(trajectory.end_time).toBeTruthy();
		const startTime = new Date(trajectory.start_time).getTime();
		const endTime = new Date(trajectory.end_time!).getTime();
		expect(endTime, 'end_time must be after start_time').toBeGreaterThan(startTime);

		// Steps in chronological order
		for (let i = 1; i < trajectory.steps.length; i++) {
			const prev = new Date(trajectory.steps[i - 1].timestamp).getTime();
			const curr = new Date(trajectory.steps[i].timestamp).getTime();
			expect(curr, `step ${i} should be >= step ${i - 1}`).toBeGreaterThanOrEqual(prev);
		}

		// Model call step validation
		// Note: agentic-loop splits model calls into request (prompt) and response
		// (response+tokens) steps with the same request_id.
		const modelCalls = trajectory.steps.filter((s) => s.step_type === 'model_call');
		expect(modelCalls.length, 'should have at least one model_call').toBeGreaterThanOrEqual(1);

		// At least one step should have the prompt (request step)
		expect(
			modelCalls.some((s) => s.prompt),
			'should have a model_call step with prompt'
		).toBe(true);
		// At least one step should have the response (completion step)
		expect(hasModelResponse(trajectory), 'should have a model_call step with response').toBe(
			true
		);
		// Response steps should have token counts
		const responseSteps = modelCalls.filter((s) => s.response);
		for (const step of responseSteps) {
			expect(step.tokens_in, 'response step tokens_in').toBeGreaterThan(0);
			expect(step.tokens_out, 'response step tokens_out').toBeGreaterThan(0);
		}

		// Step token sums should be non-zero
		const { tokensIn: sumTokensIn, tokensOut: sumTokensOut } =
			trajectoryTokenSums(trajectory);
		expect(sumTokensIn).toBeGreaterThan(0);
		expect(sumTokensOut).toBeGreaterThan(0);

		// No tool_call steps for apprentice agent
		const toolCalls = trajectory.steps.filter((s) => s.step_type === 'tool_call');
		expect(toolCalls.length, 'apprentice agent should have no tool_call steps').toBe(0);

		console.log(
			`[Ollama] Trajectory validation passed: ${trajectory.steps.length} steps, ` +
				`${sumTokensIn}+${sumTokensOut} tokens, ` +
				`${trajectory.duration}ms, outcome=${trajectory.outcome}`
		);
	});
});

// =============================================================================
// GROUP D: Agent XP Validation
// =============================================================================

test.describe('Agent XP Validation @integration @ollama', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasOllama(),
			'Requires running backend (E2E_BACKEND_AVAILABLE=true) and Ollama (E2E_OLLAMA=true)'
		);
	});

	test('agent earns XP after Ollama quest', async ({ lifecycleApi }) => {
		test.setTimeout(480_000);

		const agent = await lifecycleApi.recruitAgent('ollama-xp-test');
		const agentInstance = extractInstance(agent.id);

		// Record initial state (fresh agent starts at level 1 with 0 XP)
		const initialAgent = await lifecycleApi.getAgent(agentInstance);
		expect(initialAgent.level, 'fresh agent should start at level 1').toBe(1);

		const quest = await lifecycleApi.createQuest(
			'What are two advantages of using version control? Answer briefly.',
			1
		);
		const questInstance = extractInstance(quest.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok).toBeTruthy();

		// Wait for quest completion
		await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status}`);
				}
			},
			{
				...OLLAMA_POLL.questExecution,
				message: 'XP test quest did not complete'
			}
		);

		// Wait for agent to settle after quest terminal state.
		const finalQuest2 = await lifecycleApi.getQuest(questInstance);
		const finalAgent = await retry(
			async () => {
				const a = await lifecycleApi.getAgent(agentInstance);
				if (finalQuest2.status === 'escalated') {
					if (a.status !== 'on_quest') throw new Error(`Escalated but agent is ${a.status}`);
				} else {
					if (a.status !== 'idle' && a.status !== 'cooldown') throw new Error(`Agent still ${a.status}`);
				}
				return a;
			},
			{
				...OLLAMA_POLL.agentIdle,
				message: 'Agent did not reach expected status after XP quest'
			}
		);

		// Agent level should not decrease regardless of outcome.
		// On escalation, no XP change occurs (quest is paused, not resolved).
		expect(
			finalAgent.level,
			'agent level should not decrease'
		).toBeGreaterThanOrEqual(initialAgent.level);
		console.log(
			`[Ollama] Agent progression: level ${initialAgent.level} -> ${finalAgent.level}`
		);
	});
});

// =============================================================================
// GROUP E: Boss Battle Review Pipeline
// =============================================================================

test.describe('Boss Battle Review Pipeline @integration @ollama', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasOllama(),
			'Requires running backend (E2E_BACKEND_AVAILABLE=true) and Ollama (E2E_OLLAMA=true)'
		);
	});

	test('quest with review triggers boss battle after agentic completion', async ({
		lifecycleApi
	}) => {
		// Full adversarial review pipeline:
		//   agentic loop completes → questbridge sets in_review (not completed)
		//   → bossbattle detects in_review transition → starts battle
		//   → evaluator runs judges → verdict → quest completed/failed
		test.setTimeout(480_000);

		// 1. Create a quest that requires review (review_level=1 = Standard)
		const quest = await lifecycleApi.createQuestWithReview(
			'Explain the difference between unit and integration tests in 2 sentences.',
			1 // ReviewStandard
		);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		// Confirm the review flag was set
		const reviewFlag =
			(quest as QuestResponse & { constraints?: { require_review?: boolean } })
				.constraints?.require_review;
		expect(reviewFlag, 'quest must have require_review=true').toBe(true);

		// 2. Recruit agent and start quest to trigger agentic loop
		const agent = await lifecycleApi.recruitAgent('ollama-bossbattle-test');
		const agentInstance = extractInstance(agent.id);
		const initialAgent = await lifecycleApi.getAgent(agentInstance);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// 3. Poll until quest reaches in_review or terminal state.
		//    All quests now go through review, so we expect in_review first.
		const REVIEW_OR_TERMINAL = new Set(['in_review', 'completed', 'failed', 'escalated']);
		const reviewedQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!REVIEW_OR_TERMINAL.has(q.status)) {
					throw new Error(`Quest still ${q.status} — waiting for agentic loop`);
				}
				return q;
			},
			{
				...OLLAMA_POLL.questExecution,
				message:
					'Quest did not reach in_review or terminal status. ' +
					'Check questbridge routes RequireReview quests to in_review.'
			}
		);

		console.log(`[Boss Battle] Quest reached status: ${reviewedQuest.status}`);

		// If quest went to in_review, the boss battle pipeline should fire.
		// If it went straight to completed/failed, the review routing may not be working.
		// Either way, wait for a terminal state after battle resolution.

		// 4. Poll for a boss battle referencing this quest
		const battle = await retry(
			async () => {
				const battles = await lifecycleApi.listBattles();
				const match = battles.find(
					(b: BattleResponse) =>
						b.quest_id === quest.id ||
						extractInstance(b.quest_id ?? '') === questInstance
				);
				if (!match) {
					throw new Error('No boss battle found for this quest yet');
				}
				return match;
			},
			{
				timeout: 60_000,
				interval: 2000,
				message:
					'No boss battle was created for the reviewed quest. ' +
					'Check bossbattle processor is enabled and watches in_review transitions.'
			}
		);

		expect(battle.id, 'boss battle must have an entity ID').toBeTruthy();
		console.log(`[Boss Battle] Battle created: ${battle.id}, status: ${battle.status}`);

		// 5. Poll until battle resolves (victory or defeat).
		//    The bossbattle processor makes an LLM evaluation call which can
		//    take 90-120s with Ollama — use the full quest execution timeout.
		const resolvedBattle = await retry(
			async () => {
				const battles = await lifecycleApi.listBattles();
				const current = battles.find((b: BattleResponse) => b.id === battle.id);
				if (!current) {
					throw new Error('Battle disappeared from list');
				}
				const isResolved =
					current.status === 'victory' ||
					current.status === 'defeat' ||
					current.verdict !== undefined;
				if (!isResolved) {
					throw new Error(`Battle still ${current.status}`);
				}
				return current;
			},
			{
				...OLLAMA_POLL.questExecution,
				message: 'Boss battle did not reach a verdict within timeout'
			}
		);

		expect(resolvedBattle.verdict, 'battle must have a verdict').toBeTruthy();
		console.log(
			`[Boss Battle] Verdict: ${resolvedBattle.status}, ` +
				`passed=${resolvedBattle.verdict?.passed}, ` +
				`quality=${resolvedBattle.verdict?.quality_score}`
		);

		// 6. Quest should reach terminal state after battle verdict
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status} after battle verdict`);
				}
				return q;
			},
			{
				timeout: 30_000,
				interval: 1500,
				message: 'Quest did not reach terminal status after boss battle verdict'
			}
		);

		// If boss battle passed, quest should have verdict set
		if (finalQuest.status === 'completed') {
			const questVerdict = (finalQuest as QuestResponse & { verdict?: { passed?: boolean; quality_score?: number } }).verdict;
			expect(questVerdict, 'completed quest should have verdict from boss battle').toBeTruthy();
			if (questVerdict) {
				expect(questVerdict.passed, 'verdict should be passed for completed quest').toBe(true);
				expect(
					questVerdict.quality_score,
					'verdict should have quality score'
				).toBeGreaterThan(0);
			}
		}

		console.log(`[Boss Battle] Final quest status: ${finalQuest.status}`);

		// 7. Agent should return to idle (or cooldown on defeat) after battle
		const finalAgent = await retry(
			async () => {
				const a = await lifecycleApi.getAgent(agentInstance);
				// After victory: idle. After defeat: idle or cooldown (brief penalty).
				if (a.status !== 'idle' && a.status !== 'cooldown') {
					throw new Error(`Agent still ${a.status}`);
				}
				return a;
			},
			{
				timeout: 60_000,
				interval: 2000,
				message: 'Agent did not return to idle/cooldown after boss battle'
			}
		);

		expect(
			finalAgent.status === 'idle' || finalAgent.status === 'cooldown',
			`agent should be idle or cooldown, got ${finalAgent.status}`
		).toBe(true);
		expect(
			finalAgent.level,
			'agent level should not decrease after battle'
		).toBeGreaterThanOrEqual(initialAgent.level);

		// 8. Trajectory should still be accessible via loop_id
		if (finalQuest.loop_id) {
			const trajectory = await lifecycleApi.getTrajectory(finalQuest.loop_id);
			expect(trajectory.steps.length).toBeGreaterThanOrEqual(1);
			expect(hasModelResponse(trajectory)).toBe(true);

			console.log(
				`[Boss Battle] Trajectory: ${trajectory.steps.length} steps, ` +
					`outcome=${trajectory.outcome}`
			);
		}
	});
});
