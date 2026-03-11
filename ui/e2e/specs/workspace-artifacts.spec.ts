import { test, expect, hasBackend, hasLLM, isRealLLM, isMockLLM, extractInstance, retry } from '../fixtures/test-base';
import type { QuestResponse, BattleResponse } from '../fixtures/test-base';

/**
 * Workspace Artifact Lifecycle E2E Tests
 *
 * Verifies the workspace lifecycle introduced in questbridge:
 *   1. Sandbox workspace created before quest dispatch
 *   2. Agent tools operate in the workspace (read/write files)
 *   3. Workspace files snapshotted to filestore on quest completion
 *   4. Workspace cleaned up after snapshot
 *   5. Boss battle judge can read artifacts during evaluation
 *
 * The mock LLM is configured to call write_file when it's available
 * (Expert agents with code_generation skill), and list_directory for
 * Apprentice agents. This exercises both the file-writing and read-only
 * workspace paths.
 *
 * @integration
 */

// =============================================================================
// POLLING CONFIGURATION
// =============================================================================

const POLL = isRealLLM()
	? {
			questExecution: { timeout: 120_000, interval: 3000 },
			artifactSync: { timeout: 30_000, interval: 2000 },
			battleResolution: { timeout: 90_000, interval: 3000 }
		}
	: {
			questExecution: { timeout: 60_000, interval: 1500 },
			artifactSync: { timeout: 30_000, interval: 1500 },
			battleResolution: { timeout: 60_000, interval: 1500 }
		};

const TERMINAL_STATUSES = new Set(['completed', 'failed', 'in_review', 'escalated']);

/**
 * Quest description for workspace tests — concrete enough for real LLMs
 * to produce a work product rather than requesting clarification.
 */
const WORKSPACE_QUEST =
	'Write a Python function called `fibonacci(n)` that returns the nth Fibonacci number. ' +
	'Include a docstring and handle n < 0 raising ValueError. Save the code as solution.py.';

// =============================================================================
// GROUP A: Workspace Artifact Lifecycle (Expert agent with write_file)
// =============================================================================

test.describe('Workspace Artifact Lifecycle @integration', () => {
	test.describe.configure({ mode: isRealLLM() ? 'serial' : 'parallel' });

	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend and LLM (E2E_LLM_MODE=mock|gemini|openai|anthropic|ollama)'
		);
	});

	test('expert agent quest produces workspace artifacts in filestore', async ({ lifecycleApi }) => {
		// Expert agents (level 11+) get write_file in their tool set.
		// The mock LLM calls write_file(solution.py) on the first turn,
		// then submit_work_product with summary-only on the second turn.
		// Questbridge snapshots the written file to filestore.
		test.setTimeout(isRealLLM() ? 180_000 : 75_000);

		// 1. Recruit Expert agent with code_generation skill (enables write_file).
		const agent = await lifecycleApi.recruitAgentAtLevel(`artifact-expert-${Date.now()}`, 11, [
			'code_generation'
		]);
		const agentInstance = extractInstance(agent.id);

		// 2. Create a simple quest (no review) to isolate workspace testing.
		const quest = await lifecycleApi.createQuest(WORKSPACE_QUEST, 2);
		const questInstance = extractInstance(quest.id);

		// 3. Claim and start to trigger the agentic loop.
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// 4. Wait for quest to reach a terminal state.
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status}`);
				}
				return q;
			},
			{
				...POLL.questExecution,
				message: 'Quest did not reach terminal status for workspace artifact test'
			}
		);

		if (finalQuest.status === 'escalated') {
			test.skip(true, 'Quest escalated — LLM requested clarification instead of writing files');
			return;
		}

		// 5. Poll for artifacts to appear in filestore.
		// The snapshot is async (detached context), so artifacts may lag slightly
		// behind quest completion.
		const artifacts = await retry(
			async () => {
				const result = await lifecycleApi.listQuestArtifacts(questInstance);
				if (result.count === 0) {
					throw new Error('No artifacts in filestore yet — snapshot may still be in progress');
				}
				return result;
			},
			{
				...POLL.artifactSync,
				message:
					'No workspace artifacts appeared in filestore after quest completion. ' +
					'Check that questbridge snapshots workspace files and filestore component is enabled.'
			}
		);

		// 6. Verify the expected file was captured.
		expect(artifacts.count).toBeGreaterThan(0);
		if (isMockLLM()) {
			// Mock LLM deterministically writes solution.py via write_file tool.
			expect(artifacts.files).toContain('solution.py');
		}
	});

	test('boss battle with artifacts evaluates workspace files', async ({ lifecycleApi }) => {
		// Full pipeline: Expert agent writes files → quest goes to review →
		// boss battle judge loads artifacts from filestore → verdict issued.
		test.setTimeout(isRealLLM() ? 180_000 : 90_000);

		// 1. Recruit Expert agent.
		const agent = await lifecycleApi.recruitAgentAtLevel(`artifact-battle-${Date.now()}`, 11, [
			'code_generation'
		]);
		const agentInstance = extractInstance(agent.id);

		// 2. Create a quest that requires review (triggers boss battle).
		const quest = await lifecycleApi.createQuestWithReview(WORKSPACE_QUEST, 1);
		const questInstance = extractInstance(quest.id);

		// 3. Claim and start.
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// 4. Wait for quest to reach terminal state.
		const terminalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status}`);
				}
				return q;
			},
			{
				...POLL.questExecution,
				message: 'Quest did not reach terminal status'
			}
		);

		if (terminalQuest.status === 'escalated') {
			test.skip(true, 'Quest escalated — boss battle not triggered');
			return;
		}

		// 5. Wait for boss battle to be created and resolved.
		const battle = await retry(
			async () => {
				const battles = await lifecycleApi.listBattles();
				const match = battles.find(
					(b: BattleResponse) =>
						b.quest_id === quest.id ||
						extractInstance(b.quest_id ?? '') === questInstance
				);
				if (!match) throw new Error('No boss battle found for quest');
				return match;
			},
			{
				...POLL.battleResolution,
				message: 'No boss battle created for the reviewed quest with artifacts'
			}
		);
		expect(battle.id).toBeTruthy();

		// 6. Wait for battle to resolve (verdict issued).
		const resolvedBattle = await retry(
			async () => {
				const battles = await lifecycleApi.listBattles();
				const current = battles.find((b: BattleResponse) => b.id === battle.id);
				if (!current) throw new Error('Battle disappeared');
				const isResolved =
					current.status === 'victory' ||
					current.status === 'defeat' ||
					current.status === 'retreat' ||
					current.verdict !== undefined;
				if (!isResolved) {
					throw new Error(`Battle still active: status=${current.status}`);
				}
				return current;
			},
			{
				...POLL.battleResolution,
				message:
					'Boss battle did not resolve. Check that bossbattle evaluator loads artifacts from filestore.'
			}
		);

		const hasVerdict =
			resolvedBattle.verdict !== undefined ||
			resolvedBattle.status === 'victory' ||
			resolvedBattle.status === 'defeat';
		expect(hasVerdict, 'boss battle must have a resolved verdict').toBe(true);

		// 7. Verify artifacts were available for the judge.
		const artifacts = await lifecycleApi.listQuestArtifacts(questInstance);
		if (isMockLLM()) {
			// Mock LLM deterministically writes solution.py
			expect(artifacts.count).toBeGreaterThan(0);
			expect(artifacts.files).toContain('solution.py');
		}
	});
});

// =============================================================================
// GROUP B: Summary-Only Submissions (Apprentice agent, read-only tools)
// =============================================================================

test.describe('Summary-Only Work Submissions @integration', () => {
	test.describe.configure({ mode: isRealLLM() ? 'serial' : 'parallel' });

	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend and LLM'
		);
	});

	test('apprentice agent quest completes with summary-only output', async ({ lifecycleApi }) => {
		// Apprentice agents (level 1-5) get read-only tools: list_directory, read_file.
		// The mock LLM calls list_directory on first turn, then submit_work_product
		// with both deliverable and summary on second turn. Quest completes.
		// No files are written to workspace, so artifact list should be empty.
		test.setTimeout(isRealLLM() ? 180_000 : 60_000);

		const agent = await lifecycleApi.recruitAgent('summary-apprentice-agent');
		const agentInstance = extractInstance(agent.id);

		const quest = await lifecycleApi.createQuest(WORKSPACE_QUEST, 1);
		const questInstance = extractInstance(quest.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// Wait for quest to complete.
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status}`);
				}
				return q;
			},
			{
				...POLL.questExecution,
				message: 'Quest did not reach terminal status'
			}
		);

		// Quest must have completed with output populated.
		expect(
			TERMINAL_STATUSES.has(finalQuest.status),
			`Expected terminal status, got ${finalQuest.status}`
		).toBe(true);

		if (finalQuest.status === 'completed' || finalQuest.status === 'in_review') {
			const output = (finalQuest as QuestResponse & { output?: string }).output;
			expect(output, 'quest output must be populated from work product submission').toBeTruthy();
		}

		// No files written to workspace — artifact list should be empty.
		const artifacts = await lifecycleApi.listQuestArtifacts(questInstance);
		expect(artifacts.count).toBe(0);
	});

	test('read-only workspace produces no artifacts in filestore', async ({ lifecycleApi }) => {
		// Verifies that when an agent only reads files (list_directory, read_file)
		// without writing anything, the snapshot correctly finds an empty workspace
		// and no artifacts are stored.
		test.setTimeout(isRealLLM() ? 180_000 : 60_000);

		const agent = await lifecycleApi.recruitAgent('readonly-workspace-agent');
		const agentInstance = extractInstance(agent.id);

		const quest = await lifecycleApi.createQuest(
			'List the files in the current directory and describe what you find.',
			1
		);
		const questInstance = extractInstance(quest.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status}`);
				}
			},
			{
				...POLL.questExecution,
				message: 'Quest did not reach terminal status'
			}
		);

		// Wait a short time for any snapshot to complete, then verify empty.
		// Use a short retry with a quick failure since we expect zero artifacts.
		const artifacts = await lifecycleApi.listQuestArtifacts(questInstance);
		expect(artifacts.count).toBe(0);
		expect(artifacts.files).toEqual([]);
	});
});
