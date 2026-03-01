import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

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
			'/game': 'http://localhost:8080',
			'/health': 'http://localhost:8080',
			'/message-logger': 'http://localhost:8080'
		}
	}
});
