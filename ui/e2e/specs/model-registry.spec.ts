import { test, expect, hasBackend, isRealLLM } from '../fixtures/test-base';

/**
 * Model Registry E2E Tests
 *
 * Verifies tier-qualified capability routing via the GET /game/models endpoint.
 * These are API-level tests — no browser UI needed, just HTTP calls.
 *
 * The tiered config maps different trust tiers to different Gemini models:
 *   - apprentice  → gemini-2.0-flash-lite
 *   - journeyman  → gemini-2.5-flash
 *   - expert      → gemini-2.5-pro
 *   - master      → gemini-2.5-pro
 *
 * Environment requirements:
 *   - E2E_BACKEND_AVAILABLE=true (set by global-setup.ts)
 *   - E2E_LLM_MODE=gemini|openai|anthropic (real LLM with tiered config)
 */

// Response shapes for the /game/models endpoint
interface ModelResolveResponse {
	capability: string;
	endpoint_name: string;
	model?: string;
	provider?: string;
	fallback_chain?: string[];
}

interface ModelEndpointSummary {
	name: string;
	provider: string;
	model: string;
	max_tokens: number;
	supports_tools: boolean;
	reasoning_effort?: string;
}

interface ModelRegistrySummary {
	endpoints: ModelEndpointSummary[];
	capabilities: string[];
}

const API_URL = process.env.API_URL || `http://localhost:${process.env.BACKEND_PORT || '8081'}`;

test.describe('Model Registry @integration', () => {
	test.beforeEach(() => {
		test.skip(!hasBackend(), 'Requires running backend (E2E_BACKEND_AVAILABLE=true)');
	});

	// ─── Registry Summary ────────────────────────────────────────────

	test('GET /game/models returns endpoints and capabilities', async ({ playwright }) => {
		const api = await playwright.request.newContext({ baseURL: API_URL });
		try {
			const res = await api.get('/game/models');
			expect(res.ok()).toBe(true);

			const body: ModelRegistrySummary = await res.json();
			expect(body.endpoints).toBeInstanceOf(Array);
			expect(body.endpoints.length).toBeGreaterThan(0);
			expect(body.capabilities).toBeInstanceOf(Array);
			expect(body.capabilities.length).toBeGreaterThan(0);

			// Every endpoint must have required fields
			for (const ep of body.endpoints) {
				expect(ep.name).toBeTruthy();
				expect(ep.provider).toBeTruthy();
				expect(ep.model).toBeTruthy();
			}
		} finally {
			await api.dispose();
		}
	});

	// ─── Capability Resolution ───────────────────────────────────────

	test('resolves a known capability', async ({ playwright }) => {
		const api = await playwright.request.newContext({ baseURL: API_URL });
		try {
			const res = await api.get('/game/models?resolve=agent-work');
			expect(res.ok()).toBe(true);

			const body: ModelResolveResponse = await res.json();
			expect(body.capability).toBe('agent-work');
			expect(body.endpoint_name).toBeTruthy();
			expect(body.model).toBeTruthy();
			expect(body.provider).toBeTruthy();
		} finally {
			await api.dispose();
		}
	});

	test('resolves unknown capability with fallback', async ({ playwright }) => {
		const api = await playwright.request.newContext({ baseURL: API_URL });
		try {
			// Request a capability that doesn't exist — should fall back to default
			const res = await api.get('/game/models?resolve=nonexistent-capability');
			expect(res.ok()).toBe(true);

			const body: ModelResolveResponse = await res.json();
			expect(body.capability).toBe('nonexistent-capability');
			// Should still resolve to something (the default endpoint)
			expect(body.endpoint_name).toBeTruthy();
		} finally {
			await api.dispose();
		}
	});

	// ─── Tiered Routing (requires tiered config) ─────────────────────

	test.describe('Tiered Gemini Routing', () => {
		test.beforeEach(() => {
			test.skip(
				!isRealLLM(),
				'Tiered routing tests require real LLM (E2E_LLM_MODE=gemini|openai|anthropic|ollama)'
			);
		});

		test('resolves apprentice tier to lite model', async ({ playwright }) => {
			const api = await playwright.request.newContext({ baseURL: API_URL });
			try {
				const res = await api.get('/game/models?resolve=agent-work.apprentice');
				expect(res.ok()).toBe(true);

				const body: ModelResolveResponse = await res.json();
				expect(body.capability).toBe('agent-work.apprentice');
				expect(body.model).toBe('gemini-2.5-flash-lite');
				expect(body.endpoint_name).toBe('gemini-lite');
			} finally {
				await api.dispose();
			}
		});

		test('resolves journeyman tier to flash model', async ({ playwright }) => {
			const api = await playwright.request.newContext({ baseURL: API_URL });
			try {
				const res = await api.get('/game/models?resolve=agent-work.journeyman');
				expect(res.ok()).toBe(true);

				const body: ModelResolveResponse = await res.json();
				expect(body.capability).toBe('agent-work.journeyman');
				expect(body.model).toBe('gemini-2.5-flash');
				expect(body.endpoint_name).toBe('gemini-flash');
			} finally {
				await api.dispose();
			}
		});

		test('resolves expert tier to pro model', async ({ playwright }) => {
			const api = await playwright.request.newContext({ baseURL: API_URL });
			try {
				const res = await api.get('/game/models?resolve=agent-work.expert');
				expect(res.ok()).toBe(true);

				const body: ModelResolveResponse = await res.json();
				expect(body.capability).toBe('agent-work.expert');
				expect(body.model).toBe('gemini-2.5-pro');
				expect(body.endpoint_name).toBe('gemini-pro');
			} finally {
				await api.dispose();
			}
		});

		test('resolves master tier to pro model', async ({ playwright }) => {
			const api = await playwright.request.newContext({ baseURL: API_URL });
			try {
				const res = await api.get('/game/models?resolve=agent-work.master');
				expect(res.ok()).toBe(true);

				const body: ModelResolveResponse = await res.json();
				expect(body.capability).toBe('agent-work.master');
				expect(body.model).toBe('gemini-2.5-pro');
			} finally {
				await api.dispose();
			}
		});

		test('resolves grandmaster tier to pro model', async ({ playwright }) => {
			const api = await playwright.request.newContext({ baseURL: API_URL });
			try {
				const res = await api.get('/game/models?resolve=agent-work.grandmaster');
				expect(res.ok()).toBe(true);

				const body: ModelResolveResponse = await res.json();
				expect(body.capability).toBe('agent-work.grandmaster');
				expect(body.model).toBe('gemini-2.5-pro');
				expect(body.endpoint_name).toBe('gemini-pro');
			} finally {
				await api.dispose();
			}
		});

		test('lists all configured tier capabilities', async ({ playwright }) => {
			const api = await playwright.request.newContext({ baseURL: API_URL });
			try {
				const res = await api.get('/game/models');
				expect(res.ok()).toBe(true);

				const body: ModelRegistrySummary = await res.json();
				expect(body.capabilities).toContain('agent-work.apprentice');
				expect(body.capabilities).toContain('agent-work.journeyman');
				expect(body.capabilities).toContain('agent-work.expert');
				expect(body.capabilities).toContain('agent-work.master');
				expect(body.capabilities).toContain('agent-work.grandmaster');
				expect(body.capabilities).toContain('agent-work');
				expect(body.capabilities).toContain('boss-battle');
				expect(body.capabilities).toContain('dm-chat');
			} finally {
				await api.dispose();
			}
		});

		test('lists all tiered endpoints', async ({ playwright }) => {
			const api = await playwright.request.newContext({ baseURL: API_URL });
			try {
				const res = await api.get('/game/models');
				expect(res.ok()).toBe(true);

				const body: ModelRegistrySummary = await res.json();
				const epNames = body.endpoints.map((ep) => ep.name);
				expect(epNames).toContain('gemini-lite');
				expect(epNames).toContain('gemini-flash');
				expect(epNames).toContain('gemini-pro');
			} finally {
				await api.dispose();
			}
		});

		test('endpoints include reasoning_effort when configured', async ({ playwright }) => {
			const api = await playwright.request.newContext({ baseURL: API_URL });
			try {
				const res = await api.get('/game/models');
				expect(res.ok()).toBe(true);

				const body: ModelRegistrySummary = await res.json();

				// gemini-flash should have reasoning_effort: "low"
				const flash = body.endpoints.find((ep) => ep.name === 'gemini-flash');
				expect(flash).toBeDefined();
				expect(flash!.reasoning_effort).toBe('low');

				// gemini-pro should have reasoning_effort: "medium"
				const pro = body.endpoints.find((ep) => ep.name === 'gemini-pro');
				expect(pro).toBeDefined();
				expect(pro!.reasoning_effort).toBe('medium');

				// gemini-lite should NOT have reasoning_effort
				const lite = body.endpoints.find((ep) => ep.name === 'gemini-lite');
				expect(lite).toBeDefined();
				expect(lite!.reasoning_effort).toBeUndefined();
			} finally {
				await api.dispose();
			}
		});
	});
});
