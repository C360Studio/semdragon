/**
 * Playwright Global Setup
 *
 * Runs once before all tests. Detects whether the backend is available
 * AND has a working KV bucket (SSE endpoint). Both must pass for
 * backend-dependent tests to run.
 *
 * Usage in tests:
 *   import { hasBackend } from '../fixtures/test-base';
 *   test.skip(!hasBackend(), 'Requires backend');
 */
const API_URL = process.env.API_URL || 'http://localhost:8080';

async function globalSetup(): Promise<void> {
	const healthOk = await checkHealth(API_URL);

	if (!healthOk) {
		process.env.E2E_BACKEND_AVAILABLE = 'false';
		console.log(
			`[E2E Setup] Backend NOT available at ${API_URL} — backend-dependent tests will be skipped`
		);
		return;
	}

	// Health is up, but check if SSE endpoint works (KV bucket exists)
	const sseOk = await checkSSE(API_URL);

	if (sseOk) {
		process.env.E2E_BACKEND_AVAILABLE = 'true';
		console.log(`[E2E Setup] Backend available at ${API_URL} (health OK, SSE OK)`);
	} else {
		process.env.E2E_BACKEND_AVAILABLE = 'false';
		console.log(
			`[E2E Setup] Backend health OK but SSE endpoint not ready — backend-dependent tests will be skipped`
		);
	}
}

async function checkHealth(baseURL: string, timeout = 5000): Promise<boolean> {
	const start = Date.now();

	while (Date.now() - start < timeout) {
		try {
			const response = await fetch(`${baseURL}/health`);
			if (response.ok) return true;
		} catch {
			// Not ready yet
		}
		await new Promise((resolve) => setTimeout(resolve, 500));
	}

	return false;
}

async function checkSSE(baseURL: string): Promise<boolean> {
	try {
		// The SSE endpoint returns an "error" event if the KV bucket doesn't exist.
		// A working endpoint returns a "connected" event first.
		const controller = new AbortController();
		const timeout = setTimeout(() => controller.abort(), 3000);

		const response = await fetch(`${baseURL}/game/events`, {
			signal: controller.signal
		});

		clearTimeout(timeout);

		if (!response.ok) return false;
		if (!response.body) return false;

		// Read the first chunk to check for "connected" vs "error" event
		const reader = response.body.getReader();
		const decoder = new TextDecoder();
		let text = '';

		const readWithTimeout = new Promise<string>((resolve) => {
			const t = setTimeout(() => {
				reader.cancel();
				resolve(text);
			}, 2000);

			(async () => {
				try {
					const { value, done } = await reader.read();
					clearTimeout(t);
					if (!done && value) {
						text = decoder.decode(value);
					}
					reader.cancel();
					resolve(text);
				} catch {
					clearTimeout(t);
					resolve(text);
				}
			})();
		});

		text = await readWithTimeout;
		return text.includes('event: connected');
	} catch {
		return false;
	}
}

export default globalSetup;
