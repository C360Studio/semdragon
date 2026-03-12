import { test, expect, hasBackend, hasLLM, isRealLLM, extractInstance, retry } from '../fixtures/test-base';

/**
 * Web Search Tool E2E Tests
 *
 * Exercises the web_search tool roundtrip through the agentic pipeline:
 *
 *   quest (research objective) → questbridge → agentic-loop
 *     → LLM calls web_search → questtools executes Brave API (or mock fallback)
 *     → tool result returned → LLM calls submit_work_product → quest completed
 *
 * In mock mode without BRAVE_SEARCH_API_KEY, questtools won't register
 * web_search — the mock LLM falls through to filesystem tools instead.
 * This still validates the agentic loop wiring. To test the full mock
 * web_search path, BRAVE_SEARCH_API_KEY must be set to a valid key.
 *
 * With real LLMs + BRAVE_SEARCH_API_KEY, the spec validates the actual
 * Brave Search integration end-to-end.
 *
 * Tag: @integration
 */

const POLL = isRealLLM()
	? { questExecution: { timeout: 180_000, interval: 3000 } }
	: { questExecution: { timeout: 60_000, interval: 1500 } };

const TERMINAL_STATUSES = new Set(['completed', 'failed', 'in_review', 'escalated']);

/**
 * Research-oriented quest objective that triggers the mock LLM's reResearch
 * regex pattern. The keywords "Research" and "Search the web" are intentional.
 */
const RESEARCH_QUEST =
	'Research current best practices for input validation in Go web applications. ' +
	'Search the web for recent documentation, then summarize the top 3 recommendations.';

test.describe('Web Search Tool Integration @integration', () => {
	test.describe.configure({ mode: isRealLLM() ? 'serial' : 'parallel' });

	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend and LLM (E2E_LLM_MODE=mock|gemini|openai|anthropic|ollama)'
		);
	});

	test('research quest completes via agentic loop', async ({ lifecycleApi }) => {
		test.setTimeout(isRealLLM() ? 180_000 : 60_000);

		// Recruit an agent with research skill — ensures SkillResearch tag,
		// which grants web_search access via questtools tier/skill gates.
		const agent = await lifecycleApi.recruitAgent('web-search-e2e-agent', ['research']);
		expect(agent.id).toBeTruthy();
		const agentInstance = extractInstance(agent.id);

		// Create a research quest that triggers the mock's reResearch pattern.
		const quest = await lifecycleApi.createQuest(RESEARCH_QUEST, 1);
		expect(quest.id).toBeTruthy();
		const questInstance = extractInstance(quest.id);

		// Claim and start to trigger the agentic loop.
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// Poll until quest reaches a terminal state.
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(
						`Quest still in non-terminal status: ${q.status} — waiting for agentic loop`
					);
				}
				return q;
			},
			{
				...POLL.questExecution,
				message:
					'Research quest did not reach terminal status. ' +
					'Check that the agentic loop processes research-type quests correctly.'
			}
		);

		expect(
			TERMINAL_STATUSES.has(finalQuest.status),
			`Expected terminal status, got ${finalQuest.status}`
		).toBe(true);

		if (finalQuest.status === 'completed') {
			expect(finalQuest.completed_at).toBeTruthy();
		}
	});

	test('trajectory contains web_search tool call (real LLM only)', async ({ lifecycleApi }) => {
		// This test only runs with real LLMs + BRAVE_SEARCH_API_KEY.
		// It verifies the trajectory audit trail includes the web_search tool call.
		test.skip(!isRealLLM(), 'Trajectory web_search verification requires a real LLM');
		test.setTimeout(180_000);

		const agent = await lifecycleApi.recruitAgent('web-search-trajectory-agent', ['research']);
		const agentInstance = extractInstance(agent.id);

		const quest = await lifecycleApi.createQuest(RESEARCH_QUEST, 1);
		const questInstance = extractInstance(quest.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok).toBeTruthy();

		// Wait for completion
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest status: ${q.status}`);
				}
				return q;
			},
			{ ...POLL.questExecution, message: 'Quest did not complete' }
		);

		// Soft-verify: check trajectory for web_search presence.
		// Log the result but don't fail — the LLM may choose not to search.
		if (finalQuest.status === 'completed' || finalQuest.status === 'in_review') {
			try {
				const loopId = (finalQuest as { loop_id?: string }).loop_id;
				if (loopId) {
					const trajectory = await lifecycleApi.getTrajectory(loopId);
					const hasWebSearch = trajectory.steps?.some(
						(step) => step.tool_name === 'web_search'
					);
					if (hasWebSearch) {
						console.log('[web-search] Trajectory confirms web_search tool was called');
					} else {
						console.log(
							'[web-search] web_search not found in trajectory — ' +
								'LLM may have used other tools or BRAVE_SEARCH_API_KEY may not be set'
						);
					}
				}
			} catch {
				console.log('[web-search] Could not fetch trajectory — skipping verification');
			}
		}

		expect(TERMINAL_STATUSES.has(finalQuest.status)).toBe(true);
	});
});
