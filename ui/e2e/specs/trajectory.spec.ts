import { test, expect, hasBackend, waitForHydration, type Page } from '../fixtures/test-base';
import type { Route } from '@playwright/test';

// =============================================================================
// MOCK TRAJECTORY DATA
// =============================================================================

/** A completed agentic loop trajectory with model and tool calls. */
function mockCompletedTrajectory() {
	return {
		loop_id: 'loop-test-abc123',
		start_time: '2026-03-02T10:00:00Z',
		end_time: '2026-03-02T10:00:08Z',
		steps: [
			{
				timestamp: '2026-03-02T10:00:01Z',
				step_type: 'model_call',
				request_id: 'req-1',
				prompt: 'You are an agent working on: Implement authentication module',
				response:
					'I will start by reading the existing auth code to understand the current structure.',
				tokens_in: 250,
				tokens_out: 45,
				duration: 1200
			},
			{
				timestamp: '2026-03-02T10:00:03Z',
				step_type: 'tool_call',
				tool_name: 'read_file',
				tool_arguments: { path: 'src/auth/handler.go', sandbox_dir: '/workspace' },
				tool_result:
					'package auth\n\nfunc HandleLogin(w http.ResponseWriter, r *http.Request) {\n\t// TODO: implement\n}',
				duration: 150
			},
			{
				timestamp: '2026-03-02T10:00:04Z',
				step_type: 'model_call',
				request_id: 'req-2',
				prompt: 'Tool result: package auth...',
				response:
					'Now I will implement the login handler with proper JWT token generation and validation.',
				tokens_in: 380,
				tokens_out: 120,
				duration: 2100
			},
			{
				timestamp: '2026-03-02T10:00:06Z',
				step_type: 'tool_call',
				tool_name: 'write_file',
				tool_arguments: {
					path: 'src/auth/handler.go',
					content: 'package auth\n\nfunc HandleLogin(...) { ... }'
				},
				tool_result: 'File written successfully',
				duration: 80
			},
			{
				timestamp: '2026-03-02T10:00:07Z',
				step_type: 'model_call',
				request_id: 'req-3',
				response: 'Authentication module implemented. The handler now validates credentials and issues JWT tokens.',
				tokens_in: 150,
				tokens_out: 30,
				duration: 900
			}
		],
		outcome: 'complete',
		total_tokens_in: 780,
		total_tokens_out: 195,
		duration: 8000
	};
}

/** A trajectory with no steps (just started). */
function mockEmptyTrajectory() {
	return {
		loop_id: 'loop-empty-xyz',
		start_time: '2026-03-02T11:00:00Z',
		steps: [],
		total_tokens_in: 0,
		total_tokens_out: 0,
		duration: 0
	};
}

/** A failed trajectory. */
function mockFailedTrajectory() {
	return {
		loop_id: 'loop-failed-def',
		start_time: '2026-03-02T12:00:00Z',
		end_time: '2026-03-02T12:00:03Z',
		steps: [
			{
				timestamp: '2026-03-02T12:00:01Z',
				step_type: 'model_call',
				request_id: 'req-fail-1',
				prompt: 'Deploy to production',
				response: 'I will attempt the deployment now.',
				tokens_in: 100,
				tokens_out: 20,
				duration: 800
			}
		],
		outcome: 'failed',
		total_tokens_in: 100,
		total_tokens_out: 20,
		duration: 3000
	};
}

// =============================================================================
// ROUTE INTERCEPTION HELPERS
// =============================================================================

/**
 * Intercept GET /game/trajectories/{id} and respond with mock data.
 * Returns a cleanup function.
 */
async function interceptTrajectory(
	page: Page,
	trajectoryId: string,
	responseBody: Record<string, unknown>
) {
	await page.route(`**/game/trajectories/${trajectoryId}`, async (route: Route) => {
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify(responseBody)
		});
	});
}

/** Intercept trajectory GET and respond with 404. */
async function interceptTrajectoryNotFound(page: Page, trajectoryId: string) {
	await page.route(`**/game/trajectories/${trajectoryId}`, async (route: Route) => {
		await route.fulfill({
			status: 404,
			contentType: 'application/json',
			body: JSON.stringify({ error: 'trajectory not found' })
		});
	});
}

/** Intercept trajectory GET and respond with 500. */
async function interceptTrajectoryError(page: Page, trajectoryId: string) {
	await page.route(`**/game/trajectories/${trajectoryId}`, async (route: Route) => {
		await route.fulfill({
			status: 500,
			contentType: 'application/json',
			body: JSON.stringify({ error: 'internal server error' })
		});
	});
}

// =============================================================================
// TRAJECTORY LIST PAGE
// =============================================================================

test.describe('Trajectory List Page', () => {
	test('renders page heading and description', async ({ page }) => {
		await page.goto('/trajectories');
		await waitForHydration(page);

		await expect(page.getByTestId('trajectories-heading')).toHaveText('Trajectory Explorer');
		await expect(page.getByTestId('trajectories-page')).toContainText(
			'Browse the full event timeline'
		);
	});

	test('trajectory list is present on the page', async ({ page }) => {
		await page.goto('/trajectories');
		await waitForHydration(page);

		// The list container should always be rendered
		await expect(page.getByTestId('trajectory-list')).toBeVisible();
	});
});

// Tests that require seeded quests with trajectory_ids
test.describe('Trajectory List - With Backend', () => {
	test.beforeEach(async () => {
		test.skip(!hasBackend(), 'Requires running backend with seeded data');
	});

	test('displays trajectory items from seeded quests', async ({ page, lifecycleApi }) => {
		// Create a quest — it should get a trajectory_id
		const quest = await lifecycleApi.createQuest('Trajectory list test quest');
		expect(quest.id).toBeTruthy();

		await page.goto('/trajectories');
		await waitForHydration(page);

		// Wait for SSE to hydrate worldStore — trajectory items may appear
		// If quests have trajectory_ids, items should show up
		const list = page.getByTestId('trajectory-list');
		await expect(list).toBeVisible();
	});
});

// =============================================================================
// TRAJECTORY DETAIL PAGE - MOCK TESTS
// =============================================================================

test.describe('Trajectory Detail - Completed', () => {
	const trajectoryId = 'loop-test-abc123';

	test.beforeEach(async ({ page }) => {
		await interceptTrajectory(page, trajectoryId, mockCompletedTrajectory());
		await page.goto(`/trajectories/${trajectoryId}`);
		await waitForHydration(page);
	});

	test('renders page heading and trajectory ID', async ({ page }) => {
		await expect(page.getByTestId('trajectory-heading')).toHaveText('Trajectory Timeline');
		await expect(page.getByTestId('trajectory-id')).toHaveText(trajectoryId);
	});

	test('shows back link to trajectory list', async ({ page }) => {
		const backLink = page.getByTestId('trajectory-back-link');
		await expect(backLink).toBeVisible();
		await expect(backLink).toHaveText('Back to Trajectories');
		await expect(backLink).toHaveAttribute('href', '/trajectories');
	});

	test('displays trajectory summary with outcome, tokens, and duration', async ({ page }) => {
		const summary = page.getByTestId('trajectory-summary');
		await expect(summary).toBeVisible();

		await expect(page.getByTestId('trajectory-outcome')).toContainText('complete');
		await expect(page.getByTestId('trajectory-tokens')).toContainText('780');
		await expect(page.getByTestId('trajectory-tokens')).toContainText('195');
		await expect(page.getByTestId('trajectory-duration')).toContainText('8.0s');
	});

	test('renders timeline with correct number of events', async ({ page }) => {
		const timeline = page.getByTestId('trajectory-timeline');
		await expect(timeline).toBeVisible();

		const events = page.getByTestId('timeline-event');
		await expect(events).toHaveCount(5);
	});

	test('model_call events show type and token counts', async ({ page }) => {
		const modelEvents = page.locator('[data-testid="timeline-event"][data-step-type="model_call"]');
		await expect(modelEvents).toHaveCount(3);

		// First model call
		const first = modelEvents.first();
		await expect(first.getByTestId('event-type')).toHaveText('model_call');
		await expect(first.getByTestId('event-tokens')).toContainText('250/45');
		await expect(first.getByTestId('event-duration')).toContainText('1.2s');
	});

	test('tool_call events show tool name and arguments', async ({ page }) => {
		const toolEvents = page.locator('[data-testid="timeline-event"][data-step-type="tool_call"]');
		await expect(toolEvents).toHaveCount(2);

		// First tool call — read_file
		const readFile = toolEvents.first();
		await expect(readFile.getByTestId('event-type')).toHaveText('read_file');

		// Click to expand, then check tool arguments
		await readFile.locator('.event-content').click();
		await expect(readFile.getByTestId('event-tool-args')).toContainText('src/auth/handler.go');
	});

	test('tool_call events display duration', async ({ page }) => {
		const toolEvents = page.locator('[data-testid="timeline-event"][data-step-type="tool_call"]');

		// read_file: 150ms
		await expect(toolEvents.first().getByTestId('event-duration')).toContainText('150ms');

		// write_file: 80ms
		await expect(toolEvents.nth(1).getByTestId('event-duration')).toContainText('80ms');
	});

	test('model_call events show response when expanded', async ({ page }) => {
		const modelEvents = page.locator('[data-testid="timeline-event"][data-step-type="model_call"]');

		// Click to expand the first model call, then check response content
		const first = modelEvents.first();
		await first.locator('.event-content').click();
		await expect(first.getByTestId('event-response')).toContainText('reading the existing auth code');
	});

	test('events are in chronological order', async ({ page }) => {
		const events = page.getByTestId('timeline-event');
		// Wait for all 5 events to render before reading attributes
		await expect(events).toHaveCount(5);

		// Verify alternating pattern: model, tool, model, tool, model
		const types: string[] = [];
		for (let i = 0; i < 5; i++) {
			const type = await events.nth(i).getAttribute('data-step-type');
			types.push(type!);
		}
		expect(types).toEqual(['model_call', 'tool_call', 'model_call', 'tool_call', 'model_call']);
	});
});

// =============================================================================
// TRAJECTORY DETAIL - EMPTY STEPS
// =============================================================================

test.describe('Trajectory Detail - Empty Steps', () => {
	const trajectoryId = 'loop-empty-xyz';

	test('shows empty steps message when trajectory has no steps', async ({ page }) => {
		await interceptTrajectory(page, trajectoryId, mockEmptyTrajectory());
		await page.goto(`/trajectories/${trajectoryId}`);
		await waitForHydration(page);

		const emptySteps = page.getByTestId('trajectory-empty-steps');
		await expect(emptySteps).toBeVisible();
		await expect(emptySteps).toContainText('No steps recorded yet');
		await expect(emptySteps).toContainText('Steps will appear here as the quest progresses');

		// No summary for zero-token trajectory
		await expect(page.getByTestId('trajectory-summary')).not.toBeVisible();
		// No timeline
		await expect(page.getByTestId('trajectory-timeline')).not.toBeVisible();
	});
});

// =============================================================================
// TRAJECTORY DETAIL - FAILED
// =============================================================================

test.describe('Trajectory Detail - Failed', () => {
	const trajectoryId = 'loop-failed-def';

	test('displays failed outcome in summary', async ({ page }) => {
		await interceptTrajectory(page, trajectoryId, mockFailedTrajectory());
		await page.goto(`/trajectories/${trajectoryId}`);
		await waitForHydration(page);

		await expect(page.getByTestId('trajectory-outcome')).toContainText('failed');
		await expect(page.getByTestId('trajectory-tokens')).toContainText('100');
		await expect(page.getByTestId('trajectory-duration')).toContainText('3.0s');

		// Only 1 step
		await expect(page.getByTestId('timeline-event')).toHaveCount(1);
	});
});

// =============================================================================
// TRAJECTORY DETAIL - ERROR STATES
// =============================================================================

test.describe('Trajectory Detail - Error Handling', () => {
	test('shows error message when API returns 500', async ({ page }) => {
		const trajectoryId = 'loop-server-error';
		await interceptTrajectoryError(page, trajectoryId);
		await page.goto(`/trajectories/${trajectoryId}`);
		await waitForHydration(page);

		const error = page.getByTestId('trajectory-error');
		await expect(error).toBeVisible();
		await expect(error).toContainText('Failed to load trajectory');
	});

	test('shows error message when API returns 404', async ({ page }) => {
		const trajectoryId = 'loop-not-found';
		await interceptTrajectoryNotFound(page, trajectoryId);
		await page.goto(`/trajectories/${trajectoryId}`);
		await waitForHydration(page);

		// 404 is thrown as ApiError, caught by the component's error handler
		const error = page.getByTestId('trajectory-error');
		await expect(error).toBeVisible();
		await expect(error).toContainText('Failed to load trajectory');
	});

	test('shows loading state before data arrives', async ({ page }) => {
		const trajectoryId = 'loop-slow';
		// Delay the response to observe loading state
		await page.route(`**/game/trajectories/${trajectoryId}`, async (route: Route) => {
			await new Promise((r) => setTimeout(r, 1000));
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify(mockCompletedTrajectory())
			});
		});

		await page.goto(`/trajectories/${trajectoryId}`);

		// Loading should appear before data arrives
		const loading = page.getByTestId('trajectory-loading');
		await expect(loading).toBeVisible();
		await expect(loading).toContainText('Loading trajectory');

		// After data arrives, loading disappears and timeline appears
		await expect(loading).not.toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('trajectory-timeline')).toBeVisible();
	});
});

// =============================================================================
// TRAJECTORY DETAIL - NAVIGATION
// =============================================================================

test.describe('Trajectory Detail - Navigation', () => {
	test('back link navigates to trajectory list', async ({ page }) => {
		const trajectoryId = 'loop-nav-test';
		await interceptTrajectory(page, trajectoryId, mockCompletedTrajectory());
		await page.goto(`/trajectories/${trajectoryId}`);
		await waitForHydration(page);

		await page.getByTestId('trajectory-back-link').click();
		await expect(page).toHaveURL(/\/trajectories$/);
		await expect(page.getByTestId('trajectories-heading')).toBeVisible();
	});
});
