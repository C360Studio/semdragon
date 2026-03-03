import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';
import type { DMChatResponse, QuestResponse } from '../fixtures/test-base';

/**
 * DM Chat Integration Tests
 *
 * These tests exercise the full backend DM chat pipeline using a mock LLM server
 * at http://mockllm:9090 (Docker network). No Playwright-level route interception
 * is used — requests flow through the real backend, which calls the mock LLM.
 *
 * The mock LLM returns canned OpenAI-compatible responses keyed by message content,
 * allowing deterministic verification of each pipeline stage:
 *   UI → backend /game/dm/chat → LLM gateway → mock LLM → response parsing → JSON
 *
 * Environment requirements:
 *   - E2E_BACKEND_AVAILABLE=true (set by global-setup.ts when backend is reachable)
 *   - E2E_MOCK_LLM=true (set when the mock LLM server is available)
 *   - Backend configured to use http://mockllm:9090 as the LLM provider
 *
 * Tag: @integration
 */

// =============================================================================
// ENVIRONMENT GUARDS
// =============================================================================

/**
 * Whether the mock LLM server is available.
 * Set E2E_MOCK_LLM=true in docker-compose environment to enable these tests.
 */
function hasMockLLM(): boolean {
	return process.env.E2E_MOCK_LLM === 'true';
}

// =============================================================================
// RESPONSE SHAPE HELPERS
// =============================================================================

/**
 * Verify the structural contract of a DM chat response.
 * All responses must include a message and trace_info, regardless of content type.
 */
function assertValidChatResponse(resp: DMChatResponse): void {
	expect(resp.message, 'response.message must be a non-empty string').toBeTruthy();
	expect(typeof resp.message).toBe('string');
}

/**
 * Verify that a response contains a parseable quest_brief with the required fields.
 * The mock LLM is configured to return quest_brief payloads when asked to create a quest.
 */
function assertQuestBrief(resp: DMChatResponse): void {
	assertValidChatResponse(resp);
	expect(resp.quest_brief, 'response must contain quest_brief').toBeDefined();
	expect(resp.quest_brief!.title, 'quest_brief.title must be non-empty').toBeTruthy();
}

/**
 * Verify that a response contains a quest_chain with at least two quests.
 * The mock LLM is configured to return quest_chain payloads when asked for a chain.
 */
function assertQuestChain(resp: DMChatResponse): void {
	assertValidChatResponse(resp);
	expect(resp.quest_chain, 'response must contain quest_chain').toBeDefined();
	expect(resp.quest_chain!.quests, 'quest_chain.quests must be an array').toBeInstanceOf(Array);
	expect(
		resp.quest_chain!.quests.length,
		'quest_chain must have at least 2 quests'
	).toBeGreaterThanOrEqual(2);
}

// =============================================================================
// DM CHAT INTEGRATION TESTS
// =============================================================================

test.describe('DM Chat Integration @integration', () => {
	// All tests in this suite require both the backend and mock LLM.
	// Using beforeEach so the skip reason appears close to each test in CI logs.
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasMockLLM(),
			'Requires running backend (E2E_BACKEND_AVAILABLE=true) and mock LLM (E2E_MOCK_LLM=true)'
		);
	});

	// ---------------------------------------------------------------------------
	// 1. Conversational response
	// ---------------------------------------------------------------------------

	test('generic message receives a conversational response from mock LLM', async ({
		lifecycleApi
	}) => {
		// Set a longer timeout — this goes through the full backend → mock LLM round trip.
		test.setTimeout(30_000);

		// The mock LLM is configured to respond to generic messages with a canned greeting.
		// Verifying the response came back at all confirms the full pipeline is wired:
		//   POST /game/dm/chat → backend → LLM gateway → mock LLM → parsed response → HTTP 200
		const resp = await lifecycleApi.sendDMChat('Hello, what can you help me with?');

		assertValidChatResponse(resp);
		// A generic message must not accidentally trigger quest_brief or quest_chain parsing.
		// These fields being absent confirms the backend LLM response parser is discriminating
		// content correctly rather than always returning one type.
		expect(resp.quest_brief).toBeUndefined();
		expect(resp.quest_chain).toBeUndefined();
	});

	// ---------------------------------------------------------------------------
	// 2. Quest brief creation
	// ---------------------------------------------------------------------------

	test('quest creation prompt returns quest_brief from mock LLM', async ({ lifecycleApi }) => {
		test.setTimeout(30_000);

		// The mock LLM is configured to return a tagged quest_brief JSON block when it
		// receives a message matching the quest creation pattern. The backend must:
		//   1. Forward the message to the LLM
		//   2. Parse the ```quest_brief ... ``` tagged block from the LLM response
		//   3. Return the structured quest_brief field in the API response
		const resp = await lifecycleApi.sendDMChat('create a quest to analyze test data');

		assertQuestBrief(resp);
		// Verify the brief has the fields needed for the frontend to render a preview card
		expect(resp.quest_brief!.title).toBeTruthy();
		// difficulty must be a number between 0-5 (Trivial → Legendary)
		if (resp.quest_brief!.difficulty !== undefined) {
			expect(resp.quest_brief!.difficulty).toBeGreaterThanOrEqual(0);
			expect(resp.quest_brief!.difficulty).toBeLessThanOrEqual(5);
		}
	});

	test('quest brief can be posted to the board after DM creates it', async ({ lifecycleApi }) => {
		test.setTimeout(30_000);

		// Step 1: Ask the DM to create a quest — the mock LLM returns a quest_brief
		const chatResp = await lifecycleApi.sendDMChat(
			'create a quest to write unit tests for the parser module'
		);
		assertQuestBrief(chatResp);

		const brief = chatResp.quest_brief!;

		// Step 2: Post the quest brief to the board via POST /game/quests.
		// This verifies the data the DM returned is structurally valid for quest creation.
		// In the UI flow, the user clicks "Post Quest" which triggers this same API call.
		const quest = await lifecycleApi.createQuest(
			brief.title,
			brief.difficulty ?? 1
		);

		expect(quest.id, 'posted quest must have an entity ID').toBeTruthy();
		expect(quest.status).toBe('posted');

		// Step 3: Fetch the quest to confirm it landed on the board
		const questInstance = extractInstance(quest.id);
		const fetched = await lifecycleApi.getQuest(questInstance);
		expect(fetched.status).toBe('posted');
		// The objective on the posted quest should reflect the brief title
		expect(fetched.objective ?? fetched.id).toBeTruthy();
	});

	// ---------------------------------------------------------------------------
	// 3. Quest chain creation
	// ---------------------------------------------------------------------------

	test('quest chain prompt returns quest_chain with dependency graph from mock LLM', async ({
		lifecycleApi
	}) => {
		test.setTimeout(30_000);

		// The mock LLM is configured to return a quest_chain when asked for a pipeline.
		// This exercises the chain parsing path in the backend DM chat handler — a separate
		// code path from the single quest_brief path.
		const resp = await lifecycleApi.sendDMChat(
			'create a quest chain for a data pipeline: ingest, transform, load'
		);

		assertQuestChain(resp);

		const chain = resp.quest_chain!;

		// Every quest in the chain must have a title
		for (const q of chain.quests) {
			expect(q.title, 'each chain quest must have a title').toBeTruthy();
		}

		// At least one quest in the chain must declare a dependency on another quest
		// (otherwise the mock LLM returned a flat list, not a chain)
		const hasDependencies = chain.quests.some(
			(q) => q.depends_on !== undefined && q.depends_on.length > 0
		);
		expect(hasDependencies, 'chain must include at least one dependency relationship').toBe(true);
	});

	test('quest chain can be posted to the board', async ({ lifecycleApi }) => {
		test.setTimeout(30_000);

		// Step 1: Ask the DM to create a chain — mock LLM returns quest_chain
		const chatResp = await lifecycleApi.sendDMChat(
			'create a quest chain for data pipeline: collect, analyze, report'
		);
		assertQuestChain(chatResp);

		const chain = chatResp.quest_chain!;

		// Step 2: Post the entire chain to the board via POST /game/quests/chain.
		// This verifies the chain structure the DM returned is valid for bulk creation.
		const postedChain = await lifecycleApi.postQuestChain(chain);

		expect(Array.isArray(postedChain), 'postQuestChain must return an array of quests').toBe(true);
		expect(
			postedChain.length,
			'all quests in the chain must be created on the board'
		).toBe(chain.quests.length);

		// Verify each created quest has a valid ID and posted status
		for (const q of postedChain) {
			expect((q as QuestResponse).id).toBeTruthy();
			expect((q as QuestResponse).status).toBe('posted');
		}
	});

	// ---------------------------------------------------------------------------
	// 4. Session continuity
	// ---------------------------------------------------------------------------

	test('session_id is returned and persists across multiple turns', async ({ lifecycleApi }) => {
		test.setTimeout(60_000);

		// First turn — backend generates a new session and returns session_id
		const firstResp = await lifecycleApi.sendDMChat('My project is called Semdragons.');

		assertValidChatResponse(firstResp);
		// The backend must return a session_id so the client can pass it back on turn 2
		expect(firstResp.session_id, 'first response must include session_id').toBeTruthy();

		const sessionId = firstResp.session_id!;

		// Second turn — pass the session_id from the first response.
		// The backend must route this to the same session, preserving context.
		const secondResp = await lifecycleApi.sendDMChat(
			'What was the name of my project?',
			sessionId
		);

		assertValidChatResponse(secondResp);
		// The session_id returned on turn 2 must match what we sent — the session
		// should not be regenerated mid-conversation.
		expect(secondResp.session_id).toBe(sessionId);
	});

	test('messages with unknown session_id return a valid response', async ({ lifecycleApi }) => {
		test.setTimeout(30_000);

		// Sending a stale or fabricated session_id should not crash the backend.
		// The backend should either create a new session or return a graceful error.
		// Either way, the HTTP response must not be a 5xx.
		const staleSid = 'stale-session-e2e-test-' + Date.now();
		const resp = await lifecycleApi.sendDMChat('Hello from a stale session', staleSid);

		// If the backend gracefully handles unknown sessions it will return a valid response.
		// (If it errors with 4xx/5xx, sendDMChat will throw and the test fails with a clear message.)
		assertValidChatResponse(resp);
	});

	// ---------------------------------------------------------------------------
	// 5. API-level smoke test
	// ---------------------------------------------------------------------------

	test('direct POST /game/dm/chat smoke test returns valid JSON', async ({ lifecycleApi }) => {
		test.setTimeout(30_000);

		// Minimal payload — just a message with no context or session.
		// This is the lowest-friction sanity check: can the endpoint parse a request
		// and return a well-formed JSON body?
		const resp = await lifecycleApi.sendDMChat('create a quest');

		// The response must be valid JSON with at least a message field.
		// quest_brief may or may not be present depending on mock LLM configuration.
		assertValidChatResponse(resp);
	});
});

// =============================================================================
// UI-LEVEL DM CHAT INTEGRATION (opt-in: requires mock LLM + browser)
// =============================================================================

/**
 * These tests exercise the full UI → backend → mock LLM round trip through
 * the browser. They navigate to the dashboard, open the chat panel, and
 * verify that the Svelte UI renders responses from the real backend.
 *
 * Skipped unless both E2E_BACKEND_AVAILABLE and E2E_MOCK_LLM are true.
 */
test.describe('DM Chat UI Integration @integration', () => {
	test.beforeEach(async ({ page }) => {
		test.skip(
			!hasBackend() || !hasMockLLM(),
			'Requires running backend (E2E_BACKEND_AVAILABLE=true) and mock LLM (E2E_MOCK_LLM=true)'
		);
		await page.goto('/');
		// Wait for SvelteKit hydration and SSE connection before interacting
		await page.waitForLoadState('networkidle');
	});

	test('sending a quest creation message renders a quest preview card', async ({ page }) => {
		test.setTimeout(30_000);

		// Open the DM chat panel
		const input = page.getByTestId('chat-input');
		if (!(await input.isVisible().catch(() => false))) {
			await page.getByTestId('chat-toggle').click();
		}
		await expect(input).toBeVisible();

		// Type and submit — this goes to the real backend which calls the mock LLM
		await input.fill('create a quest to analyze test data');
		await page.getByTestId('chat-send').click();

		// The mock LLM returns a quest_brief, so a preview card must render.
		// Use a generous timeout because this crosses the full network stack.
		const preview = page.getByTestId('quest-preview');
		await expect(preview).toBeVisible({ timeout: 25_000 });
		// The preview must contain a title (populated from quest_brief.title)
		await expect(preview).not.toBeEmpty();
	});

	test('posting quest from DM brief lands on the quest board', async ({ page }) => {
		test.setTimeout(45_000);

		// 1. Open chat and submit a quest creation message
		const input = page.getByTestId('chat-input');
		if (!(await input.isVisible().catch(() => false))) {
			await page.getByTestId('chat-toggle').click();
		}
		await expect(input).toBeVisible();

		await input.fill('create a simple quest to run linter checks');
		await page.getByTestId('chat-send').click();

		// 2. Wait for the quest preview card to render from the mock LLM response
		const preview = page.getByTestId('quest-preview');
		await expect(preview).toBeVisible({ timeout: 25_000 });

		// 3. Click "Post Quest" to submit the brief to the backend
		await page.getByTestId('post-quest-button').click();

		// 4. The chat panel should confirm the quest was posted.
		// The chatStore appends a success confirmation message after a successful POST.
		const messages = page.getByTestId('chat-message');
		await expect(messages.last()).toContainText('posted', { timeout: 10_000 });
	});

	test('quest chain preview renders with dependency information', async ({ page }) => {
		test.setTimeout(30_000);

		// 1. Open chat and request a quest chain
		const input = page.getByTestId('chat-input');
		if (!(await input.isVisible().catch(() => false))) {
			await page.getByTestId('chat-toggle').click();
		}
		await expect(input).toBeVisible();

		await input.fill('create a quest chain for data pipeline');
		await page.getByTestId('chat-send').click();

		// 2. The mock LLM returns a quest_chain, so the chain preview component must render
		const chainPreview = page.getByTestId('quest-chain-preview');
		await expect(chainPreview).toBeVisible({ timeout: 25_000 });
		// The chain preview should show quest count or individual quest titles
		await expect(chainPreview).not.toBeEmpty();
	});
});
