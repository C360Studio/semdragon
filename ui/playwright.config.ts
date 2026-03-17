import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright E2E Test Configuration
 *
 * Two modes of operation:
 *
 * 1. **Standalone** (no backend): `npm run test:e2e`
 *    - Tests page structure, navigation, layout, accessibility
 *    - Backend-dependent tests auto-skip via global setup detection
 *
 * 2. **Full stack** (with backend): `make e2e` or `SEED_E2E=true docker compose up -d && npm run test:e2e`
 *    - All tests run including real-time SSE, seeded data, API mutations
 *    - Backend must be started with SEED_E2E=true for pre-seeded data
 *
 * Global setup (e2e/global-setup.ts) probes the backend health endpoint
 * and sets E2E_BACKEND_AVAILABLE=true|false. Tests use this to skip gracefully.
 */
export default defineConfig({
	testDir: './e2e',
	globalSetup: './e2e/global-setup.ts',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 1,
	workers: process.env.CI ? 2 : 4,
	reporter: [['html'], ['list']],

	use: {
		baseURL: `http://localhost:${process.env.UI_PORT || '5173'}`,
		trace: 'on-first-retry',
		screenshot: 'only-on-failure',
		video: 'on-first-retry'
	},

	projects: [
		// Tier 0: Cross-browser UI rendering tests.
		// No backend or LLM required — tests page structure, navigation, layout.
		// Excludes all integration specs and scenarios.
		{
			name: 'chromium',
			testIgnore: /-integration|quest-pipeline-e2e|scenarios\//,
			use: { ...devices['Desktop Chrome'] }
		},
		{
			name: 'firefox',
			testIgnore: /-integration|quest-pipeline-e2e|scenarios\//,
			use: { ...devices['Desktop Firefox'] }
		},
		{
			name: 'webkit',
			testIgnore: /-integration|quest-pipeline-e2e|scenarios\//,
			use: { ...devices['Desktop Safari'] }
		},
		// Tier 1: API-driven integration + UI component checks (serial, 1 worker).
		// These tests mutate shared backend state (quests, agentic loops, battles)
		// so they must run sequentially to avoid cross-test contamination.
		{
			name: 'tier1',
			testIgnore: /scenarios\//,
			fullyParallel: false,
			workers: 1,
			use: { ...devices['Desktop Chrome'] }
		},
		// Tier 2: Full user journeys through DM chat UI (serial, 1 worker).
		// No retries — these tests use real LLM tokens and retries create
		// duplicate quests that waste tokens and pollute state.
		{
			name: 'tier2-scenarios',
			testMatch: /scenarios\//,
			testIgnore: /scenarios\/epic/,
			fullyParallel: false,
			retries: 0,
			workers: 1,
			use: { ...devices['Desktop Chrome'] }
		},
		// Tier 3: Epic quest pipeline (serial, 1 worker, real LLM only).
		// Long-running test exercising full party quest DAG pipeline with
		// a complex real-world prompt. Opt-in via task e2e:epic:<provider>.
		{
			name: 'tier3-epic',
			testMatch: /scenarios\/epic/,
			fullyParallel: false,
			retries: 0,
			workers: 1,
			use: { ...devices['Desktop Chrome'] }
		}
	],

	webServer: {
		command: 'npm run dev',
		url: `http://localhost:${process.env.UI_PORT || '5173'}`,
		reuseExistingServer: !process.env.CI,
		timeout: 120000
	},

	expect: {
		timeout: 10000
	},

	timeout: 60000
});
