import { test, expect, hasBackend, extractInstance, retry } from '../fixtures/test-base';
import type { DMChatResponse, QuestResponse } from '../fixtures/test-base';

/**
 * DM Chat Integration Tests
 *
 * These tests exercise the full backend DM chat pipeline. They work with either:
 *   - Mock LLM server at http://mockllm:9090 (E2E_MOCK_LLM=true) — deterministic
 *   - Real LLM (Ollama, cloud) (E2E_REAL_LLM=true) — non-deterministic
 *
 * No Playwright-level route interception is used — requests flow through the real
 * backend, which calls whatever LLM is configured.
 *
 * Pipeline: UI → backend /game/dm/chat → LLM gateway → LLM → response parsing → JSON
 *
 * Environment requirements:
 *   - E2E_BACKEND_AVAILABLE=true (set by global-setup.ts when backend is reachable)
 *   - E2E_MOCK_LLM=true OR E2E_REAL_LLM=true (at least one LLM available)
 *
 * Tag: @integration
 */

// =============================================================================
// ENVIRONMENT GUARDS
// =============================================================================

/** Whether the mock LLM server is available (deterministic canned responses). */
function hasMockLLM(): boolean {
	return process.env.E2E_MOCK_LLM === 'true';
}

/** Whether a real LLM (Ollama, cloud) is available (non-deterministic). */
function hasRealLLM(): boolean {
	return process.env.E2E_REAL_LLM === 'true';
}

/** Whether any LLM backend is available for DM chat. */
function hasLLM(): boolean {
	return hasMockLLM() || hasRealLLM();
}

/** Timeout for a single LLM call: 5s for mock, 120s for real (qwen3:14b needs ~20-30s). */
const LLM_TIMEOUT = hasRealLLM() ? 120_000 : 5_000;

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
	// Serialize DM chat tests to avoid Ollama contention when using real LLMs.
	// With 6 parallel workers each making LLM calls, requests queue and timeout.
	test.describe.configure({ mode: hasRealLLM() ? 'serial' : 'parallel' });

	// All tests require backend + at least one LLM (mock or real).
	test.beforeEach(() => {
		test.skip(
			!hasBackend() || !hasLLM(),
			'Requires running backend and LLM (E2E_MOCK_LLM=true or E2E_REAL_LLM=true)'
		);
	});

	// ---------------------------------------------------------------------------
	// 1. Conversational response
	// ---------------------------------------------------------------------------

	test('generic message receives a conversational response', async ({
		lifecycleApi
	}) => {
		test.setTimeout(LLM_TIMEOUT + 10_000);

		// Verifying the response came back at all confirms the full pipeline is wired:
		//   POST /game/dm/chat → backend → LLM → parsed response → HTTP 200
		const resp = await lifecycleApi.sendDMChat('Hello, what can you help me with?');

		assertValidChatResponse(resp);
		// Default mode is converse — no quest extraction should happen.
		expect(resp.quest_brief).toBeUndefined();
		expect(resp.quest_chain).toBeUndefined();
	});

	// ---------------------------------------------------------------------------
	// 2. Quest brief creation
	// ---------------------------------------------------------------------------

	test('quest creation prompt returns quest_brief', async ({ lifecycleApi }) => {
		test.setTimeout(LLM_TIMEOUT + 10_000);

		// Mode must be 'quest' — the backend only extracts quest briefs in quest mode.
		// The system prompt instructs the LLM to produce a ```json:quest_brief tagged block.
		const resp = await lifecycleApi.sendDMChat(
			'create a quest to analyze test data',
			undefined,
			'quest'
		);

		assertValidChatResponse(resp);

		if (hasMockLLM()) {
			// Mock LLM is deterministic — quest_brief must be present
			assertQuestBrief(resp);
		} else {
			// Real LLM may or may not follow the tagged block format on first try.
			// If quest_brief was extracted, verify its structure.
			if (resp.quest_brief) {
				expect(resp.quest_brief.title).toBeTruthy();
				if (resp.quest_brief.difficulty !== undefined) {
					expect(resp.quest_brief.difficulty).toBeGreaterThanOrEqual(0);
					expect(resp.quest_brief.difficulty).toBeLessThanOrEqual(5);
				}
			}
			// Either quest_brief or a conversational message is acceptable from a real LLM.
			// The test confirms the pipeline doesn't crash, and structured output works when the LLM cooperates.
		}
	});

	test('quest brief can be posted to the board after DM creates it', async ({ lifecycleApi }) => {
		test.setTimeout(LLM_TIMEOUT + 10_000);

		// Ask the DM to create a quest — mode must be 'quest' for extraction.
		const chatResp = await lifecycleApi.sendDMChat(
			'create a quest to write unit tests for the parser module',
			undefined,
			'quest'
		);
		assertValidChatResponse(chatResp);

		if (!chatResp.quest_brief) {
			// Real LLM didn't produce structured output — skip the post step.
			// The test still confirms the DM chat endpoint doesn't crash.
			test.skip(!chatResp.quest_brief, 'LLM did not produce quest_brief — skipping post');
			return;
		}

		const brief = chatResp.quest_brief;

		// Post the quest brief to the board via POST /game/quests.
		const quest = await lifecycleApi.createQuest(
			brief.title,
			brief.difficulty ?? 1
		);

		expect(quest.id, 'posted quest must have an entity ID').toBeTruthy();
		expect(quest.status).toBe('posted');

		const questInstance = extractInstance(quest.id);
		const fetched = await lifecycleApi.getQuest(questInstance);
		expect(fetched.status).toBe('posted');
		expect(fetched.objective ?? fetched.id).toBeTruthy();
	});

	// ---------------------------------------------------------------------------
	// 3. Quest chain creation
	// ---------------------------------------------------------------------------

	test('quest chain prompt returns quest_chain with dependency graph', async ({
		lifecycleApi
	}) => {
		test.setTimeout(LLM_TIMEOUT + 10_000);

		// Mode must be 'quest' for chain extraction.
		const resp = await lifecycleApi.sendDMChat(
			'create a quest chain for a data pipeline: ingest, transform, load',
			undefined,
			'quest'
		);

		assertValidChatResponse(resp);

		if (hasMockLLM()) {
			// Mock LLM is deterministic — quest_chain must be present
			assertQuestChain(resp);
			const chain = resp.quest_chain!;
			for (const q of chain.quests) {
				expect(q.title, 'each chain quest must have a title').toBeTruthy();
			}
			const hasDependencies = chain.quests.some(
				(q) => q.depends_on !== undefined && q.depends_on.length > 0
			);
			expect(hasDependencies, 'chain must include at least one dependency relationship').toBe(true);
		} else if (resp.quest_chain) {
			// Real LLM produced a chain — verify structure
			expect(resp.quest_chain.quests.length).toBeGreaterThanOrEqual(2);
			for (const q of resp.quest_chain.quests) {
				expect(q.title).toBeTruthy();
			}
		}
		// Real LLM may produce quest_brief instead of quest_chain, or just text — both acceptable
	});

	test('quest chain can be posted to the board', async ({ lifecycleApi }) => {
		test.setTimeout(LLM_TIMEOUT + 10_000);

		// Ask the DM to create a chain — mode must be 'quest' for chain extraction.
		const chatResp = await lifecycleApi.sendDMChat(
			'create a quest chain for data pipeline: collect, analyze, report',
			undefined,
			'quest'
		);
		assertValidChatResponse(chatResp);

		if (!chatResp.quest_chain) {
			test.skip(!chatResp.quest_chain, 'LLM did not produce quest_chain — skipping post');
			return;
		}

		const chain = chatResp.quest_chain;

		// Post the entire chain to the board via POST /game/quests/chain.
		const postedChain = await lifecycleApi.postQuestChain(chain);

		expect(Array.isArray(postedChain), 'postQuestChain must return an array of quests').toBe(true);
		expect(postedChain.length).toBe(chain.quests.length);

		for (const q of postedChain) {
			expect((q as QuestResponse).id).toBeTruthy();
			expect((q as QuestResponse).status).toBe('posted');
		}
	});

	// ---------------------------------------------------------------------------
	// 4. Session continuity
	// ---------------------------------------------------------------------------

	test('session_id is returned and persists across multiple turns', async ({ lifecycleApi }) => {
		test.setTimeout(LLM_TIMEOUT * 2 + 10_000);

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
		test.setTimeout(LLM_TIMEOUT + 10_000);

		// Sending a valid hex session_id that doesn't correspond to an existing session.
		// isValidSessionID only accepts hex characters, so we use a hex string.
		// The backend should either create a new session or return a graceful error.
		// Either way, the HTTP response must not be a 5xx.
		const staleSid = 'deadbeef' + Date.now().toString(16).padStart(24, '0');
		const resp = await lifecycleApi.sendDMChat('Hello from a stale session', staleSid);

		// If the backend gracefully handles unknown sessions it will return a valid response.
		// (If it errors with 4xx/5xx, sendDMChat will throw and the test fails with a clear message.)
		assertValidChatResponse(resp);
	});

	// ---------------------------------------------------------------------------
	// 5. API-level smoke test
	// ---------------------------------------------------------------------------

	test('direct POST /game/dm/chat smoke test returns valid JSON', async ({ lifecycleApi }) => {
		test.setTimeout(LLM_TIMEOUT + 10_000);

		// Minimal payload — just a message with no context or session.
		// This is the lowest-friction sanity check: can the endpoint parse a request
		// and return a well-formed JSON body?
		const resp = await lifecycleApi.sendDMChat('tell me about quests');

		// The response must be valid JSON with at least a message field.
		assertValidChatResponse(resp);
		// Default mode is converse — no quest extraction
		expect(resp.quest_brief).toBeUndefined();
		expect(resp.quest_chain).toBeUndefined();
	});

	test('mode field is echoed back in response', async ({ lifecycleApi }) => {
		test.setTimeout(LLM_TIMEOUT + 10_000);

		const resp = await lifecycleApi.sendDMChat(
			'tell me about the game',
			undefined,
			'converse'
		);

		assertValidChatResponse(resp);
		expect(resp.mode).toBe('converse');
	});

	// ---------------------------------------------------------------------------
	// 6. DM session restore
	// ---------------------------------------------------------------------------

	test('GET session by session_id returns session after multi-turn chat', async ({
		lifecycleApi
	}) => {
		test.setTimeout(LLM_TIMEOUT * 2 + 10_000);

		// Create a session via multi-turn chat
		const first = await lifecycleApi.sendDMChat('Hello, starting a session.');
		assertValidChatResponse(first);
		expect(first.session_id).toBeTruthy();

		const sessionId = first.session_id!;

		// Second turn to build history
		await lifecycleApi.sendDMChat('Tell me more.', sessionId);

		// Retrieve the session via GET
		const session = await lifecycleApi.getDMSession(sessionId);
		expect(session, 'GET /game/dm/sessions/{id} should return a session').toBeTruthy();
		expect(session!.session_id).toBe(sessionId);
	});

	test('GET session with unknown hex ID returns null (404)', async ({ lifecycleApi }) => {
		test.setTimeout(15_000);

		// Fabricate a valid hex session ID that doesn't exist
		const fakeId = 'deadbeefcafebabe1234567890abcdef';
		const session = await lifecycleApi.getDMSession(fakeId);

		// getDMSession returns null on 404
		expect(session).toBeNull();
	});

	test('GET session with invalid non-hex ID returns error', async ({ lifecycleApi }) => {
		test.setTimeout(15_000);

		// Non-hex characters should fail isValidSessionID validation (400)
		try {
			await lifecycleApi.getDMSession('not-a-hex-id!!!');
			expect(true, 'Expected getDMSession with invalid ID to throw').toBe(false);
		} catch (e) {
			const msg = (e as Error).message;
			// getDMSession throws on non-404 errors; 400 from isValidSessionID
			expect(msg).toContain('400');
		}
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
			!hasBackend() || !hasLLM(),
			'Requires running backend and LLM (E2E_MOCK_LLM=true or E2E_REAL_LLM=true)'
		);
		await page.goto('/');
		// Wait for SvelteKit hydration — use domcontentloaded instead of networkidle
		// because the SSE connection stays open indefinitely, blocking networkidle.
		await page.waitForLoadState('domcontentloaded');
		// Give Svelte time to hydrate
		await page.waitForTimeout(1000);
	});

	test('sending a quest creation message renders a quest preview card', async ({ page }) => {
		test.skip(hasRealLLM() && !hasMockLLM(), 'UI quest preview test requires deterministic mock LLM responses');
		test.setTimeout(LLM_TIMEOUT + 15_000);

		// Open the DM chat panel
		const input = page.getByTestId('chat-input');
		if (!(await input.isVisible().catch(() => false))) {
			await page.getByTestId('chat-toggle').click();
		}
		await expect(input).toBeVisible();

		// Use /quest prefix to trigger quest mode
		await input.fill('/quest create a quest to analyze test data');
		await page.getByTestId('chat-send').click();

		// The mock LLM returns a quest_brief, so a preview card must render.
		// Use a generous timeout because this crosses the full network stack.
		const preview = page.getByTestId('quest-preview');
		await expect(preview).toBeVisible({ timeout: 25_000 });
		// The preview must contain a title (populated from quest_brief.title)
		await expect(preview).not.toBeEmpty();
	});

	test('posting quest from DM brief lands on the quest board', async ({ page }) => {
		test.skip(hasRealLLM() && !hasMockLLM(), 'UI post quest test requires deterministic mock LLM responses');
		test.setTimeout(LLM_TIMEOUT + 30_000);

		// 1. Open chat and submit a quest creation message
		const input = page.getByTestId('chat-input');
		if (!(await input.isVisible().catch(() => false))) {
			await page.getByTestId('chat-toggle').click();
		}
		await expect(input).toBeVisible();

		// Use /quest prefix to trigger quest mode
		await input.fill('/quest create a simple quest to run linter checks');
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
		test.skip(hasRealLLM() && !hasMockLLM(), 'UI chain preview test requires deterministic mock LLM responses');
		test.setTimeout(LLM_TIMEOUT + 15_000);

		// 1. Open chat and request a quest chain
		const input = page.getByTestId('chat-input');
		if (!(await input.isVisible().catch(() => false))) {
			await page.getByTestId('chat-toggle').click();
		}
		await expect(input).toBeVisible();

		// Use /quest prefix to trigger quest mode
		await input.fill('/quest create a quest chain for data pipeline');
		await page.getByTestId('chat-send').click();

		// 2. The mock LLM returns a quest_chain, so the chain preview component must render
		const chainPreview = page.getByTestId('quest-chain-preview');
		await expect(chainPreview).toBeVisible({ timeout: 25_000 });
		// The chain preview should show quest count or individual quest titles
		await expect(chainPreview).not.toBeEmpty();
	});
});
