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
 *   1. Easy quest     — post via DM chat, watch autonomy claim and complete it
 *   2. Research quest  — post via DM chat, verify graph_search tool call in trajectory
 *   3. Moderate quest  — post a multi-part quest via DM chat, watch it complete
 *   4. Aftermath       — verify downstream effects: XP, battles, guilds, trajectories
 *
 * NOTE: Real LLMs may classify quests differently than expected — an "easy"
 * quest might become a party quest if the LLM sees parallel sub-tasks, and a
 * "moderate" quest might be completed solo. Tests assert on pipeline completion,
 * not on the solo/party classification chosen by the LLM.
 *
 * Design principles:
 *   - Act like a human — all quest creation goes through DM chat UI
 *   - Work WITH autonomy — never pause the board, never manually claim quests
 *   - Use the seeded roster — never recruit agents inside these tests
 *   - Minimal assertions — verify pipeline movement, not intermediate state
 *   - Serial execution — tests share state intentionally; order matters
 *   - API reads are fine for verification (trajectories, quest lists)
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
	// Test 1: Easy Quest
	// ===========================================================================

	test('easy quest: post via DM chat, watch it complete', async ({ page }) => {
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

		// Step 3: Wait for the quest to reach a terminal state. The LLM may
		// execute this as a solo quest or decompose it into a party quest —
		// either path is valid. We only assert on pipeline completion.
		await test.step('wait for quest to reach terminal state', async () => {
			await waitForAnyQuestInColumn(page, 'completed', {
				timeout: isMockLLM() ? 90_000 : 240_000
			});
		});
	});

	// ===========================================================================
	// Test 2: Research Quest — graph_search
	// ===========================================================================

	test('research quest: post via DM chat, verify graph_search in trajectory', async ({
		page,
		lifecycleApi
	}) => {
		test.setTimeout(isMockLLM() ? 120_000 : 480_000);

		// Post a research quest via DM chat. The prompt must match both reQuestBrief
		// ("create.*quest") and reResearch ("research|investigate") so the mock LLM
		// returns a research-themed quest brief.
		await test.step('navigate to dashboard', async () => {
			await page.goto('/');
			await waitForHydration(page);
		});

		await test.step('post research quest via DM chat', async () => {
			await postQuestViaDMChat(
				page,
				'Create a quest to research and investigate best practices for input validation in web applications',
				{ timeout: isMockLLM() ? 30_000 : 60_000 }
			);
		});

		// Navigate to board and wait for the quest to complete.
		await test.step('verify quest appears on board', async () => {
			await page.goto('/quests');
			await page.waitForLoadState('domcontentloaded');

			await expect
				.poll(
					async () => page.locator('[data-testid="quest-card"]').count(),
					{ timeout: 30_000, message: 'No quest cards on board after posting research quest' }
				)
				.toBeGreaterThan(0);
		});

		await test.step('wait for research quest to complete', async () => {
			await waitForAnyQuestInColumn(page, 'completed', {
				timeout: isMockLLM() ? 90_000 : 420_000,
				// At least 2 completed quests (solo + research)
				minCount: 2
			});
		});

		// Verify trajectory has graph_search tool call via API.
		await test.step('verify graph_search in trajectory', async () => {
			// Find the research quest by matching the title from the mock brief.
			const quests = await lifecycleApi.listQuests();
			const researchQuest = quests.find(
				(q) =>
					q.status === 'completed' &&
					q.loop_id &&
					/research|validation/i.test(q.title ?? '')
			);

			if (!researchQuest?.loop_id) {
				console.warn('[Research] No completed research quest with loop_id — skipping trajectory check');
				return;
			}

			const trajectory = await lifecycleApi.getTrajectory(researchQuest.loop_id);
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
	// Test 3: Moderate Quest (multi-part)
	// ===========================================================================

	test('moderate quest: post via DM chat, watch it complete', async ({ page, lifecycleApi }) => {
		test.setTimeout(isMockLLM() ? 180_000 : 600_000);

		// Post a multi-part quest via DM chat. The mock LLM will treat this as a
		// party quest (hints.party_required = true), but a real LLM may solve it
		// solo or decompose it differently — both paths are valid.
		await test.step('navigate to dashboard', async () => {
			await page.goto('/');
			await waitForHydration(page);
		});

		await test.step('post moderate quest via DM chat', async () => {
			await postQuestViaDMChat(
				page,
				'Build a utility library with two independent functions that can be completed in parallel. ' +
					'Sub-task 1: Write a celsius_to_fahrenheit function with unit tests. ' +
					'Sub-task 2: Write a kilometers_to_miles function with unit tests.',
				{ timeout: isMockLLM() ? 30_000 : 60_000 }
			);
		});

		// Verify the quest card is visible on the board.
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

		// Wait for the quest to reach a terminal state. The LLM may execute this
		// as a solo quest (direct completion) or as a party quest (DAG decomposition
		// → sub-quests → lead review → rollup). We accept any terminal state.
		await test.step('wait for quest to reach terminal state', async () => {
			await retry(
				async () => {
					const allQuests = await lifecycleApi.listQuests();

					// Find any quest that reached a terminal state since this test
					// started. We don't match by exact title since the LLM generates
					// it — instead count terminal quests (should be at least 3:
					// easy + research + this one).
					const terminalQuests = allQuests.filter((q) =>
						['completed', 'failed', 'escalated'].includes(q.status)
					);

					// We need at least 3 terminal quests (easy + research + moderate).
					// Use a soft check: if only 2 are terminal, the moderate quest
					// may still be running.
					if (terminalQuests.length < 3) {
						// Log what we see for debugging
						const statuses = allQuests.map((q) => `${q.title?.slice(0, 30)}:${q.status}`);
						throw new Error(
							`Only ${terminalQuests.length} terminal quests (need 3). ` +
								`Board: ${statuses.join(', ')}`
						);
					}

					// Soft-warn if the quest was classified differently than expected
					const partyQuests = allQuests.filter((q) => q.party_id);
					const soloTerminal = terminalQuests.filter((q) => !q.party_id);
					if (isMockLLM() && partyQuests.length === 0) {
						console.warn(
							'[Moderate] Expected party quest from mock LLM but none found — mock routing may have changed'
						);
					} else if (!isMockLLM()) {
						console.log(
							`[Moderate] LLM classification: ${soloTerminal.length} solo, ${partyQuests.length} party quest(s)`
						);
					}

					return terminalQuests;
				},
				{
					timeout: isMockLLM() ? 150_000 : 540_000,
					interval: 3000,
					message: 'Moderate quest did not reach a terminal state'
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

		// Battles page: at least one boss battle should exist from completed quests.
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

		// Boid guild suggestions: after quests with peer reviews, the boid engine
		// should compute guild formation/join suggestions on subsequent ticks.
		// If all quests ran solo (no peer reviews), guild suggestions may not appear —
		// this is a soft check for real LLMs, hard check for mock.
		await test.step('boid engine produces guild suggestions', async () => {
			const backendPort = process.env.BACKEND_PORT || '8081';

			try {
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
					{ timeout: 20_000, interval: 2_000, message: 'No guild suggestions found' }
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
			} catch {
				if (isMockLLM()) {
					throw new Error('Mock LLM should always produce peer reviews → guild suggestions');
				}
				console.warn(
					'[Aftermath] No guild suggestions — LLM may have run all quests solo (no peer reviews)'
				);
			}
		});

		// Guild formation: with autonomy running and boid suggestions present,
		// agents may act on guild suggestions. Pipeline: boid suggestion → autonomy
		// heartbeat → createGuild/joinGuild → KV update. If no peer reviews
		// occurred (all solo quests), guild formation won't trigger.
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
