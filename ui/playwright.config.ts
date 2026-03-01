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
	retries: process.env.CI ? 2 : 0,
	workers: process.env.CI ? 1 : undefined,
	reporter: [['html'], ['list']],

	use: {
		baseURL: 'http://localhost:5173',
		trace: 'on-first-retry',
		screenshot: 'only-on-failure',
		video: 'on-first-retry'
	},

	projects: [
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'] }
		},
		{
			name: 'firefox',
			use: { ...devices['Desktop Firefox'] }
		},
		{
			name: 'webkit',
			use: { ...devices['Desktop Safari'] }
		}
	],

	webServer: {
		command: 'npm run dev',
		url: 'http://localhost:5173',
		reuseExistingServer: !process.env.CI,
		timeout: 120000
	},

	expect: {
		timeout: 10000
	},

	timeout: 60000
});
