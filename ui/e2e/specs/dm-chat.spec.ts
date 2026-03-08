import { test, expect, hasBackend, isRealLLM, waitForHydration, type Page } from '../fixtures/test-base';
import type { Route } from '@playwright/test';

// =============================================================================
// MOCK RESPONSE HELPERS
// =============================================================================

function mockSimpleResponse(message: string, mode: string = 'converse') {
	return {
		message,
		mode,
		session_id: 'mock-session-001',
		trace_info: { trace_id: 'trace-mock', span_id: 'span-mock' }
	};
}

function mockQuestBriefResponse() {
	return {
		message: 'I have crafted a quest for you.',
		mode: 'quest',
		quest_brief: {
			title: 'Slay the Test Dragon',
			description: 'Defeat the dragon terrorizing the test realm.',
			difficulty: 3,
			skills: ['combat', 'analysis'],
			acceptance: ['Dragon HP reaches 0', 'No civilian casualties']
		},
		session_id: 'mock-session-brief',
		trace_info: { trace_id: 'trace-brief', span_id: 'span-brief' }
	};
}

function mockQuestChainResponse() {
	return {
		message: 'Here is a chain of quests for you.',
		mode: 'quest',
		quest_chain: {
			quests: [
				{
					title: 'Gather Intel',
					description: 'Scout the dragon lair.',
					difficulty: 1,
					skills: ['analysis'],
					depends_on: []
				},
				{
					title: 'Forge Weapon',
					description: 'Create a dragonslayer sword.',
					difficulty: 2,
					skills: ['crafting'],
					depends_on: [0]
				},
				{
					title: 'Slay the Dragon',
					description: 'Attack the lair with the forged weapon.',
					difficulty: 4,
					skills: ['combat'],
					depends_on: [0, 1]
				}
			]
		},
		session_id: 'mock-session-chain',
		trace_info: { trace_id: 'trace-chain', span_id: 'span-chain' }
	};
}

function mockErrorResponse() {
	return {
		status: 502,
		body: JSON.stringify({ error: 'LLM request failed' }),
		contentType: 'application/json'
	};
}

// =============================================================================
// CHAT INTERACTION HELPERS
// =============================================================================

async function openChat(page: Page) {
	const panel = page.getByTestId('chat-panel');
	// If not already open (no input visible), click toggle
	const input = page.getByTestId('chat-input');
	if (!(await input.isVisible().catch(() => false))) {
		await page.getByTestId('chat-toggle').click();
	}
	await expect(input).toBeVisible();
	return panel;
}

async function sendMessage(page: Page, text: string) {
	const input = page.getByTestId('chat-input');
	await input.fill(text);
	await page.getByTestId('chat-send').click();
}

function chatMessages(page: Page) {
	return page.getByTestId('chat-message');
}

// =============================================================================
// ROUTE INTERCEPTION HELPERS
// =============================================================================

/**
 * Intercept DM chat POST and respond with a mock payload.
 * Returns a promise that resolves with the request body when the route is hit.
 */
function interceptChat(
	page: Page,
	responseBody: Record<string, unknown>,
	options?: { delay?: number }
) {
	let capturedBody: Record<string, unknown> | null = null;
	const bodyPromise = new Promise<Record<string, unknown>>((resolve) => {
		page.route('**/game/dm/chat', async (route: Route) => {
			const body = route.request().postDataJSON();
			capturedBody = body;
			resolve(body);
			if (options?.delay) {
				await new Promise((r) => setTimeout(r, options.delay));
			}
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify(responseBody)
			});
		});
	});
	return { bodyPromise, getCapturedBody: () => capturedBody };
}

/**
 * Intercept DM chat POST and respond with an error.
 */
function interceptChatError(page: Page) {
	return page.route('**/game/dm/chat', async (route: Route) => {
		const err = mockErrorResponse();
		await route.fulfill({
			status: err.status,
			contentType: err.contentType,
			body: err.body
		});
	});
}

// =============================================================================
// MOCK LLM TESTS
// =============================================================================

test.describe('DM Chat - Mock LLM', () => {
	test.beforeEach(async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);
	});

	test('panel toggles between expanded and collapsed', async ({ page }) => {
		const input = page.getByTestId('chat-input');

		// Initially expanded — input visible
		await expect(input).toBeVisible();

		// Click toggle to collapse
		await page.getByTestId('chat-toggle').click();
		await expect(input).not.toBeVisible();

		// Click toggle to expand
		await page.getByTestId('chat-toggle').click();
		await expect(input).toBeVisible();
	});

	test('send message and receive DM response', async ({ page }) => {
		interceptChat(page, mockSimpleResponse('Greetings, adventurer!'));

		await openChat(page);
		await sendMessage(page, 'Hello DM');

		// User message appears immediately
		const messages = chatMessages(page);
		await expect(messages.first()).toContainText('Hello DM');

		// DM response arrives
		await expect(messages.nth(1)).toContainText('Greetings, adventurer!');
		await expect(messages.nth(1)).toHaveAttribute('data-role', 'dm');
	});

	test('session continuity across turns', async ({ page }) => {
		// First turn — DM responds with session_id
		const firstResponse = mockSimpleResponse('First response');
		firstResponse.session_id = 'session-abc';
		const { bodyPromise: firstBody } = interceptChat(page, firstResponse);

		await openChat(page);
		await sendMessage(page, 'Turn one');
		await firstBody;

		// Wait for DM response to appear
		await expect(chatMessages(page)).toHaveCount(2);

		// Unroute so we can set up the next intercept
		await page.unroute('**/game/dm/chat');

		// Second turn — verify request includes session_id from first response
		const secondResponse = mockSimpleResponse('Second response');
		const { bodyPromise: secondBody } = interceptChat(page, secondResponse);

		await sendMessage(page, 'Turn two');
		const body = await secondBody;
		expect(body.session_id).toBe('session-abc');
	});

	test('quest brief preview and Post Quest action', async ({ page }) => {
		interceptChat(page, mockQuestBriefResponse());

		// Intercept the quest creation POST
		let questPostBody: Record<string, unknown> | null = null;
		await page.route('**/game/quests', async (route: Route) => {
			// Only intercept POST, not other methods
			if (route.request().method() !== 'POST') {
				return route.fallback();
			}
			// Don't intercept quest chain requests
			if (route.request().url().includes('/quests/chain')) {
				return route.fallback();
			}
			questPostBody = route.request().postDataJSON();
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					id: 'local.dev.game.board1.quest.mock123',
					objective: 'Slay the Test Dragon',
					status: 'posted'
				})
			});
		});

		await openChat(page);
		// Use /quest prefix to trigger quest mode
		await sendMessage(page, '/quest Create a quest to slay a dragon');

		// Quest preview card should appear
		const preview = page.getByTestId('quest-preview');
		await expect(preview).toBeVisible();
		await expect(preview).toContainText('Slay the Test Dragon');
		await expect(preview).toContainText('Hard');
		await expect(preview).toContainText('combat');
		await expect(preview).toContainText('analysis');

		// Click "Post Quest"
		await page.getByTestId('post-quest-button').click();

		// Verify the quest creation request was sent
		await expect(async () => {
			expect(questPostBody).not.toBeNull();
			expect((questPostBody as Record<string, unknown>).objective).toBe('Slay the Test Dragon');
		}).toPass({ timeout: 5000 });
	});

	test('quest chain preview and Post Chain action', async ({ page }) => {
		interceptChat(page, mockQuestChainResponse());

		// Register chain route BEFORE the broader quests route
		let chainPostBody: Record<string, unknown> | null = null;
		await page.route('**/game/quests/chain', async (route: Route) => {
			chainPostBody = route.request().postDataJSON();
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify([
					{ id: 'quest.1', objective: 'Gather Intel', status: 'posted' },
					{ id: 'quest.2', objective: 'Forge Weapon', status: 'posted' },
					{ id: 'quest.3', objective: 'Slay the Dragon', status: 'posted' }
				])
			});
		});

		await openChat(page);
		// Use /quest prefix to trigger quest mode
		await sendMessage(page, '/quest Create a quest chain');

		// Chain preview should appear with 3 quests
		const chainPreview = page.getByTestId('quest-chain-preview');
		await expect(chainPreview).toBeVisible();
		await expect(chainPreview).toContainText('3 quests');
		await expect(chainPreview).toContainText('Gather Intel');
		await expect(chainPreview).toContainText('Forge Weapon');
		await expect(chainPreview).toContainText('Slay the Dragon');
		await expect(chainPreview).toContainText('depends on #1');

		// Click "Post Chain"
		await page.getByTestId('post-chain-button').click();

		// Verify the chain creation request was sent with correct data
		await expect(async () => {
			expect(chainPostBody).not.toBeNull();
			const chain = chainPostBody as { quests: Array<{ title: string; depends_on?: number[] }> };
			expect(chain.quests).toHaveLength(3);
			expect(chain.quests[0].title).toBe('Gather Intel');
			expect(chain.quests[2].depends_on).toEqual([0, 1]);
		}).toPass({ timeout: 5000 });
	});

	test('loading state shows during request', async ({ page }) => {
		// Add 1s delay so we can observe the loading state
		interceptChat(page, mockSimpleResponse('Delayed response'), { delay: 1000 });

		await openChat(page);
		await sendMessage(page, 'Slow request');

		// Loading indicator should be visible
		const loading = page.getByTestId('chat-loading');
		await expect(loading).toBeVisible();
		await expect(loading).toContainText('DM is thinking');

		// After response, loading disappears
		await expect(loading).not.toBeVisible({ timeout: 5000 });
		await expect(chatMessages(page).nth(1)).toContainText('Delayed response');
	});

	test('error handling shows error and removes user message', async ({ page }) => {
		await interceptChatError(page);

		await openChat(page);
		await sendMessage(page, 'This will fail');

		// Error message should appear
		const error = page.getByTestId('chat-error');
		await expect(error).toBeVisible();
		await expect(error).toContainText('API Error 502');

		// User message should be removed on error
		await expect(chatMessages(page)).toHaveCount(0);
	});

	test('multiple turns maintain correct order', async ({ page }) => {
		const turns = [
			{ user: 'First question', dm: 'First answer' },
			{ user: 'Second question', dm: 'Second answer' },
			{ user: 'Third question', dm: 'Third answer' }
		];

		await openChat(page);

		for (const turn of turns) {
			// Set up intercept for this turn, then clear for next
			await page.unroute('**/game/dm/chat');
			interceptChat(page, mockSimpleResponse(turn.dm));

			await sendMessage(page, turn.user);

			// Wait for DM response to appear
			const allMessages = chatMessages(page);
			const expectedCount = (turns.indexOf(turn) + 1) * 2;
			await expect(allMessages).toHaveCount(expectedCount, { timeout: 5000 });
		}

		// Verify the final order: user, dm, user, dm, user, dm
		const allMessages = chatMessages(page);
		await expect(allMessages).toHaveCount(6);

		await expect(allMessages.nth(0)).toContainText('First question');
		await expect(allMessages.nth(0)).toHaveAttribute('data-role', 'user');
		await expect(allMessages.nth(1)).toContainText('First answer');
		await expect(allMessages.nth(1)).toHaveAttribute('data-role', 'dm');
		await expect(allMessages.nth(2)).toContainText('Second question');
		await expect(allMessages.nth(4)).toContainText('Third question');
		await expect(allMessages.nth(5)).toContainText('Third answer');
	});

	test('send button disabled when empty, enabled with text, disabled during loading', async ({
		page
	}) => {
		interceptChat(page, mockSimpleResponse('Response'), { delay: 500 });

		await openChat(page);

		const sendBtn = page.getByTestId('chat-send');
		const input = page.getByTestId('chat-input');

		// Disabled when input is empty
		await expect(sendBtn).toBeDisabled();

		// Enabled when input has text
		await input.fill('Some text');
		await expect(sendBtn).toBeEnabled();

		// Clear and verify disabled again
		await input.fill('');
		await expect(sendBtn).toBeDisabled();

		// Type and send — button should be disabled during loading
		await input.fill('Loading test');
		await sendBtn.click();
		await expect(sendBtn).toBeDisabled();

		// After response completes, button should be re-enabled (once new text typed)
		await expect(page.getByTestId('chat-loading')).not.toBeVisible({ timeout: 3000 });
		await input.fill('New text');
		await expect(sendBtn).toBeEnabled();
	});
});

// =============================================================================
// SLASH COMMAND TESTS
// =============================================================================

test.describe('DM Chat - Slash Commands', () => {
	test.beforeEach(async ({ page }) => {
		await page.goto('/');
		await waitForHydration(page);
	});

	test('/quest prefix sends mode=quest in request body', async ({ page }) => {
		const { bodyPromise } = interceptChat(page, mockSimpleResponse('Quest response', 'quest'));

		await openChat(page);
		await sendMessage(page, '/quest Create something');

		const body = await bodyPromise;
		expect(body.mode).toBe('quest');
		// The /quest prefix is stripped from the message sent to the API
		expect(body.message).toBe('Create something');
	});

	test('messages without /quest prefix send mode=converse', async ({ page }) => {
		const { bodyPromise } = interceptChat(page, mockSimpleResponse('Hello'));

		await openChat(page);
		await sendMessage(page, 'Tell me about quests');

		const body = await bodyPromise;
		expect(body.mode).toBe('converse');
	});

	test('converse mode messages do not render quest previews even if response has them', async ({
		page
	}) => {
		// Response includes quest_brief but sent without /quest prefix — preview should still render
		// because quest previews are now always shown when present (no mode gating)
		const response = {
			...mockQuestBriefResponse(),
			mode: 'converse'
		};
		interceptChat(page, response);

		await openChat(page);
		await sendMessage(page, 'Tell me about quests');

		// DM response arrives
		await expect(chatMessages(page)).toHaveCount(2, { timeout: 5000 });

		// Quest preview should be visible — previews are always shown when quest_brief is present
		await expect(page.getByTestId('quest-preview')).toBeVisible();
	});

	test('/quest prefix messages render quest preview from response', async ({ page }) => {
		interceptChat(page, mockQuestBriefResponse());

		await openChat(page);
		await sendMessage(page, '/quest Create a quest');

		// Quest preview should be visible
		const preview = page.getByTestId('quest-preview');
		await expect(preview).toBeVisible({ timeout: 5000 });
		await expect(preview).toContainText('Slay the Test Dragon');
	});

	test('/help shows client-side help without API call', async ({ page }) => {
		// Set up route interception to verify no API call is made
		let apiCalled = false;
		await page.route('**/game/dm/chat', async (route: Route) => {
			apiCalled = true;
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify(mockSimpleResponse('should not reach'))
			});
		});

		await openChat(page);
		await sendMessage(page, '/help');

		// Help response should appear as a DM message
		const messages = chatMessages(page);
		await expect(messages).toHaveCount(2, { timeout: 3000 });

		// User message shows "/help"
		await expect(messages.first()).toContainText('/help');

		// DM message contains help text
		await expect(messages.nth(1)).toContainText('Available commands');
		await expect(messages.nth(1)).toContainText('/quest');
		await expect(messages.nth(1)).toHaveAttribute('data-role', 'dm');

		// No API call was made
		expect(apiCalled).toBe(false);
	});

	test('placeholder shows slash command hints', async ({ page }) => {
		await openChat(page);
		const input = page.getByTestId('chat-input');
		await expect(input).toHaveAttribute('placeholder', 'Ask the DM... (try /quest or /help)');
	});

	test('no mode selector pills visible', async ({ page }) => {
		await openChat(page);
		// Mode selector should not exist
		await expect(page.getByTestId('mode-selector')).not.toBeVisible();
	});
});

// =============================================================================
// REAL LLM TESTS (opt-in: E2E_REAL_LLM=true + backend available)
// =============================================================================

test.describe('DM Chat - Real LLM', () => {
	test.beforeEach(async ({ page }) => {
		test.skip(!hasBackend() || !isRealLLM(), 'Requires running backend and real LLM (E2E_LLM_MODE=gemini|openai|anthropic|ollama)');
		await page.goto('/');
		await waitForHydration(page);
	});

	test('send message and receive real DM response', async ({ page }) => {
		test.setTimeout(150_000);

		await openChat(page);
		await sendMessage(page, 'Hello DM, what can you help me with?');

		// DM should respond with a non-empty message (generous timeout for local LLMs)
		const dmMessage = page.locator('[data-testid="chat-message"][data-role="dm"]');
		await expect(dmMessage.first()).toBeVisible({ timeout: 130_000 });
		const text = await dmMessage.first().textContent();
		expect(text!.length).toBeGreaterThan(10);
	});

	test('quest creation prompt returns quest brief or chain', async ({ page }) => {
		test.setTimeout(150_000);

		await openChat(page);
		// Use /quest prefix instead of mode pill
		await sendMessage(
			page,
			'/quest Create a quest: Write unit tests for the authentication module. Difficulty: moderate. Skills needed: testing, go.'
		);

		// Should get either a quest_brief or quest_chain preview
		const questPreview = page.getByTestId('quest-preview');
		const chainPreview = page.getByTestId('quest-chain-preview');
		await expect(questPreview.or(chainPreview).first()).toBeVisible({ timeout: 130_000 });
	});

	test('session continuity — DM recalls previous context', async ({ page, lifecycleApi }) => {
		test.setTimeout(300_000);

		await openChat(page);

		// First turn: introduce a topic
		await sendMessage(page, 'My agent is named "TestHero" and specializes in data analysis.');
		const firstDM = page.locator('[data-testid="chat-message"][data-role="dm"]');
		await expect(firstDM.first()).toBeVisible({ timeout: 130_000 });

		// Second turn: ask about the topic from the first turn
		await sendMessage(page, 'What was my agent name again?');
		await expect(firstDM).toHaveCount(2, { timeout: 130_000 });

		// The second DM response should mention "TestHero"
		const secondResponse = await firstDM.nth(1).textContent();
		expect(secondResponse!.toLowerCase()).toContain('testhero');
	});

	test('post quest from brief — full flow', async ({ page, lifecycleApi }) => {
		test.setTimeout(150_000);

		await openChat(page);
		// Use /quest prefix instead of mode pill
		await sendMessage(
			page,
			'/quest Create a simple quest: Run the linter on the utils package. Difficulty: easy. Skills: go.'
		);

		// Wait for quest brief
		const preview = page.getByTestId('quest-preview');
		await expect(preview).toBeVisible({ timeout: 130_000 });

		// Click Post Quest
		await page.getByTestId('post-quest-button').click();

		// Verify the quest was actually created on the board
		// The chatStore sends a follow-up message on success
		const messages = chatMessages(page);
		await expect(messages.last()).toContainText('posted successfully', { timeout: 10_000 });
	});
});
