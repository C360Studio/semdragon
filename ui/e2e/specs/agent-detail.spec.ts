import { test, expect, hasBackend, hasLLM, extractInstance, retry, waitForHydration } from '../fixtures/test-base';

/**
 * Agent Detail Page
 *
 * Tests for the full agent profile page at /agents/[id].
 *
 * The page has two logical sections:
 *   1. Static profile info — name, tier, level, XP bar, stats cards, lifecycle
 *   2. Tabbed history — quests, parties, boss battles, collaborators
 *
 * "Page structure" tests work without a backend because the worldStore is
 * seeded from SSE on startup; any agent already in the roster is reachable
 * by navigating to it from the agents list.
 *
 * "History" tests seed fresh data via the lifecycle API so they are
 * deterministic regardless of pre-existing board state.
 */

// ---------------------------------------------------------------------------
// Page Structure (no backend required beyond what seeds the roster)
// ---------------------------------------------------------------------------

test.describe('Agent Detail - Page Structure', () => {
	test('displays agent name and tier badge', async ({ page, agentsPage }) => {
		// Navigate to the roster, pick the first agent that has a profile link,
		// and verify the detail page shows the matching name and a tier badge.
		await agentsPage.goto();

		const cardCount = await agentsPage.getVisibleAgentCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		const cardDetails = await agentsPage.getAgentCardDetails(0);
		await agentsPage.goToAgentProfile();
		await waitForHydration(page);

		await expect(page.locator('[data-testid="agent-name"]')).toBeVisible();

		const name = await page.locator('[data-testid="agent-name"]').textContent();
		expect(name?.trim()).toBe(cardDetails.name);

		await expect(page.locator('.tier-badge').first()).toBeVisible();
		const tier = await page.locator('.tier-badge').first().textContent();
		expect(tier?.trim().toLowerCase()).toMatch(/apprentice|journeyman|expert|master|grandmaster/);
	});

	test('displays level card with XP bar', async ({ page, agentsPage }) => {
		await agentsPage.goto();

		const cardCount = await agentsPage.getVisibleAgentCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		await agentsPage.selectAgent(0);
		await agentsPage.goToAgentProfile();
		await waitForHydration(page);

		await expect(page.locator('[data-testid="agent-level"]')).toBeVisible();

		// The level value is a large number; verify it parses as a positive integer
		const levelText = await page.locator('[data-testid="agent-level"] .level-value').textContent();
		const level = parseInt(levelText?.trim() ?? '0', 10);
		expect(level).toBeGreaterThanOrEqual(1);

		// XP bar should be present inside the level card
		await expect(page.locator('[data-testid="agent-level"] .xp-bar')).toBeVisible();
	});

	test('displays lifetime stats card', async ({ page, agentsPage }) => {
		await agentsPage.goto();

		const cardCount = await agentsPage.getVisibleAgentCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		await agentsPage.selectAgent(0);
		await agentsPage.goToAgentProfile();
		await waitForHydration(page);

		// The stats card uses a <dl class="stats-grid"> inside a .detail-card
		await expect(page.locator('.stats-grid')).toBeVisible();

		// Must show at least the quests-completed stat
		const statsText = await page.locator('.stats-grid').textContent();
		expect(statsText).toContain('Quests Completed');
	});

	test('displays back link to roster', async ({ page, agentsPage }) => {
		await agentsPage.goto();

		const cardCount = await agentsPage.getVisibleAgentCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		await agentsPage.selectAgent(0);
		await agentsPage.goToAgentProfile();
		await waitForHydration(page);

		const backLink = page.locator('.back-link');
		await expect(backLink).toBeVisible();
		await expect(backLink).toHaveAttribute('href', '/agents');
		await expect(backLink).toContainText('Back to Agent Roster');
	});

	test('shows not-found state for invalid agent ID', async ({ page }) => {
		await page.goto('/agents/definitely-not-a-real-agent-id-xyz987');
		await waitForHydration(page);

		await expect(page.locator('[data-testid="agent-not-found"]')).toBeVisible();
		await expect(page.locator('[data-testid="agent-not-found"]')).toContainText('Agent not found');
	});
});

// ---------------------------------------------------------------------------
// History Tabs (require backend so worldStore has live data)
// ---------------------------------------------------------------------------

test.describe('Agent Detail - History Tabs', () => {
	test('shows history section with tab bar', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const agent = await lifecycleApi.recruitAgent('e2e-tabs-structure-agent');
		const agentInstance = extractInstance(agent.id);

		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		await expect(page.locator('.history-section')).toBeVisible();
		await expect(page.locator('.tab-bar[role="tablist"]')).toBeVisible();

		// Should have exactly 4 tabs: Quests, Parties, Boss Battles, Collaborators
		const tabs = page.locator('.tab-bar [role="tab"]');
		await expect(tabs).toHaveCount(4);
	});

	test('defaults to quests tab', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const agent = await lifecycleApi.recruitAgent('e2e-tabs-default-agent');
		const agentInstance = extractInstance(agent.id);

		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		// The quests tab should be active (aria-selected="true") on first load
		const questsTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Quests' });
		await expect(questsTab).toHaveAttribute('aria-selected', 'true');

		// The tab panel should be present
		await expect(page.locator('[role="tabpanel"]')).toBeVisible();
	});

	test('switches between tabs', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const agent = await lifecycleApi.recruitAgent('e2e-tabs-switch-agent');
		const agentInstance = extractInstance(agent.id);

		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		// Click the "Boss Battles" tab
		const battlesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Boss Battles' });
		await battlesTab.click();
		await expect(battlesTab).toHaveAttribute('aria-selected', 'true');

		// Click the "Parties" tab
		const partiesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Parties' });
		await partiesTab.click();
		await expect(partiesTab).toHaveAttribute('aria-selected', 'true');

		// Click back to "Quests"
		const questsTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Quests' });
		await questsTab.click();
		await expect(questsTab).toHaveAttribute('aria-selected', 'true');
	});

	test('tab counts reflect agent data', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		// A fresh agent has no quests yet — all counts should be 0
		const agent = await lifecycleApi.recruitAgent('e2e-tabs-count-agent');
		const agentInstance = extractInstance(agent.id);

		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		// Count badges are `.tab-count` spans inside each tab button.
		// A newly recruited agent has 0 quests, parties, and battles.
		const questsTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Quests' });
		const questCountEl = questsTab.locator('.tab-count');
		if ((await questCountEl.count()) > 0) {
			const count = parseInt((await questCountEl.textContent()) ?? '0', 10);
			expect(count).toBe(0);
		}

		// Collaborators tab has no count badge — verify the tab still renders
		const collabTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Collaborators' });
		await expect(collabTab).toBeVisible();
		const collabCountEl = collabTab.locator('.tab-count');
		expect(await collabCountEl.count()).toBe(0);
	});
});

// ---------------------------------------------------------------------------
// Quest History
// ---------------------------------------------------------------------------

test.describe('Agent Detail - Quest History', () => {
	test('shows quest history after completing a quest', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(120_000);

		// 1. Recruit agent and create an easy quest (no LLM required — manual lifecycle)
		const agent = await lifecycleApi.recruitAgent('e2e-quest-history-agent');
		const agentInstance = extractInstance(agent.id);

		const quest = await lifecycleApi.createQuest('E2E quest history test quest', 1);
		const questInstance = extractInstance(quest.id);

		// 2. Run the quest through to completion manually
		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// Submit with output (bossbattle will auto-evaluate; we don't need LLM mode
		// since we only care about the quest showing up, not the review result)
		const submitRes = await lifecycleApi.submitQuest(questInstance, 'E2E test output');
		expect(submitRes.ok, `submit failed: ${submitRes.status}`).toBeTruthy();

		// 3. Wait for the quest to be associated with the agent in world state,
		//    then navigate to the agent detail page and verify history
		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		// The SSE feed should propagate the quest update — wait for it to appear
		await expect(async () => {
			const historyList = page.locator('.quest-history .history-list');
			await expect(historyList).toBeVisible({ timeout: 5000 });
			const itemCount = await historyList.locator('li').count();
			expect(itemCount).toBeGreaterThanOrEqual(1);
		}).toPass({ timeout: 30_000 });
	});

	test('quest summary shows completed count', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.skip(!hasLLM(), 'Requires LLM for quest auto-completion');
		test.setTimeout(120_000);

		// This test requires the full agentic loop so the quest reaches "completed".
		const agent = await lifecycleApi.recruitAgent('e2e-quest-summary-agent');
		const agentInstance = extractInstance(agent.id);

		const quest = await lifecycleApi.createQuest('E2E quest summary test quest', 1);
		const questInstance = extractInstance(quest.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// Wait for quest to auto-complete via agentic loop
		await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (!['completed', 'failed'].includes(q.status)) {
					throw new Error(`Quest not finished, status: ${q.status}`);
				}
			},
			{ timeout: 90_000, interval: 2000, message: 'Quest did not complete' }
		);

		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		// The summary row shows "N completed" and "N failed" chips
		await expect(async () => {
			const summaryRow = page.locator('.quest-history .summary-row');
			await expect(summaryRow).toBeVisible({ timeout: 5000 });
			const chips = summaryRow.locator('.summary-chip');
			const count = await chips.count();
			expect(count).toBeGreaterThanOrEqual(2); // at minimum "N completed" and "N failed"
		}).toPass({ timeout: 15_000 });
	});
});

// ---------------------------------------------------------------------------
// Battle History
// ---------------------------------------------------------------------------

test.describe('Agent Detail - Battle History', () => {
	test('shows battle record after boss battle', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend() || !hasLLM(), 'Requires running backend and LLM');
		test.setTimeout(120_000);

		// 1. Recruit agent and create a quest that forces review (which spawns a battle)
		const agent = await lifecycleApi.recruitAgent('e2e-battle-history-agent');
		const agentInstance = extractInstance(agent.id);

		const quest = await lifecycleApi.createQuestWithReview('E2E battle history quest', 1);
		const questInstance = extractInstance(quest.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// 2. Wait for a battle to be created for this quest
		await retry(
			async () => {
				const battles = await lifecycleApi.listBattles();
				const match = battles.find(
					(b) =>
						b.quest_id === quest.id ||
						extractInstance(b.quest_id ?? '') === questInstance
				);
				if (!match) {
					throw new Error('Battle not created yet');
				}
			},
			{ timeout: 90_000, interval: 2000, message: 'No battle appeared for quest' }
		);

		// 3. Navigate to the agent detail page and verify the battle appears
		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		const battlesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Boss Battles' });
		await battlesTab.click();

		await expect(async () => {
			const historyList = page.locator('.battle-history .history-list');
			await expect(historyList).toBeVisible({ timeout: 5000 });
			const itemCount = await historyList.locator('li').count();
			expect(itemCount).toBeGreaterThanOrEqual(1);
		}).toPass({ timeout: 30_000 });
	});

	test('battle summary shows W/L stats', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend() || !hasLLM(), 'Requires running backend and LLM');
		test.setTimeout(120_000);

		const agent = await lifecycleApi.recruitAgent('e2e-battle-summary-agent');
		const agentInstance = extractInstance(agent.id);

		const quest = await lifecycleApi.createQuestWithReview('E2E battle summary quest', 1);
		const questInstance = extractInstance(quest.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		// Wait for a resolved battle (victory or defeat)
		await retry(
			async () => {
				const battles = await lifecycleApi.listBattles();
				const match = battles.find(
					(b) =>
						(b.quest_id === quest.id ||
							extractInstance(b.quest_id ?? '') === questInstance) &&
						['victory', 'defeat'].includes(b.status ?? '')
				);
				if (!match) {
					throw new Error('No resolved battle yet');
				}
			},
			{ timeout: 90_000, interval: 2000, message: 'Battle did not resolve' }
		);

		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		const battlesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Boss Battles' });
		await battlesTab.click();

		// The summary row shows "NW", "NL", and optionally "N% win rate"
		await expect(async () => {
			const summaryRow = page.locator('.battle-history .summary-row');
			await expect(summaryRow).toBeVisible({ timeout: 5000 });

			const chips = summaryRow.locator('.summary-chip');
			const count = await chips.count();
			// At minimum: wins chip and losses chip
			expect(count).toBeGreaterThanOrEqual(2);

			// The first chip should end in "W" and the second in "L"
			const winsText = (await chips.nth(0).textContent()) ?? '';
			const lossesText = (await chips.nth(1).textContent()) ?? '';
			expect(winsText.trim()).toMatch(/^\d+W$/);
			expect(lossesText.trim()).toMatch(/^\d+L$/);
		}).toPass({ timeout: 15_000 });
	});
});

// ---------------------------------------------------------------------------
// Party History
// ---------------------------------------------------------------------------

test.describe('Agent Detail - Party History', () => {
	test('shows party history after party quest', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(120_000);

		// Recruit a Master-tier lead (level 16) so partycoord can auto-form a party
		await lifecycleApi.recruitAgentAtLevel(`e2e-party-hist-lead-${Date.now()}`, 16, [
			'analysis',
			'coding'
		]);

		// Recruit a second member so the party has at least 2 agents
		const member = await lifecycleApi.recruitAgent(`e2e-party-hist-member-${Date.now()}`);
		const memberInstance = extractInstance(member.id);

		// Create a party-required quest — partycoord will auto-form a party
		const quest = await lifecycleApi.createQuestWithParty('E2E party history quest', 2);
		const questInstance = extractInstance(quest.id);

		// Wait for a party to form and claim the quest
		await retry(
			async () => {
				const parties = await lifecycleApi.listParties();
				const match = parties.find(
					(p) =>
						extractInstance(p.quest_id) === questInstance ||
						p.quest_id === quest.id
				);
				if (!match) {
					throw new Error('Party not formed yet');
				}
			},
			{ timeout: 60_000, interval: 2000, message: 'Party did not form within 60s' }
		);

		// Navigate to the member's detail page and check the parties tab
		// (The member agent is idle so partycoord should pick it up)
		await page.goto(`/agents/${memberInstance}`);
		await waitForHydration(page);

		const partiesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Parties' });
		await partiesTab.click();

		await expect(async () => {
			const historyList = page.locator('.party-history .history-list');
			await expect(historyList).toBeVisible({ timeout: 5000 });
			const itemCount = await historyList.locator('li').count();
			expect(itemCount).toBeGreaterThanOrEqual(1);
		}).toPass({ timeout: 30_000 });
	});
});

// ---------------------------------------------------------------------------
// Collaborators
// ---------------------------------------------------------------------------

test.describe('Agent Detail - Collaborators', () => {
	test('shows collaborators from shared party', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(120_000);

		// Same setup as party history: recruit a lead + a member, form a party
		await lifecycleApi.recruitAgentAtLevel(`e2e-collab-lead-${Date.now()}`, 16, [
			'analysis',
			'coding'
		]);

		const member = await lifecycleApi.recruitAgent(`e2e-collab-member-${Date.now()}`);
		const memberInstance = extractInstance(member.id);

		const quest = await lifecycleApi.createQuestWithParty('E2E collaborators quest', 2);
		const questInstance = extractInstance(quest.id);

		await retry(
			async () => {
				const parties = await lifecycleApi.listParties();
				const match = parties.find(
					(p) =>
						extractInstance(p.quest_id) === questInstance ||
						p.quest_id === quest.id
				);
				if (!match) {
					throw new Error('Party not formed yet');
				}
			},
			{ timeout: 60_000, interval: 2000, message: 'Party did not form within 60s' }
		);

		// Navigate to the member's detail page and check the collaborators tab
		await page.goto(`/agents/${memberInstance}`);
		await waitForHydration(page);

		const collabTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Collaborators' });
		await collabTab.click();

		await expect(async () => {
			const collabList = page.locator('.collaborators .collaborator-list');
			await expect(collabList).toBeVisible({ timeout: 5000 });
			const itemCount = await collabList.locator('li').count();
			// At least the lead agent should appear as a collaborator
			expect(itemCount).toBeGreaterThanOrEqual(1);
		}).toPass({ timeout: 30_000 });
	});
});

// ---------------------------------------------------------------------------
// Empty states
// ---------------------------------------------------------------------------

test.describe('Agent Detail - Empty States', () => {
	test('new agent shows empty state for all history tabs', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const agent = await lifecycleApi.recruitAgent('e2e-empty-state-agent');
		const agentInstance = extractInstance(agent.id);

		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);

		// Quests tab (default) — fresh agent has no quests
		const questsTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Quests' });
		await questsTab.click();
		await expect(page.locator('[role="tabpanel"] .empty-state')).toBeVisible();

		// Parties tab
		const partiesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Parties' });
		await partiesTab.click();
		await expect(page.locator('[role="tabpanel"] .empty-state')).toBeVisible();

		// Boss Battles tab
		const battlesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Boss Battles' });
		await battlesTab.click();
		await expect(page.locator('[role="tabpanel"] .empty-state')).toBeVisible();

		// Collaborators tab
		const collabTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Collaborators' });
		await collabTab.click();
		await expect(page.locator('[role="tabpanel"] .empty-state')).toBeVisible();
	});
});
