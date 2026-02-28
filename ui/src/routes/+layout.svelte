<script lang="ts">
	import '../styles/global.css';
	import { onMount } from 'svelte';
	import { websocketService } from '$services/websocket';
	import { api } from '$services/api';
	import { worldStore } from '$stores/worldStore.svelte';
	import { browser } from '$app/environment';
	import { env } from '$env/dynamic/public';
	import type { Snippet } from 'svelte';

	interface LayoutProps {
		children: Snippet;
	}

	let { children }: LayoutProps = $props();

	onMount(() => {
		if (browser) {
			// Configure API URL from environment
			const apiUrl = env.PUBLIC_API_URL || 'http://localhost:8080';
			api.setApiUrl(apiUrl);

			// Connect to WebSocket
			const wsUrl = env.PUBLIC_WS_URL || 'ws://localhost:8080/events';
			websocketService.connect(wsUrl);

			// Load initial world state
			loadWorldState();

			return () => {
				websocketService.disconnect();
			};
		}
	});

	async function loadWorldState() {
		worldStore.setLoading(true);
		try {
			const state = await api.getWorldState();
			worldStore.setWorldState(state);
		} catch (err) {
			console.error('Failed to load world state:', err);
			worldStore.setError('Failed to load world state');
		} finally {
			worldStore.setLoading(false);
		}
	}
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
