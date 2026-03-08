import { test, expect, hasBackend, retry } from '../fixtures/test-base';

/**
 * Boid Suggestions Observability
 *
 * Verifies that the boid engine produces quest-claiming suggestions and that
 * they are observable via the message-logger KV SSE endpoint. The boid engine
 * runs on a periodic tick and writes suggestions to the BOID_SUGGESTIONS KV
 * bucket, which the message-logger exposes as a Server-Sent Events stream.
 *
 * This test does not assert on the UI — it verifies the backend pipeline:
 *   boidengine tick -> BOID_SUGGESTIONS KV put -> message-logger SSE
 *
 * Requires agents and quests to exist so the engine has data to compute
 * attraction scores against.
 */

test.describe('Boid Suggestions Observability', () => {
	test.beforeEach(async () => {
		if (!hasBackend()) test.skip();
	});

	test('boid suggestions appear in KV bucket after agents and quests exist', async ({
		lifecycleApi
	}) => {
		// Seed: create agents and quests so boid engine has work to do
		const agent1 = await lifecycleApi.recruitAgent(`boid-test-a-${Date.now()}`, ['analysis']);
		const agent2 = await lifecycleApi.recruitAgent(`boid-test-b-${Date.now()}`, ['coding']);

		await lifecycleApi.createQuest('Boid target quest 1', 1);
		await lifecycleApi.createQuest('Boid target quest 2', 1);

		// Poll the message-logger KV endpoint for boid suggestions.
		// Boid engine ticks every 1s; allow up to 15s for entries to appear.
		const entries = await retry(
			async () => {
				const res = await fetch(
					`http://localhost:${process.env.BACKEND_PORT || '8081'}/message-logger/kv/BOID_SUGGESTIONS/watch?pattern=*`
				);
				if (!res.ok) {
					throw new Error(`message-logger request failed: ${res.status}`);
				}

				// SSE endpoint — read a chunk and look for data lines
				const reader = res.body?.getReader();
				if (!reader) throw new Error('No response body');

				const decoder = new TextDecoder();
				let accumulated = '';
				const dataEntries: string[] = [];

				// Read for up to 3s to collect SSE events
				const readTimeout = setTimeout(() => reader.cancel(), 3000);
				try {
					while (true) {
						const { done, value } = await reader.read();
						if (done) break;
						accumulated += decoder.decode(value, { stream: true });

						// Parse SSE data lines
						const lines = accumulated.split('\n');
						for (const line of lines) {
							if (line.startsWith('data: ')) {
								dataEntries.push(line.slice(6));
							}
						}
					}
				} catch {
					// Reader cancelled by timeout — expected
				} finally {
					clearTimeout(readTimeout);
				}

				if (dataEntries.length === 0) {
					throw new Error('No boid suggestion entries found in KV');
				}

				return dataEntries;
			},
			{
				timeout: 15000,
				interval: 2000,
				message: 'Boid suggestions did not appear in KV within 15s'
			}
		);

		expect(entries.length).toBeGreaterThan(0);

		// Verify at least one entry parses as JSON with expected structure.
		// Suggestions are arrays of {agent_id, quest_id, score, ...}
		const parsed = JSON.parse(entries[0]);
		if (Array.isArray(parsed)) {
			expect(parsed.length).toBeGreaterThan(0);
			expect(parsed[0]).toHaveProperty('quest_id');
			expect(parsed[0]).toHaveProperty('score');
		}

		// Clean up references (agents/quests remain in backend; env is ephemeral)
		void agent1;
		void agent2;
	});
});
