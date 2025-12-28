import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	// Consult https://svelte.dev/docs/kit/integrations
	// for more information about preprocessors
	preprocess: vitePreprocess(),

	kit: {
		adapter: adapter({
			// Build output goes to ui/dist (embedded into Go binary)
			pages: '../ui/dist',
			assets: '../ui/dist',
			fallback: 'index.html', // SPA fallback for client-side routing
			precompress: false,
			strict: false // Allow dynamic routes to be handled by fallback
		}),
		paths: {
			// Ensure assets work when served from Go
			base: ''
		},
		prerender: {
			// Dynamic routes like /apps/[id] can't be prerendered
			handleMissingId: 'ignore',
			handleHttpError: 'warn',
			// Only prerender static routes
			entries: ['/', '/login', '/dashboard', '/apps', '/nodes']
		}
	}
};

export default config;
