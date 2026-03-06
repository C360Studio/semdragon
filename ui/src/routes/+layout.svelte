<script lang="ts">
	import '../styles/global.css';
	import { onMount } from 'svelte';
	import { sseService } from '$services/sse';
	import { api } from '$services/api';
	import { worldStore } from '$stores/worldStore.svelte';
	import { browser } from '$app/environment';
	import { env } from '$env/dynamic/public';
	import type { Snippet } from 'svelte';
	import ChatPanel from '$lib/components/chat/ChatPanel.svelte';
	import GameStatusBar from '$lib/components/layout/GameStatusBar.svelte';

	interface LayoutProps {
		children: Snippet;
	}

	let { children }: LayoutProps = $props();

	const SSE_TIMEOUT_MS = 15_000;
	const TOKEN_POLL_MS = 30_000;

	function pollTokenStats() {
		api.getTokenStats()
			.then((ts) => {
				worldStore.setTokenStats(
					ts.hourly_usage.total_tokens,
					ts.hourly_limit,
					ts.budget_pct,
					ts.breaker,
					ts.hourly_cost_usd,
					ts.total_cost_usd
				);
			})
			.catch(() => {
				// Token stats unavailable — leave defaults.
			});
	}

	onMount(() => {
		if (browser) {
			document.body.classList.add('hydrated');

			// In Docker: Caddy reverse-proxies /game/* and /health to the backend,
			// so all requests are same-origin (PUBLIC_API_URL is empty).
			// In local dev without Docker: Vite proxies the same paths.
			const baseUrl = env.PUBLIC_API_URL || '';
			api.setApiUrl(baseUrl);

			worldStore.setLoading(true);
			sseService.connect(baseUrl);

			// Hydrate board pause state (graceful fallback if endpoint unavailable).
			api.getBoardStatus()
				.then((status) => {
					worldStore.setBoardPaused(status.paused, status.paused_at, status.paused_by);
				})
				.catch(() => {
					// Endpoint unavailable — treat as running.
				});

			// Hydrate token stats and poll periodically.
			pollTokenStats();
			const tokenInterval = setInterval(pollTokenStats, TOKEN_POLL_MS);

			// Fallback: clear loading if SSE never completes initial sync
			const timeout = setTimeout(() => {
				if (worldStore.loading) {
					worldStore.setLoading(false);
					worldStore.setError('Connection timed out — data may be incomplete');
				}
			}, SSE_TIMEOUT_MS);

			return () => {
				clearTimeout(timeout);
				clearInterval(tokenInterval);
				sseService.disconnect();
				document.body.classList.remove('hydrated');
			};
		}
	});
</script>

<div class="app">
	<GameStatusBar />
	<div class="app-content">
		{@render children()}
	</div>
	<ChatPanel />
</div>

<style>
	.app {
		height: 100vh;
		width: 100vw;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.app-content {
		flex: 1;
		overflow: hidden;
	}
</style>
