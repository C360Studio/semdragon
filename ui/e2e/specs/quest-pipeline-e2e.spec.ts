import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';
import type {
	QuestResponse,
	AgentResponse,
	WorldStateResponse
} from '../fixtures/test-base';

/**
 * Autonomous Quest Pipeline E2E Tests
 *
 * Tests the full autonomous workflow: post a quest, autonomy claims and
 * completes it via real LLM (Ollama), verify dashboard updates reactively.
 *
 * Unlike ollama-integration.spec.ts which manually claims/starts quests,
 * these tests let the autonomy processor handle everything — matching
 * the real production flow.
 *
 * Environment requirements:
 *   - E2E_OLLAMA=true (gate variable)
 *   - Ollama running on host with qwen2.5-coder:7b pulled
 *   - Backend running with autonomy enabled
 *   - SEED_E2E=true for seeded agents
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
	questTerminal: { timeout: 300_000, interval: 3000 },
	agentIdle: { timeout: 60_000, interval: 2000 },
	uiUpdate: { timeout: 30_000, interval: 1000 }
} as const;

const TERMINAL_STATUSES = new Set(['completed', 'failed']);

const API_URL = process.env.API_URL || 'http://localhost:8080';

// =============================================================================
// GROUP A: Autonomous Quest Completion
// =============================================================================

test.describe('Autonomous Quest Pipeline @integration @ollama', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasOllama(),
			'Requires running backend (E2E_BACKEND_AVAILABLE=true) and Ollama (E2E_OLLAMA=true)'
		);
	});

	test('A1: posted quest is autonomously claimed and completed', async ({ lifecycleApi }) => {
		test.setTimeout(360_000);

		// Verify agents exist and at least one is idle
		const world = await lifecycleApi.getWorldState();
		const agents = world.agents ?? [];
		expect(agents.length, 'need seeded agents (SEED_E2E=true)').toBeGreaterThan(0);

		const idleAgents = agents.filter((a) => a.status === 'idle');
		expect(idleAgents.length, 'need at least one idle agent').toBeGreaterThan(0);

		// Post quest — autonomy should pick it up
		const quest = await lifecycleApi.createQuest(
			'Explain in one sentence what a hash table is and why it is useful.',
			1
		);
		expect(quest.id).toBeTruthy();
		console.log(`[Pipeline] Posted quest ${quest.id}`);

		// Wait for autonomous completion
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(extractInstance(quest.id));
				if (!TERMINAL_STATUSES.has(q.status)) {
					throw new Error(`Quest still ${q.status}`);
				}
				return q;
			},
			{
				...POLL.questTerminal,
				message: 'Quest was not autonomously claimed and completed within 5 minutes'
			}
		);

		expect(finalQuest.status).toBe('completed');
		expect(finalQuest.agent_id || finalQuest.claimed_by, 'quest must have a claiming agent').toBeTruthy();
		expect(finalQuest.loop_id, 'quest must have loop_id (agentic-loop ran)').toBeTruthy();
		console.log(`[Pipeline] Quest completed by agent, loop_id: ${finalQuest.loop_id}`);

		// Verify the claiming agent returns to idle
		const claimingAgentId = (finalQuest as QuestResponse & { claimed_by?: string }).claimed_by
			?? finalQuest.agent_id;
		if (claimingAgentId) {
			const agent = await retry(
				async () => {
					const a = await lifecycleApi.getAgent(extractInstance(claimingAgentId));
					if (a.status !== 'idle') {
						throw new Error(`Agent still ${a.status}`);
					}
					return a;
				},
				{
					...POLL.agentIdle,
					message: 'Claiming agent did not return to idle'
				}
			);
			expect(agent.status).toBe('idle');
			console.log(`[Pipeline] Agent ${claimingAgentId} returned to idle`);
		}
	});

	test('A2: second quest is also autonomously completed', async ({ lifecycleApi }) => {
		test.setTimeout(360_000);

		// Post first quest and wait
		const quest1 = await lifecycleApi.createQuest(
			'What is the difference between a stack and a queue? One sentence.',
			1
		);
		console.log(`[Pipeline] Posted quest 1: ${quest1.id}`);

		const finalQuest1 = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(extractInstance(quest1.id));
				if (!TERMINAL_STATUSES.has(q.status)) throw new Error(`Quest 1 still ${q.status}`);
				return q;
			},
			{ ...POLL.questTerminal, message: 'Quest 1 did not complete' }
		);

		// Verify quest 1's claiming agent returns to idle
		const agent1Id = (finalQuest1 as QuestResponse & { claimed_by?: string }).claimed_by
			?? finalQuest1.agent_id;
		if (agent1Id) {
			await retry(
				async () => {
					const a = await lifecycleApi.getAgent(extractInstance(agent1Id));
					if (a.status !== 'idle') throw new Error(`Agent still ${a.status}`);
				},
				{ ...POLL.agentIdle, message: 'Agent did not return to idle after quest 1' }
			);
		}

		// Post second quest
		const quest2 = await lifecycleApi.createQuest(
			'Name three sorting algorithms and their time complexities. One sentence each.',
			1
		);
		console.log(`[Pipeline] Posted quest 2: ${quest2.id}`);

		const finalQuest2 = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(extractInstance(quest2.id));
				if (!TERMINAL_STATUSES.has(q.status)) throw new Error(`Quest 2 still ${q.status}`);
				return q;
			},
			{ ...POLL.questTerminal, message: 'Quest 2 did not complete within 5 minutes' }
		);

		expect(finalQuest2.status).toBe('completed');
		console.log(`[Pipeline] Quest 2 completed`);

		// Verify quest 2's claiming agent returns to idle
		const agent2Id = (finalQuest2 as QuestResponse & { claimed_by?: string }).claimed_by
			?? finalQuest2.agent_id;
		if (agent2Id) {
			await retry(
				async () => {
					const a = await lifecycleApi.getAgent(extractInstance(agent2Id));
					if (a.status !== 'idle') throw new Error(`Agent still ${a.status}`);
				},
				{ ...POLL.agentIdle, message: 'Agent did not return to idle after quest 2' }
			);
			console.log(`[Pipeline] Agent ${agent2Id} returned to idle after quest 2`);
		}
	});
});

// =============================================================================
// GROUP B: Dashboard Reactivity
// =============================================================================

test.describe('Dashboard Reactivity @integration @ollama', () => {
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasOllama(),
			'Requires running backend and Ollama'
		);
	});

	test('B1: dashboard updates reactively when quest is posted and completed', async ({ page }) => {
		test.setTimeout(360_000);

		// Navigate to dashboard and wait for hydration
		await page.goto('/');
		await page.locator('body.hydrated').waitFor({ state: 'attached', timeout: 10_000 });

		// Wait for SSE sync (quest list should be present after sync)
		await page.waitForSelector('[data-testid="nav-quests"]', { timeout: 10_000 });

		// Capture initial quest count from nav
		const navCountBefore = await page.locator('[data-testid="nav-count-quests"]').textContent();
		const questCountBefore = parseInt(navCountBefore ?? '0', 10);

		// Post a quest via API (not through the UI)
		const res = await fetch(`${API_URL}/game/quests`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				objective: 'Dashboard reactivity test: explain what recursion is in one sentence.',
				hints: { suggested_difficulty: 1, require_human_review: false, budget: 100 }
			})
		});
		expect(res.ok, `quest creation failed: ${res.status}`).toBeTruthy();
		const quest: QuestResponse = await res.json();
		console.log(`[Reactivity] Posted quest ${quest.id}`);

		// Verify the nav quest count updates WITHOUT page refresh
		await expect(async () => {
			const navCount = await page.locator('[data-testid="nav-count-quests"]').textContent();
			const count = parseInt(navCount ?? '0', 10);
			expect(count).toBeGreaterThan(questCountBefore);
		}).toPass({ timeout: POLL.uiUpdate.timeout });

		console.log('[Reactivity] Quest count updated reactively');

		// Wait for quest to complete autonomously
		await retry(
			async () => {
				const q = await (await fetch(`${API_URL}/game/quests/${extractInstance(quest.id)}`)).json();
				if (!TERMINAL_STATUSES.has(q.status)) throw new Error(`Quest still ${q.status}`);
			},
			{ ...POLL.questTerminal, message: 'Quest did not complete' }
		);

		// Verify the activity feed has events (if visible)
		const activityFeed = page.locator('[data-testid="event-feed"]');
		if (await activityFeed.isVisible()) {
			const eventItems = activityFeed.locator('[data-testid="event-item"]');
			await expect(async () => {
				const count = await eventItems.count();
				expect(count).toBeGreaterThan(0);
			}).toPass({ timeout: POLL.uiUpdate.timeout });
			console.log('[Reactivity] Activity feed populated');
		}
	});

	test('B2: activity feed shows events on non-dashboard routes', async ({ page }) => {
		test.setTimeout(120_000);

		// Navigate to quests page
		await page.goto('/quests');
		await page.locator('body.hydrated').waitFor({ state: 'attached', timeout: 10_000 });

		// Activity feed should be visible (showActivity defaults to true)
		const activityFeed = page.locator('[data-testid="event-feed"]');
		await expect(activityFeed).toBeVisible({ timeout: 10_000 });

		// Post a quest to trigger events
		const res = await fetch(`${API_URL}/game/quests`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				objective: 'Activity feed test: what is a binary tree?',
				hints: { suggested_difficulty: 1, require_human_review: false, budget: 100 }
			})
		});
		expect(res.ok).toBeTruthy();
		console.log('[Reactivity] Posted quest for activity feed test');

		// Wait for at least one event to appear in the feed
		const eventItems = activityFeed.locator('[data-testid="event-item"]');
		await expect(async () => {
			const count = await eventItems.count();
			expect(count).toBeGreaterThan(0);
		}).toPass({ timeout: POLL.questTerminal.timeout });

		console.log('[Reactivity] Events visible on quests page');
	});
});
