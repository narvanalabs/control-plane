<script lang="ts">
	import { page } from '$app/stores';
	import { authState } from '$lib/stores.svelte';
	import { isActive } from './navigation';

	/**
	 * Sidebar Navigation Component
	 * Requirements: 2.1, 2.5
	 * 
	 * Fixed sidebar with navigation items, organization selector,
	 * active item highlighting, and user section with logout.
	 */

	interface NavItem {
		href: string;
		label: string;
		icon: string;
	}

	interface Props {
		collapsed?: boolean;
		onToggle?: () => void;
	}

	let { collapsed = false, onToggle }: Props = $props();

	const navItems: NavItem[] = [
		{ href: '/dashboard', label: 'Dashboard', icon: 'dashboard' },
		{ href: '/apps', label: 'Applications', icon: 'apps' },
		{ href: '/nodes', label: 'Infrastructure', icon: 'nodes' },
	];

	function handleLogout() {
		authState.logout();
		window.location.href = '/login';
	}

	// Icon components as SVG paths
	const icons: Record<string, string> = {
		dashboard: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6',
		apps: 'M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z',
		nodes: 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01',
		logout: 'M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1',
		chevronLeft: 'M15 19l-7-7 7-7',
		chevronRight: 'M9 5l7 7-7 7',
	};
</script>

<aside 
	class="fixed left-0 top-0 h-full bg-[var(--color-surface)] border-r border-[var(--color-border)] flex flex-col z-[var(--z-sticky)] transition-all duration-[var(--transition-normal)]"
	class:w-64={!collapsed}
	class:w-16={collapsed}
	data-testid="sidebar"
>
	<!-- Logo / Brand -->
	<div class="p-4 border-b border-[var(--color-border)]">
		<a href="/dashboard" class="flex items-center gap-3 group">
			<div class="w-9 h-9 rounded-[var(--radius-lg)] bg-[var(--color-primary)] flex items-center justify-center text-[var(--color-primary-foreground)] font-bold text-lg shrink-0 group-hover:scale-105 transition-transform">
				N
			</div>
			{#if !collapsed}
				<div class="overflow-hidden">
					<h1 class="text-base font-semibold text-[var(--color-text)] truncate">Narvana</h1>
					<p class="text-xs text-[var(--color-text-muted)] truncate">Control Plane</p>
				</div>
			{/if}
		</a>
	</div>

	<!-- Organization/Project Selector -->
	{#if !collapsed}
		<div class="p-3 border-b border-[var(--color-border)]">
			<button 
				class="w-full flex items-center gap-2 px-3 py-2 rounded-[var(--radius-md)] bg-[var(--color-background-subtle)] hover:bg-[var(--color-surface-hover)] text-left transition-colors"
				aria-label="Select organization"
			>
				<div class="w-6 h-6 rounded-[var(--radius-sm)] bg-[var(--color-primary)] flex items-center justify-center text-[var(--color-primary-foreground)] text-xs font-medium">
					P
				</div>
				<div class="flex-1 min-w-0">
					<p class="text-sm font-medium text-[var(--color-text)] truncate">Personal</p>
				</div>
				<svg class="w-4 h-4 text-[var(--color-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l4-4 4 4m0 6l-4 4-4-4" />
				</svg>
			</button>
		</div>
	{/if}

	<!-- Navigation -->
	<nav class="flex-1 p-3 space-y-1 overflow-y-auto">
		{#each navItems as item}
			{@const active = isActive(item.href, $page.url.pathname)}
			<a
				href={item.href}
				class="flex items-center gap-3 px-3 py-2.5 rounded-[var(--radius-md)] transition-all duration-[var(--transition-fast)] group"
				class:bg-[var(--color-primary)]={active}
				class:text-[var(--color-primary-foreground)]={active}
				class:text-[var(--color-text-secondary)]={!active}
				class:hover:bg-[var(--color-surface-hover)]={!active}
				class:hover:text-[var(--color-text)]={!active}
				data-active={active}
				aria-current={active ? 'page' : undefined}
				title={collapsed ? item.label : undefined}
			>
				<svg 
					class="w-5 h-5 shrink-0" 
					class:text-[var(--color-primary-foreground)]={active}
					class:text-[var(--color-text-muted)]={!active}
					class:group-hover:text-[var(--color-text)]={!active}
					fill="none" 
					stroke="currentColor" 
					viewBox="0 0 24 24"
				>
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d={icons[item.icon]} />
				</svg>
				{#if !collapsed}
					<span class="font-medium text-sm">{item.label}</span>
				{/if}
			</a>
		{/each}
	</nav>

	<!-- Collapse Toggle -->
	{#if onToggle}
		<div class="p-3 border-t border-[var(--color-border)]">
			<button
				onclick={onToggle}
				class="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-[var(--radius-md)] text-[var(--color-text-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-surface-hover)] transition-colors"
				aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
			>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d={collapsed ? icons.chevronRight : icons.chevronLeft} />
				</svg>
				{#if !collapsed}
					<span class="text-sm">Collapse</span>
				{/if}
			</button>
		</div>
	{/if}

	<!-- User Section -->
	<div class="p-3 border-t border-[var(--color-border)]">
		{#if authState.user}
			<div class="flex items-center gap-3 px-3 py-2">
				<div class="w-8 h-8 rounded-full bg-[var(--color-primary)] flex items-center justify-center text-[var(--color-primary-foreground)] font-medium text-sm shrink-0">
					{authState.user.email.charAt(0).toUpperCase()}
				</div>
				{#if !collapsed}
					<div class="flex-1 min-w-0">
						<p class="text-sm font-medium text-[var(--color-text)] truncate">{authState.user.email}</p>
					</div>
					<button
						onclick={handleLogout}
						class="p-1.5 rounded-[var(--radius-sm)] text-[var(--color-text-muted)] hover:text-[var(--color-error)] hover:bg-[var(--color-error-light)] transition-colors"
						title="Logout"
						aria-label="Logout"
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={icons.logout} />
						</svg>
					</button>
				{:else}
					<button
						onclick={handleLogout}
						class="absolute bottom-16 left-1/2 -translate-x-1/2 p-1.5 rounded-[var(--radius-sm)] text-[var(--color-text-muted)] hover:text-[var(--color-error)] hover:bg-[var(--color-error-light)] transition-colors"
						title="Logout"
						aria-label="Logout"
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={icons.logout} />
						</svg>
					</button>
				{/if}
			</div>
		{/if}
	</div>
</aside>
