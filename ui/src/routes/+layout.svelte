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

	onMount(() => {
		if (browser) {
			const baseUrl = env.PUBLIC_API_URL || 'http://localhost:8080';
			api.setApiUrl(baseUrl);

			worldStore.setLoading(true);
			sseService.connect(baseUrl);

			return () => {
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
