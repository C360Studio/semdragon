import { test, expect, hasBackend, extractInstance, waitForHydration } from '../fixtures/test-base';

/**
 * Agent Detail Page — Tier 1 (UI Structure)
 *
 * Tests page structure, tab switching, and empty states.
 * LLM-dependent history verification (quest/battle/party/collaborator
 * history tabs with real data) is covered by the tier 2 quest-pipeline
 * aftermath test.
 */

/**
 * Navigate to an agent's detail page by instance ID and wait for SSE to
 * propagate the entity into the worldStore so the page renders.
 */
async function gotoAgentDetail(page: import('@playwright/test').Page, agentInstance: string) {
	// The worldStore is populated from a snapshot on page load + SSE deltas.
	// A freshly recruited agent may not be in the snapshot yet, so we reload
	// until the agent appears (each reload fetches a fresh snapshot).
	await expect(async () => {
		await page.goto(`/agents/${agentInstance}`);
		await waitForHydration(page);
		await expect(page.locator('[data-testid="agent-name"]')).toBeVisible({ timeout: 5_000 });
	}).toPass({ timeout: 30_000 });
}

// ---------------------------------------------------------------------------
// Page Structure (no backend required beyond what seeds the roster)
// ---------------------------------------------------------------------------

test.describe('Agent Detail - Page Structure', () => {
	test('displays agent name and tier badge', async ({ page, agentsPage }) => {
		await agentsPage.goto();

		const cardCount = await agentsPage.getVisibleAgentCount();
		if (cardCount === 0) {
			test.skip();
			return;
		}

		const cardDetails = await agentsPage.getAgentCardDetails(0);
		await agentsPage.selectAgent(0);
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

		const levelText = await page.locator('[data-testid="agent-level"] .level-value').textContent();
		const level = parseInt(levelText?.trim() ?? '0', 10);
		expect(level).toBeGreaterThanOrEqual(1);

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

		await expect(page.locator('.stats-grid')).toBeVisible();

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

		await gotoAgentDetail(page, agentInstance);

		await expect(page.locator('.history-section')).toBeVisible();
		await expect(page.locator('.tab-bar[role="tablist"]')).toBeVisible();

		const tabs = page.locator('.tab-bar [role="tab"]');
		await expect(tabs).toHaveCount(4);
	});

	test('defaults to quests tab', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const agent = await lifecycleApi.recruitAgent('e2e-tabs-default-agent');
		const agentInstance = extractInstance(agent.id);

		await gotoAgentDetail(page, agentInstance);

		const questsTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Quests' });
		await expect(questsTab).toHaveAttribute('aria-selected', 'true');

		await expect(page.locator('[role="tabpanel"]')).toBeVisible();
	});

	test('switches between tabs', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const agent = await lifecycleApi.recruitAgent('e2e-tabs-switch-agent');
		const agentInstance = extractInstance(agent.id);

		await gotoAgentDetail(page, agentInstance);

		const battlesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Boss Battles' });
		await battlesTab.click();
		await expect(battlesTab).toHaveAttribute('aria-selected', 'true');

		const partiesTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Parties' });
		await partiesTab.click();
		await expect(partiesTab).toHaveAttribute('aria-selected', 'true');

		const questsTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Quests' });
		await questsTab.click();
		await expect(questsTab).toHaveAttribute('aria-selected', 'true');
	});

	test('tab counts reflect agent data', async ({ page, lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');

		const agent = await lifecycleApi.recruitAgent('e2e-tabs-count-agent');
		const agentInstance = extractInstance(agent.id);

		await gotoAgentDetail(page, agentInstance);

		const questsTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Quests' });
		const questCountEl = questsTab.locator('.tab-count');
		if ((await questCountEl.count()) > 0) {
			const count = parseInt((await questCountEl.textContent()) ?? '0', 10);
			expect(count).toBe(0);
		}

		const collabTab = page.locator('.tab-bar [role="tab"]').filter({ hasText: 'Collaborators' });
		await expect(collabTab).toBeVisible();
		const collabCountEl = collabTab.locator('.tab-count');
		expect(await collabCountEl.count()).toBe(0);
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

		await gotoAgentDetail(page, agentInstance);

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
