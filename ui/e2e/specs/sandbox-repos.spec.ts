import { test, expect, hasBackend, extractInstance } from '../fixtures/test-base';

/**
 * Sandbox Repos — Tier 1 Integration Tests
 *
 * Tests the sandbox git plumbing integration with the backend API.
 * Verifies that:
 *   1. Backend reports sandbox as available in settings/health
 *   2. Quest repo field round-trips through create → get
 *   3. Artifact list endpoint handles missing workspaces gracefully
 *
 * Full worktree/merge-to-main integration requires REPOS_DIR with real
 * repos mounted. Those paths are exercised in T2 scenarios with real LLM.
 *
 * @integration @tier1
 */

test.describe('Sandbox Repos - Backend Integration', () => {
	test.beforeEach(() => {
		test.skip(!hasBackend(), 'Requires running backend');
	});

	test('settings report sandbox available', async ({ apiRequest }) => {
		const res = await apiRequest.get('/game/settings');
		expect(res.ok()).toBeTruthy();

		const settings = await res.json();
		// The workspace field reflects sandbox availability (SANDBOX_URL set in Docker).
		expect(settings.workspace).toBeDefined();
		expect(settings.workspace.available).toBe(true);
	});

	test('health check includes artifact store OK', async ({ apiRequest }) => {
		const res = await apiRequest.get('/game/settings/health');
		expect(res.ok()).toBeTruthy();

		const health = await res.json();
		const artifactCheck = health.checks?.find(
			(c: { name: string }) => c.name === 'artifact_store'
		);

		expect(artifactCheck).toBeDefined();
		expect(artifactCheck.status).toBe('ok');
	});
});

test.describe('Sandbox Repos - Quest Repo Field', () => {
	test.beforeEach(() => {
		test.skip(!hasBackend(), 'Requires running backend');
	});

	test('quest created with repo hint persists the repo field', async ({ apiRequest }) => {
		const createRes = await apiRequest.post('/game/quests', {
			data: {
				objective: 'E2E sandbox repo field round-trip test',
				hints: {
					suggested_difficulty: 1,
					suggested_skills: [],
					require_human_review: false,
					budget: 100,
					repo: 'testrepo'
				}
			}
		});
		expect(createRes.ok(), `create failed: ${createRes.status()}`).toBeTruthy();

		const quest = await createRes.json();
		const instanceId = extractInstance(quest.id);

		// Fetch back via GET and verify repo field round-trips.
		const getRes = await apiRequest.get(`/game/quests/${instanceId}`);
		expect(getRes.ok()).toBeTruthy();

		const fetched = await getRes.json();
		expect(fetched.repo).toBe('testrepo');
	});

	test('quest created without repo has empty repo field', async ({ apiRequest }) => {
		const createRes = await apiRequest.post('/game/quests', {
			data: {
				objective: 'E2E no repo field test',
				hints: {
					suggested_difficulty: 1,
					suggested_skills: [],
					require_human_review: false,
					budget: 100
				}
			}
		});
		expect(createRes.ok()).toBeTruthy();

		const quest = await createRes.json();
		const instanceId = extractInstance(quest.id);

		const getRes = await apiRequest.get(`/game/quests/${instanceId}`);
		const fetched = await getRes.json();
		expect(fetched.repo ?? '').toBe('');
	});
});

test.describe('Sandbox Repos - Artifact Graceful Degradation', () => {
	test.beforeEach(() => {
		test.skip(!hasBackend(), 'Requires running backend');
	});

	test('artifact list returns empty for non-existent workspace', async ({ lifecycleApi }) => {
		const artifacts = await lifecycleApi.listQuestArtifacts('nonexistent-quest-id');
		expect(artifacts.count).toBe(0);
		expect(artifacts.files).toEqual([]);
	});

	test('artifact list returns empty for quest without sandbox workspace', async ({
		apiRequest,
		lifecycleApi
	}) => {
		// Create a quest but don't run it — no sandbox workspace exists.
		const createRes = await apiRequest.post('/game/quests', {
			data: {
				objective: 'E2E artifact degradation test',
				hints: {
					suggested_difficulty: 1,
					suggested_skills: [],
					require_human_review: false,
					budget: 100
				}
			}
		});
		const quest = await createRes.json();
		const instanceId = extractInstance(quest.id);

		const artifacts = await lifecycleApi.listQuestArtifacts(instanceId);
		expect(artifacts.count).toBe(0);
		expect(artifacts.files).toEqual([]);
	});
});
