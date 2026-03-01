<script lang="ts">
	import '../styles/global.css';
	import { onMount } from 'svelte';
	import { sseService } from '$services/sse';
	import { api } from '$services/api';
	import { worldStore } from '$stores/worldStore.svelte';
	import { browser } from '$app/environment';
	import { env } from '$env/dynamic/public';
	import type { Snippet } from 'svelte';

	interface LayoutProps {
		children: Snippet;
	}

	let { children }: LayoutProps = $props();

	const SSE_TIMEOUT_MS = 15_000;

	onMount(() => {
		if (browser) {
			const baseUrl = env.PUBLIC_API_URL || 'http://localhost:8080';
			api.setApiUrl(baseUrl);

			worldStore.setLoading(true);
			sseService.connect(baseUrl);

			// Fallback: clear loading if SSE never completes initial sync
			const timeout = setTimeout(() => {
				if (worldStore.loading) {
					worldStore.setLoading(false);
					worldStore.setError('Connection timed out — data may be incomplete');
				}
			}, SSE_TIMEOUT_MS);

			return () => {
				clearTimeout(timeout);
				sseService.disconnect();
			};
		}
	});
</script>

<div class="app">
	{@render children()}
</div>

<style>
	.app {
		height: 100vh;
		width: 100vw;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
</style>
