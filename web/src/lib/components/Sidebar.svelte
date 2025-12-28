<script lang="ts">
	import { page } from '$app/stores';
	import { authState } from '$lib/stores.svelte';

	interface NavItem {
		href: string;
		label: string;
		icon: string;
	}

	const navItems: NavItem[] = [
		{ href: '/dashboard', label: 'Dashboard', icon: '◈' },
		{ href: '/apps', label: 'Applications', icon: '⬡' },
		{ href: '/nodes', label: 'Infrastructure', icon: '⎔' },
	];

	function isActive(href: string, pathname: string): boolean {
		if (href === '/dashboard') return pathname === '/dashboard' || pathname === '/';
		return pathname.startsWith(href);
	}
</script>

<aside class="fixed left-0 top-0 h-full w-64 bg-[var(--color-narvana-surface)] border-r border-[var(--color-narvana-border)] flex flex-col z-50">
	<!-- Logo -->
	<div class="p-6 border-b border-[var(--color-narvana-border)]">
		<a href="/dashboard" class="flex items-center gap-3 group">
			<div class="w-10 h-10 rounded-lg bg-gradient-to-br from-[var(--color-narvana-primary)] to-[var(--color-narvana-secondary)] flex items-center justify-center text-[var(--color-narvana-bg)] font-bold text-xl group-hover:scale-105 transition-transform">
				N
			</div>
			<div>
				<h1 class="text-lg font-bold tracking-tight">Narvana</h1>
				<p class="text-xs text-[var(--color-narvana-text-muted)]">Control Plane</p>
			</div>
		</a>
	</div>

	<!-- Navigation -->
	<nav class="flex-1 p-4 space-y-1">
		{#each navItems as item}
			{@const active = isActive(item.href, $page.url.pathname)}
			<a
				href={item.href}
				class="flex items-center gap-3 px-4 py-3 rounded-lg transition-all duration-200
					{active 
						? 'bg-[var(--color-narvana-primary-glow)] text-[var(--color-narvana-primary)] border border-[var(--color-narvana-primary)]/30' 
						: 'text-[var(--color-narvana-text-dim)] hover:text-[var(--color-narvana-text)] hover:bg-[var(--color-narvana-surface-hover)]'}"
			>
				<span class="text-lg {active ? 'text-[var(--color-narvana-primary)]' : ''}">{item.icon}</span>
				<span class="font-medium">{item.label}</span>
			</a>
		{/each}
	</nav>

	<!-- User section -->
	<div class="p-4 border-t border-[var(--color-narvana-border)]">
		{#if authState.user}
			<div class="flex items-center gap-3 px-4 py-3">
				<div class="w-8 h-8 rounded-full bg-gradient-to-br from-[var(--color-narvana-secondary)] to-[var(--color-narvana-primary)] flex items-center justify-center text-[var(--color-narvana-bg)] font-bold text-sm">
					{authState.user.email.charAt(0).toUpperCase()}
				</div>
				<div class="flex-1 min-w-0">
					<p class="text-sm font-medium truncate">{authState.user.email}</p>
				</div>
				<button
					onclick={() => {
						authState.logout();
						window.location.href = '/login';
					}}
					class="p-2 rounded-lg hover:bg-[var(--color-narvana-surface-hover)] text-[var(--color-narvana-text-muted)] hover:text-[var(--color-narvana-error)] transition-colors"
					title="Logout"
				>
					⏻
				</button>
			</div>
		{/if}
	</div>
</aside>




