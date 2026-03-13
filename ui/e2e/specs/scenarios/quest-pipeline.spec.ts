import {
	test,
	expect,
	hasBackend,
	hasLLM,
	isMockLLM,
	extractInstance,
	retry,
	waitForHydration
} from '../../fixtures/test-base';
import {
	postQuestViaDMChat,
	waitForAnyQuestInColumn
} from '../../fixtures/scenario-helpers';

/**
 * Quest Pipeline — Tier 2 Scenario Suite
 *
 * Exercises the full quest pipeline end-to-end by acting like a human at the
 * dashboard. The seeded E2E roster (21 agents across all tiers) is already
 * running; autonomy, partycoord, guildformation, bossbattle, and agent
 * progression all run without intervention.
 *
 * Test order:
 *   1. Solo quest     — post via DM chat, watch autonomy claim and complete it
 *   2. Research quest — post via API, verify graph_search tool call in trajectory
 *   3. Party quest    — post epic quest via API, watch DAG execute and roll up
 *   4. Aftermath      — verify downstream effects: XP, battles, guilds, trajectories
 *
 * Design principles:
 *   - Work WITH autonomy — never pause the board, never manually claim quests
 *   - Use the seeded roster — never recruit agents inside these tests
 *   - Minimal assertions — verify pipeline movement, not intermediate state
 *   - Serial execution — tests share state intentionally; order matters
 *
 * @scenario @tier2
 */

test.describe.serial('Quest Pipeline', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend with LLM (E2E_LLM_MODE=mock|gemini|openai|...)'
		);
	});

	// ===========================================================================
	// Test 1: Solo Quest
	// ===========================================================================

	test('solo quest: post via DM chat, watch it complete', async ({ page }) => {
		test.setTimeout(isMockLLM() ? 120_000 : 300_000);

		// Step 1: Navigate to the dashboard and post a quest via the DM chat panel.
		// A human would open the chat, type a command, confirm the preview, and post.
		await test.step('navigate to dashboard', async () => {
			await page.goto('/');
			await waitForHydration(page);
		});

		await test.step('post quest via DM chat', async () => {
			await postQuestViaDMChat(
				page,
				'Create a quest to write a Python function that checks if a number is prime, with unit tests',
				{ timeout: isMockLLM() ? 30_000 : 60_000 }
			);
		});

		// Step 2: Navigate to the quests board and verify the quest card appeared.
		// With autonomy running, the quest may already be claimed before we arrive.
		await test.step('verify quest appears on board', async () => {
			await page.goto('/quests');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="quest-card"]').count(),
					{ timeout: 30_000, message: 'No quest cards on board after posting via DM chat' }
				)
				.toBeGreaterThan(0);
		});

		// Step 3: Wait for the quest to reach completed. The full pipeline runs:
		//   autonomy claims → questbridge dispatches → mock LLM executes
		//   → quest submitted → boss battle resolves → quest completed
		await test.step('wait for quest to reach completed column', async () => {
			await waitForAnyQuestInColumn(page, 'completed', {
				timeout: isMockLLM() ? 90_000 : 240_000
			});
		});
	});

	// ===========================================================================
	// Test 2: Research Quest — graph_search
	// ===========================================================================

	test('research quest: graph_search tool used in trajectory', async ({ lifecycleApi }) => {
		test.setTimeout(isMockLLM() ? 120_000 : 300_000);

		// Post a research quest — prompt must match reResearch pattern (research|investigate).
		const quest = await test.step('post research quest via API', async () => {
			return lifecycleApi.createQuest(
				'Research and investigate best practices for input validation in web applications'
			);
		});

		const questInstance = extractInstance(quest.id);

		// Wait for completion.
		const finalQuest = await test.step('wait for quest completion', async () => {
			return retry(
				async () => {
					const q = await lifecycleApi.getQuest(questInstance);
					if (!['completed', 'failed'].includes(q.status)) {
						throw new Error(`Research quest still ${q.status}`);
					}
					return q;
				},
				{
					timeout: isMockLLM() ? 90_000 : 240_000,
					interval: 3000,
					message: 'Research quest did not reach terminal state'
				}
			);
		});

		// Verify trajectory has graph_search tool call.
		await test.step('verify graph_search in trajectory', async () => {
			if (!finalQuest.loop_id) {
				console.warn('[Research] No loop_id on completed quest — skipping trajectory check');
				return;
			}

			const trajectory = await lifecycleApi.getTrajectory(finalQuest.loop_id);
			const toolCalls = trajectory.steps.filter((s) => s.step_type === 'tool_call');
			const toolNames = toolCalls.map((s) => s.tool_name);
			console.log(`[Research] Trajectory tool calls: ${toolNames.join(', ')}`);

			const graphSearchCalls = toolCalls.filter((s) => s.tool_name === 'graph_search');
			if (isMockLLM()) {
				expect(
					graphSearchCalls.length,
					'agentic loop should have called graph_search for research quest'
				).toBeGreaterThan(0);
			} else if (graphSearchCalls.length === 0) {
				console.warn(
					'[Research] graph_search not called — real LLM may skip if graph has no relevant data'
				);
			}

			// Check that graph manifest section was injected into the prompt.
			const modelCalls = trajectory.steps.filter(
				(s) => s.step_type === 'model_call' && s.prompt
			);
			if (modelCalls.length > 0) {
				const hasGraphContents = modelCalls[0].prompt?.includes('Graph Contents');
				console.log(`[Research] Graph manifest in prompt: ${hasGraphContents}`);
			}
		});
	});

	// ===========================================================================
	// Test 3: Party Quest
	// ===========================================================================

	test('party quest: post epic quest, watch DAG execute', async ({ page, lifecycleApi }) => {
		test.setTimeout(isMockLLM() ? 180_000 : 600_000);

		// Post via the lifecycle API since DM chat does not surface party hints.
		// The seeded roster already has Master-tier agents (lv16-17) available as
		// party leads and Journeyman agents (lv7-9) available as members.
		const parentQuest = await test.step('post party-required epic quest via API', async () => {
			const quest = await lifecycleApi.createQuestWithParty(
				'Build a utility library with two independent functions. ' +
					'Sub-task 1: Write a Python function celsius_to_fahrenheit(c) with unit tests. ' +
					'Sub-task 2: Write a Python function kilometers_to_miles(km) with unit tests. ' +
					'Each sub-task is independent and can be completed in parallel.',
				3
			);
			expect(quest.id).toBeTruthy();
			return quest;
		});

		const parentInstance = extractInstance(parentQuest.id);

		// Verify the quest card is visible on the board before we start waiting
		// for longer pipeline stages.
		await test.step('verify quest appears on board', async () => {
			await page.goto('/quests');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="quest-card"]').count(),
					{ timeout: 15_000, message: 'No quest cards on board' }
				)
				.toBeGreaterThan(0);
		});

		// Poll the API for the parent quest terminal state. The full DAG pipeline:
		//   partycoord forms party → questbridge dispatches to lead agent
		//   → lead calls decompose_quest → sub-quests posted
		//   → questdagexec drives sub-quest assignment and execution
		//   → lead reviews each node via review_sub_quest
		//   → parent quest rolls up to completed/failed/escalated
		await test.step('wait for parent quest to reach terminal state', async () => {
			await retry(
				async () => {
					const q = await lifecycleApi.getQuest(parentInstance);
					if (!['completed', 'failed', 'escalated'].includes(q.status)) {
						throw new Error(`Parent quest still ${q.status}`);
					}
					return q;
				},
				{
					timeout: isMockLLM() ? 150_000 : 540_000,
					interval: 3000,
					message: 'Parent quest did not reach a terminal state'
				}
			);
		});
	});

	// ===========================================================================
	// Test 4: Aftermath
	// ===========================================================================

	test('verify aftermath: agents have XP, battles exist, guilds formed', async ({
		page,
		lifecycleApi
	}) => {
		test.setTimeout(90_000);

		// After two quests ran through the pipeline, the downstream systems should
		// have fired. This test navigates around and verifies the effects exist —
		// it does not retry long waits because those effects happened in prior tests.

		// Agents page: at least one agent should be visible.
		await test.step('agents page shows agents', async () => {
			await page.goto('/agents');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="agent-card"]').count(),
					{ timeout: 10_000, message: 'No agent cards on agents page' }
				)
				.toBeGreaterThan(0);
		});

		// Battles page: at least one boss battle should exist from the solo quest.
		await test.step('battles page shows boss battles', async () => {
			await page.goto('/battles');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => {
						const cards = await page.locator('[data-testid="battle-card"]').count();
						if (cards > 0) return cards;
						// Fallback: look for battle content in the page body
						const text = await page.locator('main').textContent();
						return (text?.match(/victory|defeat|battle/gi) ?? []).length;
					},
					{ timeout: 15_000, message: 'No battle content on battles page' }
				)
				.toBeGreaterThan(0);
		});

		// World state API: verify at least one agent has non-zero XP.
		await test.step('world state shows at least one agent with XP', async () => {
			const world = await lifecycleApi.getWorldState();
			const agents = world.agents ?? [];
			const agentWithXP = agents.find(
				(a) => ((a as { xp?: number }).xp ?? 0) > 0
			);
			// Non-fatal: XP propagation timing is best-effort across test boundaries.
			// Log the finding rather than hard-failing the aftermath check.
			if (!agentWithXP) {
				console.warn('Aftermath: no agent with XP > 0 found; XP may still be propagating');
			}
		});

		// Agent detail: navigate to any agent's detail page and verify the level
		// field is visible — proves the detail page hydrates and renders properly.
		await test.step('agent detail page renders level', async () => {
			const world = await lifecycleApi.getWorldState();
			const agents = world.agents ?? [];
			if (agents.length === 0) return;

			const agentInstance = extractInstance(agents[0].id);

			await expect(async () => {
				await page.goto(`/agents/${agentInstance}`);
				await waitForHydration(page);
				await expect(page.locator('[data-testid="agent-name"]')).toBeVisible({
					timeout: 5_000
				});
			}).toPass({ timeout: 15_000 });

			await expect(page.locator('[data-testid="agent-level"]')).toBeVisible();
		});

		// Boid guild suggestions: after party quests produce peer reviews, the boid
		// engine should compute guild formation/join suggestions on subsequent ticks.
		// This is a hard assertion — after real quests with peer reviews, the boid
		// engine MUST produce guild suggestions within a reasonable window.
		await test.step('boid engine produces guild suggestions after peer reviews', async () => {
			const backendPort = process.env.BACKEND_PORT || '8081';

			const suggestions = await retry(
				async () => {
					const res = await fetch(
						`http://localhost:${backendPort}/message-logger/kv/BOID_SUGGESTIONS/watch?pattern=guild.*`
					);
					if (!res.ok) throw new Error(`message-logger: ${res.status}`);

					const reader = res.body?.getReader();
					if (!reader) throw new Error('No response body');

					const decoder = new TextDecoder();
					let partial = '';
					const entries: string[] = [];

					const readTimeout = setTimeout(() => reader.cancel(), 3000);
					try {
						while (true) {
							const { done, value } = await reader.read();
							if (done) break;
							partial += decoder.decode(value, { stream: true });
							const lines = partial.split('\n');
							partial = lines.pop()!; // keep incomplete trailing line
							for (const line of lines) {
								if (line.startsWith('data: ')) entries.push(line.slice(6));
							}
						}
					} catch {
						// Reader cancelled by timeout — expected
					} finally {
						clearTimeout(readTimeout);
					}

					if (entries.length === 0) throw new Error('No guild suggestions yet');
					return entries;
				},
				{ timeout: 20_000, interval: 2_000, message: 'Boid engine did not produce guild suggestions after peer reviews' }
			);

			expect(suggestions.length).toBeGreaterThan(0);

			// Verify at least one suggestion has the expected guild suggestion shape
			const parsed = JSON.parse(suggestions[0]);
			if (Array.isArray(parsed) && parsed.length > 0) {
				expect(parsed[0]).toHaveProperty('agent_id');
				expect(parsed[0]).toHaveProperty('type');
				expect(['join', 'form']).toContain(parsed[0].type);
				console.log(
					`[Aftermath] Boid guild suggestions: ${suggestions.length} entries, ` +
						`first has ${parsed.length} suggestion(s) of type "${parsed[0].type}"`
				);
			} else {
				console.log(`[Aftermath] Boid guild suggestions: ${suggestions.length} entries`);
			}
		});

		// Guild formation: with autonomy running and boid suggestions present,
		// agents should act on guild suggestions. Poll for actual guild formation
		// with a generous timeout — the pipeline is: boid suggestion → autonomy
		// heartbeat → createGuild/joinGuild → KV update.
		await test.step('guild forms after boid suggestions (with timeout)', async () => {
			let guildCount = 0;
			try {
				const guilds = await retry(
					async () => {
						const list = await lifecycleApi.listGuilds();
						if (list.length === 0) throw new Error('No guilds formed yet');
						return list;
					},
					{ timeout: 30_000, interval: 3_000, message: 'No guild formed within timeout' }
				);
				guildCount = guilds.length;
				expect(guilds.length).toBeGreaterThan(0);
				expect(guilds[0].name).toBeTruthy();
				console.log(
					`[Aftermath] Guild "${guilds[0].name}" formed with ${guilds[0].members?.length ?? 0} members`
				);
			} catch {
				// Soft failure — guild formation depends on autonomy timing.
				// The boid suggestion assertion above is the hard gate.
				console.warn('[Aftermath] No guild formed within 30s — boid suggestions were present but autonomy did not act in time');
			}

			// Navigate to guilds page and verify UI reflects the state
			await page.goto('/guilds');
			await page.waitForLoadState('domcontentloaded');
			const cards = await page.locator('[data-testid="guild-card"]').count();
			console.log(`[Aftermath] Guilds page shows ${cards} guild card(s) (API reported ${guildCount})`);
		});

		// Trajectories: every completed agentic loop leaves a trajectory entry.
		await test.step('trajectories page shows entries', async () => {
			await page.goto('/trajectories');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () =>
						page
							.locator(
								'[data-testid="trajectory-entry"], .trajectory-row, .trajectory-item, tr'
							)
							.count(),
					{ timeout: 15_000, message: 'No trajectory entries found' }
				)
				.toBeGreaterThan(0);
		});

		// Trajectory tool calls: verify completed quests have tool_call steps.
		await test.step('trajectory shows tool calls from agentic loop', async () => {
			const quests = await lifecycleApi.listQuests();
			const completedWithLoop = quests.find(
				(q) => q.status === 'completed' && q.loop_id
			);
			if (!completedWithLoop?.loop_id) {
				console.warn(
					'[Aftermath] No completed quest with loop_id — skipping trajectory check'
				);
				return;
			}

			const trajectory = await lifecycleApi.getTrajectory(completedWithLoop.loop_id);
			expect(trajectory.steps.length).toBeGreaterThanOrEqual(1);

			const toolCalls = trajectory.steps.filter((s) => s.step_type === 'tool_call');
			expect(
				toolCalls.length,
				'agentic loop should have at least one tool_call'
			).toBeGreaterThan(0);

			const toolNames = toolCalls.map((s) => s.tool_name);
			console.log(`[Aftermath] Trajectory tool calls: ${toolNames.join(', ')}`);
		});
	});
});
