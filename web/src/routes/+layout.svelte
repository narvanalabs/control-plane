<script lang="ts">
	import './layout.css';
	import { page } from '$app/stores';
	import { NewSidebar as Sidebar, Header, CommandPalette, ToastContainer } from '$lib/components';
	import { authState } from '$lib/stores.svelte';

	/**
	 * Main Layout Component
	 * Requirements: 2.1, 2.6, 14.2
	 * 
	 * Provides the main application layout with sidebar navigation,
	 * header with breadcrumbs, command palette, and toast notifications.
	 * Implements responsive sidebar collapse on mobile.
	 */

	let { children } = $props();

	// Sidebar state
	let sidebarCollapsed = $state(false);
	let mobileMenuOpen = $state(false);

	// Command palette state
	let commandPaletteOpen = $state(false);

	// Public routes that don't need sidebar
	const publicRoutes = ['/login', '/register', '/auth/device'];
	const isPublicRoute = $derived(publicRoutes.some(r => $page.url.pathname.startsWith(r)));

	function toggleSidebar() {
		sidebarCollapsed = !sidebarCollapsed;
	}

	function toggleMobileMenu() {
		mobileMenuOpen = !mobileMenuOpen;
	}

	function openCommandPalette() {
		commandPaletteOpen = true;
	}

	// Close mobile menu on route change
	$effect(() => {
		$page.url.pathname;
		mobileMenuOpen = false;
	});

	// Icons
	const icons = {
		menu: 'M4 6h16M4 12h16M4 18h16',
		close: 'M6 18L18 6M6 6l12 12',
	};
</script>

<svelte:head>
	<title>Narvana</title>
	<link rel="preconnect" href="https://fonts.googleapis.com">
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous">
	<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet">
</svelte:head>

{#if isPublicRoute}
	<!-- Public routes without sidebar -->
	{@render children()}
{:else if authState.isAuthenticated}
	<!-- Authenticated layout with sidebar -->
	<div class="min-h-screen bg-[var(--color-background-subtle)]">
		<!-- Mobile menu button -->
		<button
			onclick={toggleMobileMenu}
			class="fixed top-3 left-3 z-[var(--z-sticky)] p-2 rounded-[var(--radius-md)] bg-[var(--color-surface)] border border-[var(--color-border)] shadow-[var(--shadow-sm)] lg:hidden"
			aria-label={mobileMenuOpen ? 'Close menu' : 'Open menu'}
		>
			<svg class="w-5 h-5 text-[var(--color-text)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={mobileMenuOpen ? icons.close : icons.menu} />
			</svg>
		</button>

		<!-- Mobile sidebar backdrop -->
		{#if mobileMenuOpen}
			<div 
				class="fixed inset-0 bg-black/50 z-[var(--z-modal-backdrop)] lg:hidden animate-fade-in"
				onclick={toggleMobileMenu}
				role="presentation"
			></div>
		{/if}

		<!-- Sidebar - hidden on mobile unless menu is open -->
		<div 
			class="fixed inset-y-0 left-0 z-[var(--z-modal)] transform transition-transform duration-[var(--transition-normal)] lg:transform-none"
			class:translate-x-0={mobileMenuOpen}
			class:-translate-x-full={!mobileMenuOpen}
			class:lg:translate-x-0={true}
		>
			<Sidebar 
				collapsed={sidebarCollapsed} 
				onToggle={toggleSidebar}
			/>
		</div>

		<!-- Main content area -->
		<div 
			class="min-h-screen transition-all duration-[var(--transition-normal)]"
			class:lg:ml-64={!sidebarCollapsed}
			class:lg:ml-16={sidebarCollapsed}
		>
			<!-- Header -->
			<Header onSearchClick={openCommandPalette} />

			<!-- Page content -->
			<main class="min-h-[calc(100vh-3.5rem)]">
				{@render children()}
			</main>
		</div>

		<!-- Command Palette -->
		<CommandPalette bind:open={commandPaletteOpen} />

		<!-- Toast notifications -->
		<ToastContainer />
	</div>
{:else}
	<!-- Not authenticated, show login prompt -->
	<div class="min-h-screen flex items-center justify-center p-4 bg-[var(--color-background)]">
		<div class="text-center space-y-4">
			<div class="w-16 h-16 mx-auto rounded-xl bg-[var(--color-primary)] flex items-center justify-center text-[var(--color-primary-foreground)] font-bold text-2xl">
				N
			</div>
			<h1 class="text-2xl font-bold text-[var(--color-text)]">Welcome to Narvana</h1>
			<p class="text-[var(--color-text-secondary)]">Please sign in to continue</p>
			<a 
				href="/login" 
				class="inline-block px-6 py-3 rounded-[var(--radius-md)] bg-[var(--color-primary)] text-[var(--color-primary-foreground)] font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
			>
				Sign In
			</a>
		</div>
	</div>
{/if}
