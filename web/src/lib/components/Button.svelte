<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
		size?: 'sm' | 'md' | 'lg';
		disabled?: boolean;
		loading?: boolean;
		type?: 'button' | 'submit';
		class?: string;
		onclick?: () => void;
		children: Snippet;
	}

	let { 
		variant = 'primary', 
		size = 'md', 
		disabled = false, 
		loading = false,
		type = 'button',
		class: className = '',
		onclick,
		children 
	}: Props = $props();

	const baseClasses = 'inline-flex items-center justify-center gap-2 font-medium rounded-lg transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed';

	const variantClasses: Record<string, string> = {
		primary: 'bg-[var(--color-narvana-primary)] text-[var(--color-narvana-bg)] hover:bg-[var(--color-narvana-primary-dim)] glow-hover',
		secondary: 'bg-[var(--color-narvana-surface-hover)] text-[var(--color-narvana-text)] border border-[var(--color-narvana-border)] hover:border-[var(--color-narvana-border-bright)]',
		ghost: 'text-[var(--color-narvana-text-dim)] hover:text-[var(--color-narvana-text)] hover:bg-[var(--color-narvana-surface-hover)]',
		danger: 'bg-red-500/10 text-red-400 border border-red-500/30 hover:bg-red-500/20',
	};

	const sizeClasses: Record<string, string> = {
		sm: 'px-3 py-1.5 text-sm',
		md: 'px-4 py-2 text-sm',
		lg: 'px-6 py-3 text-base',
	};
</script>

<button
	{type}
	disabled={disabled || loading}
	{onclick}
	class="{baseClasses} {variantClasses[variant]} {sizeClasses[size]} {className}"
>
	{#if loading}
		<span class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin"></span>
	{/if}
	{@render children()}
</button>



