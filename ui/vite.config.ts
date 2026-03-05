import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

const backendUrl = process.env.BACKEND_URL || 'http://localhost:8080';

export default defineConfig({
	plugins: [sveltekit()],
	test: {
		include: ['src/**/*.{test,spec}.{js,ts}'],
		environment: 'jsdom',
		setupFiles: ['./vitest.setup.ts']
	},
	server: {
		port: 5173,
		host: true,
		proxy: {
			'/game': backendUrl,
			'/health': backendUrl
		}
	}
});
