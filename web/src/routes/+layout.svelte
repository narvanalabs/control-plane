<script lang="ts">
	import './layout.css';
	import { page } from '$app/stores';
	import { Sidebar } from '$lib/components';
	import { authState } from '$lib/stores.svelte';

	let { children } = $props();

	// Public routes that don't need sidebar
	const publicRoutes = ['/login', '/register', '/auth/device'];
	const isPublicRoute = $derived(publicRoutes.some(r => $page.url.pathname.startsWith(r)));
</script>

<svelte:head>
	<title>Narvana</title>
	<link rel="preconnect" href="https://fonts.googleapis.com">
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous">
	<link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet">
</svelte:head>

{#if isPublicRoute}
	<!-- Public routes without sidebar -->
	{@render children()}
{:else if authState.isAuthenticated}
	<!-- Authenticated layout with sidebar -->
	<div class="min-h-screen">
		<Sidebar />
		<main class="ml-64 min-h-screen">
			{@render children()}
		</main>
	</div>
{:else}
	<!-- Not authenticated, show login prompt -->
	<div class="min-h-screen flex items-center justify-center p-4">
		<div class="text-center space-y-4">
			<div class="w-16 h-16 mx-auto rounded-xl bg-gradient-to-br from-[var(--color-narvana-primary)] to-[var(--color-narvana-secondary)] flex items-center justify-center text-[var(--color-narvana-bg)] font-bold text-2xl">
				N
			</div>
			<h1 class="text-2xl font-bold">Welcome to Narvana</h1>
			<p class="text-[var(--color-narvana-text-dim)]">Please sign in to continue</p>
			<a 
				href="/login" 
				class="inline-block px-6 py-3 rounded-lg bg-[var(--color-narvana-primary)] text-[var(--color-narvana-bg)] font-medium hover:bg-[var(--color-narvana-primary-dim)] transition-colors"
			>
				Sign In
			</a>
		</div>
	</div>
{/if}
