<script lang="ts">
	import type { Snippet } from 'svelte';

	/**
	 * Button Component
	 * Requirements: 16.1, 1.2
	 * 
	 * Provides Button component with variants (primary, secondary, ghost, danger, outline)
	 * and sizes (sm, md, lg). Uses flat colors without gradients per design system.
	 */
	interface Props {
		variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'outline';
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

	// Base classes for all buttons - uses design system tokens
	const baseClasses = 'inline-flex items-center justify-center gap-2 font-medium transition-all duration-[var(--transition-normal)] disabled:opacity-50 disabled:cursor-not-allowed focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-[var(--color-primary)]';

	// Variant classes using design system tokens - flat colors without gradients
	const variantClasses: Record<string, string> = {
		primary: 'bg-[var(--color-primary)] text-[var(--color-primary-foreground)] hover:bg-[var(--color-primary-hover)]',
		secondary: 'bg-[var(--color-secondary)] text-[var(--color-secondary-foreground)] hover:bg-[var(--color-secondary-hover)]',
		ghost: 'text-[var(--color-text-secondary)] hover:text-[var(--color-text)] hover:bg-[var(--color-surface-hover)]',
		danger: 'bg-[var(--color-error)] text-white hover:bg-[var(--color-error)]/90',
		outline: 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-hover)] hover:border-[var(--color-border-strong)]',
	};

	// Size classes using design system spacing and radius
	const sizeClasses: Record<string, string> = {
		sm: 'px-[var(--spacing-3)] py-[var(--spacing-1-5)] text-[var(--text-sm)] rounded-[var(--radius-md)] h-8',
		md: 'px-[var(--spacing-4)] py-[var(--spacing-2)] text-[var(--text-sm)] rounded-[var(--radius-md)] h-9',
		lg: 'px-[var(--spacing-6)] py-[var(--spacing-3)] text-[var(--text-base)] rounded-[var(--radius-md)] h-11',
	};

	// Spinner size classes
	const spinnerSizeClasses: Record<string, string> = {
		sm: 'w-3 h-3',
		md: 'w-4 h-4',
		lg: 'w-5 h-5',
	};
</script>

<button
	{type}
	disabled={disabled || loading}
	{onclick}
	class="{baseClasses} {variantClasses[variant]} {sizeClasses[size]} {className}"
	data-variant={variant}
	data-size={size}
	data-loading={loading}
>
	{#if loading}
		<span class="{spinnerSizeClasses[size]} border-2 border-current border-t-transparent rounded-full animate-spin" aria-hidden="true"></span>
	{/if}
	{@render children()}
</button>




