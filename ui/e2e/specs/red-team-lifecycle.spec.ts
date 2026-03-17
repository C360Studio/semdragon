import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';

/**
 * Red-Team Review Lifecycle — Integration Tests (Tier 1)
 *
 * Exercises the red-team review processor via API calls:
 *   1. Quest with review → submitted → red-team quest auto-posted
 *   2. Red-team quest has correct classification fields
 *   3. Red-team quest completion triggers boss battle on original
 *   4. Lessons written to guild after review cycle
 *   5. Skip path: quests below min difficulty bypass red-team
 *
 * Prerequisites:
 *   - Backend running with redteam processor enabled
 *   - bossbattle.red_team_enabled: true
 *   - No LLM required (manual state transitions via API)
 *
 * Note: The redteam processor watches for in_review transitions and posts
 * red-team quests automatically. These tests drive the original quest through
 * its lifecycle and then verify the red-team quest appeared.
 */

test.describe('Red-Team Review - Lifecycle', () => {
	test('submitted quest with review triggers red-team quest', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(30_000);

		// 1. Create a quest that requires review (difficulty >= moderate = 2)
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E red-team lifecycle: implement user auth module',
			1 // ReviewStandard
		);
		const questInstance = extractInstance(quest.id);

		// 2. Recruit agent, claim, start, and submit
		const agent = await lifecycleApi.recruitAgent('rt-lifecycle-agent', ['codegen']);
		const agentInstance = extractInstance(agent.id);

		const claimRes = await lifecycleApi.claimQuest(questInstance, agentInstance);
		expect(claimRes.ok, `claim failed: ${claimRes.status}`).toBeTruthy();

		const startRes = await lifecycleApi.startQuest(questInstance);
		expect(startRes.ok, `start failed: ${startRes.status}`).toBeTruthy();

		const submitRes = await lifecycleApi.submitQuest(questInstance, JSON.stringify({
			code: 'func AuthUser(ctx context.Context, token string) (*User, error) { ... }',
			tests: 'TestAuthUser_ValidToken, TestAuthUser_ExpiredToken'
		}));
		expect(submitRes.ok, `submit failed: ${submitRes.status}`).toBeTruthy();

		// 3. Wait for the red-team quest to appear
		const redTeamQuest = await retry(
			async () => {
				const quests = await lifecycleApi.listQuests();
				const rt = quests.find(
					(q) => q.quest_type === 'red_team_review' && q.red_team_target === quest.id
				);
				if (!rt) {
					throw new Error('Red-team quest not found yet');
				}
				return rt;
			},
			{ timeout: 15_000, interval: 1000, message: 'Red-team quest was not posted' }
		);

		expect(redTeamQuest.quest_type).toBe('red_team_review');
		expect(redTeamQuest.red_team_target).toBe(quest.id);
		expect(redTeamQuest.title).toContain('Red-Team Review');
	});

	test('red-team quest has matching skills from original', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(30_000);

		// Create a quest with specific skills
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E red-team skills: analyze database performance bottlenecks',
			1
		);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('rt-skills-agent', ['analysis']);
		const agentInstance = extractInstance(agent.id);

		await lifecycleApi.claimQuest(questInstance, agentInstance);
		await lifecycleApi.startQuest(questInstance);
		await lifecycleApi.submitQuest(questInstance, 'Performance analysis: indexing needed on users.email');

		// Wait for red-team quest
		const rtQuest = await retry(
			async () => {
				const quests = await lifecycleApi.listQuests();
				const rt = quests.find(
					(q) => q.quest_type === 'red_team_review' && q.red_team_target === quest.id
				);
				if (!rt) throw new Error('Waiting for red-team quest');
				return rt;
			},
			{ timeout: 15_000, interval: 1000, message: 'Red-team quest not posted' }
		);

		// Red-team quest should inherit required skills from original
		expect(rtQuest.required_skills).toEqual(quest.required_skills);
	});

	test('trivial quest skips red-team review', async ({ lifecycleApi, apiRequest }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(15_000);

		// Create a trivial (difficulty=0) quest with review enabled.
		// Redteam min_difficulty is 1 (easy), so trivial quests should be skipped.
		const createRes = await apiRequest.post('/game/quests', {
			data: {
				objective: 'E2E red-team skip: trivial task',
				hints: {
					suggested_difficulty: 0,
					suggested_skills: [],
					require_human_review: true,
					review_level: 1,
					budget: 100
				}
			}
		});
		expect(createRes.ok()).toBeTruthy();
		const quest = await createRes.json();
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('rt-skip-agent');
		const agentInstance = extractInstance(agent.id);

		await lifecycleApi.claimQuest(questInstance, agentInstance);
		await lifecycleApi.startQuest(questInstance);
		await lifecycleApi.submitQuest(questInstance, 'Simple output');

		// Wait a bit, then verify no red-team quest was posted.
		// The redteam processor's min_difficulty is set to 1 (easy) in config,
		// so difficulty=0 (trivial) quests should be skipped.
		await new Promise((resolve) => setTimeout(resolve, 3000));

		const quests = await lifecycleApi.listQuests();
		const rtQuest = quests.find(
			(q) => q.quest_type === 'red_team_review' && q.red_team_target === quest.id
		);

		// No red-team quest should exist for this trivial quest
		expect(rtQuest).toBeUndefined();
	});
});

test.describe('Red-Team Review - Boss Battle Coordination', () => {
	test('boss battle starts after red-team completes', async ({ lifecycleApi }) => {
		test.skip(!hasBackend(), 'Requires running backend');
		// This test exercises the full reactive pipeline: redteam processor watches
		// the red-team quest KV, emits PredicateRedTeamCompleted on the original,
		// bossbattle sees the signal and starts the battle. KV propagation can take
		// several seconds depending on watcher timing.
		test.setTimeout(60_000);

		// 1. Create, claim, start, submit a quest with review
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E red-team battle: implement caching layer',
			1
		);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('rt-battle-agent', ['codegen']);
		const agentInstance = extractInstance(agent.id);

		await lifecycleApi.claimQuest(questInstance, agentInstance);
		await lifecycleApi.startQuest(questInstance);
		await lifecycleApi.submitQuest(questInstance, JSON.stringify({
			code: 'type Cache struct { mu sync.RWMutex; store map[string]any }',
			tests: 'TestCache_SetGet, TestCache_Expiry'
		}));

		// 2. Wait for red-team quest to appear
		const rtQuest = await retry(
			async () => {
				const quests = await lifecycleApi.listQuests();
				const rt = quests.find(
					(q) => q.quest_type === 'red_team_review' && q.red_team_target === quest.id
				);
				if (!rt) throw new Error('Waiting for red-team quest');
				return rt;
			},
			{ timeout: 15_000, interval: 1000, message: 'Red-team quest not posted' }
		);

		const rtInstance = extractInstance(rtQuest.id);

		// 3. Manually drive the red-team quest to completion.
		//    In real usage the agentic loop handles this. In T1 we use the API.
		const rtAgent = await lifecycleApi.recruitAgent('rt-reviewer-agent', ['codegen']);
		const rtAgentInstance = extractInstance(rtAgent.id);

		const rtClaimRes = await lifecycleApi.claimQuest(rtInstance, rtAgentInstance);
		expect(rtClaimRes.ok, `RT claim failed: ${rtClaimRes.status}`).toBeTruthy();

		const rtStartRes = await lifecycleApi.startQuest(rtInstance);
		expect(rtStartRes.ok, `RT start failed: ${rtStartRes.status}`).toBeTruthy();

		// Complete the red-team quest directly (it has RequireReview=false).
		const rtCompleteRes = await lifecycleApi.completeQuest(rtInstance);
		expect(rtCompleteRes.ok, `RT complete failed: ${rtCompleteRes.status}`).toBeTruthy();

		// 4. After red-team completes, boss battle should start on the original quest.
		//    The original quest should eventually reach completed or failed (battle verdict).
		const finalQuest = await retry(
			async () => {
				const q = await lifecycleApi.getQuest(questInstance);
				if (q.status === 'in_review') throw new Error('Still in review, waiting for battle');
				return q;
			},
			{ timeout: 45_000, interval: 2000, message: 'Original quest did not leave in_review after red-team' }
		);

		// Quest should have a verdict (boss battle ran)
		expect(['completed', 'failed', 'posted']).toContain(finalQuest.status);
	});
});

test.describe('Red-Team Review - Quest Board Display', () => {
	test('red-team quests visible in quest list with correct type', async ({
		page,
		lifecycleApi
	}) => {
		test.skip(!hasBackend(), 'Requires running backend');
		test.setTimeout(30_000);

		// Create and submit a quest to trigger red-team
		const quest = await lifecycleApi.createQuestWithReview(
			'E2E red-team display: build notification service',
			1
		);
		const questInstance = extractInstance(quest.id);

		const agent = await lifecycleApi.recruitAgent('rt-display-agent', ['codegen']);
		const agentInstance = extractInstance(agent.id);

		await lifecycleApi.claimQuest(questInstance, agentInstance);
		await lifecycleApi.startQuest(questInstance);
		await lifecycleApi.submitQuest(questInstance, 'Notification service implementation');

		// Wait for red-team quest to exist
		await retry(
			async () => {
				const quests = await lifecycleApi.listQuests();
				const rt = quests.find((q) => q.quest_type === 'red_team_review');
				if (!rt) throw new Error('Waiting for red-team quest in list');
				return rt;
			},
			{ timeout: 15_000, interval: 1000, message: 'Red-team quest not in quest list' }
		);

		// Navigate to quests page and verify the red-team quest is visible
		await page.goto('/quests');
		await page.waitForLoadState('domcontentloaded');

		// The quest board should show the red-team quest card
		// (It will appear as a posted quest since no agent has claimed it yet)
		const redTeamCard = page.locator('[data-testid="quest-card"]', {
			hasText: /Red-Team Review/
		});

		// Allow time for SSE to deliver the quest to the UI
		await expect(redTeamCard.first()).toBeVisible({ timeout: 10_000 });
	});
});
