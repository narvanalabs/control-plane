<script lang="ts">
	import { page } from '$app/stores';
	import { authState } from '$lib/stores.svelte';

	/**
	 * Header Component with Breadcrumbs
	 * Requirements: 2.2, 2.4
	 * 
	 * Displays breadcrumb navigation, search trigger for command palette,
	 * and user avatar with dropdown menu.
	 */

	interface Props {
		onSearchClick?: () => void;
	}

	let { onSearchClick }: Props = $props();

	// User dropdown state
	let showUserMenu = $state(false);

	// Generate breadcrumbs from current path
	function generateBreadcrumbs(pathname: string): Array<{ label: string; href: string }> {
		const segments = pathname.split('/').filter(Boolean);
		const breadcrumbs: Array<{ label: string; href: string }> = [];
		
		let currentPath = '';
		for (const segment of segments) {
			currentPath += `/${segment}`;
			breadcrumbs.push({
				label: formatSegment(segment),
				href: currentPath
			});
		}
		
		return breadcrumbs;
	}

	function formatSegment(segment: string): string {
		// Map known segments to display names
		const segmentMap: Record<string, string> = {
			'dashboard': 'Dashboard',
			'apps': 'Applications',
			'nodes': 'Infrastructure',
			'settings': 'Settings',
			'deployments': 'Deployments',
			'logs': 'Logs',
			'secrets': 'Secrets',
		};
		
		return segmentMap[segment] || segment.charAt(0).toUpperCase() + segment.slice(1);
	}

	function handleLogout() {
		authState.logout();
		window.location.href = '/login';
	}

	function toggleUserMenu() {
		showUserMenu = !showUserMenu;
	}

	function closeUserMenu() {
		showUserMenu = false;
	}

	// Icons
	const icons = {
		search: 'M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z',
		chevronRight: 'M9 5l7 7-7 7',
		home: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6',
		logout: 'M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1',
		settings: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 11-6 0 3 3 0 016 0z',
	};

	const breadcrumbs = $derived(generateBreadcrumbs($page.url.pathname));
</script>

<svelte:window onclick={() => showUserMenu && closeUserMenu()} />

<header class="h-14 bg-[var(--color-surface)] border-b border-[var(--color-border)] flex items-center justify-between px-6" data-testid="header">
	<!-- Breadcrumbs -->
	<nav class="flex items-center gap-2 text-sm" aria-label="Breadcrumb">
		<a 
			href="/dashboard" 
			class="text-[var(--color-text-muted)] hover:text-[var(--color-text)] transition-colors"
			aria-label="Home"
		>
			<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={icons.home} />
			</svg>
		</a>
		
		{#each breadcrumbs as crumb, index}
			<svg class="w-4 h-4 text-[var(--color-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={icons.chevronRight} />
			</svg>
			
			{#if index === breadcrumbs.length - 1}
				<span class="font-medium text-[var(--color-text)]">{crumb.label}</span>
			{:else}
				<a 
					href={crumb.href}
					class="text-[var(--color-text-muted)] hover:text-[var(--color-text)] transition-colors"
				>
					{crumb.label}
				</a>
			{/if}
		{/each}
	</nav>

	<!-- Right side: Search + User -->
	<div class="flex items-center gap-3">
		<!-- Search trigger -->
		<button
			onclick={onSearchClick}
			class="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius-md)] bg-[var(--color-background-subtle)] border border-[var(--color-border)] text-[var(--color-text-muted)] hover:text-[var(--color-text)] hover:border-[var(--color-border-strong)] transition-colors text-sm"
			aria-label="Open command palette"
		>
			<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={icons.search} />
			</svg>
			<span class="hidden sm:inline">Search...</span>
			<kbd class="hidden sm:inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-xs font-mono">
				<span class="text-[10px]">⌘</span>K
			</kbd>
		</button>

		<!-- User avatar with dropdown -->
		{#if authState.user}
			<div class="relative">
				<button
					onclick={(e) => { e.stopPropagation(); toggleUserMenu(); }}
					class="flex items-center gap-2 p-1 rounded-[var(--radius-md)] hover:bg-[var(--color-surface-hover)] transition-colors"
					aria-expanded={showUserMenu}
					aria-haspopup="true"
				>
					<div class="w-8 h-8 rounded-full bg-[var(--color-primary)] flex items-center justify-center text-[var(--color-primary-foreground)] font-medium text-sm">
						{authState.user.email.charAt(0).toUpperCase()}
					</div>
				</button>

				<!-- Dropdown menu -->
				{#if showUserMenu}
					<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
					<div 
						class="absolute right-0 top-full mt-1 w-56 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-lg)] shadow-[var(--shadow-lg)] py-1 z-[var(--z-dropdown)] animate-scale-in"
						role="menu"
						tabindex="-1"
						onclick={(e) => e.stopPropagation()}
						onkeydown={(e) => e.key === 'Escape' && closeUserMenu()}
					>
						<div class="px-3 py-2 border-b border-[var(--color-border)]">
							<p class="text-sm font-medium text-[var(--color-text)] truncate">{authState.user.email}</p>
							<p class="text-xs text-[var(--color-text-muted)]">Personal Account</p>
						</div>
						
						<div class="py-1">
							<a 
								href="/settings"
								class="flex items-center gap-2 px-3 py-2 text-sm text-[var(--color-text-secondary)] hover:text-[var(--color-text)] hover:bg-[var(--color-surface-hover)] transition-colors"
								role="menuitem"
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={icons.settings} />
								</svg>
								Settings
							</a>
						</div>
						
						<div class="border-t border-[var(--color-border)] py-1">
							<button
								onclick={handleLogout}
								class="w-full flex items-center gap-2 px-3 py-2 text-sm text-[var(--color-error)] hover:bg-[var(--color-error-light)] transition-colors"
								role="menuitem"
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={icons.logout} />
								</svg>
								Sign out
							</button>
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</header>
