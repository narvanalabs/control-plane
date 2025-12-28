<script lang="ts">
	import type { Snippet } from 'svelte';

	/**
	 * Badge Component
	 * Requirements: 16.4
	 * 
	 * Provides Badge component with default, success, warning, error, info variants.
	 * Includes optional dot indicator.
	 */
	interface Props {
		variant?: 'default' | 'success' | 'warning' | 'error' | 'info';
		size?: 'sm' | 'md';
		dot?: boolean;
		class?: string;
		children: Snippet;
	}

	let { 
		variant = 'default', 
		size = 'md',
		dot = false,
		class: className = '',
		children,
	}: Props = $props();

	// Variant classes using design system tokens
	const variantClasses: Record<string, { bg: string; text: string; dot: string }> = {
		default: {
			bg: 'bg-[var(--color-secondary)]',
			text: 'text-[var(--color-text-secondary)]',
			dot: 'bg-[var(--color-text-muted)]',
		},
		success: {
			bg: 'bg-[var(--color-success-light)]',
			text: 'text-[var(--color-success-foreground)]',
			dot: 'bg-[var(--color-success)]',
		},
		warning: {
			bg: 'bg-[var(--color-warning-light)]',
			text: 'text-[var(--color-warning-foreground)]',
			dot: 'bg-[var(--color-warning)]',
		},
		error: {
			bg: 'bg-[var(--color-error-light)]',
			text: 'text-[var(--color-error-foreground)]',
			dot: 'bg-[var(--color-error)]',
		},
		info: {
			bg: 'bg-[var(--color-info-light)]',
			text: 'text-[var(--color-info-foreground)]',
			dot: 'bg-[var(--color-info)]',
		},
	};

	// Size classes
	const sizeClasses: Record<string, string> = {
		sm: 'px-[var(--spacing-2)] py-[var(--spacing-0-5)] text-[var(--text-xs)]',
		md: 'px-[var(--spacing-2-5)] py-[var(--spacing-1)] text-[var(--text-sm)]',
	};

	// Dot size classes
	const dotSizeClasses: Record<string, string> = {
		sm: 'w-1.5 h-1.5',
		md: 'w-2 h-2',
	};

	const config = $derived(variantClasses[variant] || variantClasses.default);
</script>

<span 
	class="inline-flex items-center gap-[var(--spacing-1-5)] rounded-[var(--radius-full)] font-medium {config.bg} {config.text} {sizeClasses[size]} {className}"
	data-badge
	data-variant={variant}
	data-size={size}
>
	{#if dot}
		<span 
			class="rounded-full {config.dot} {dotSizeClasses[size]}" 
			aria-hidden="true"
			data-badge-dot
		></span>
	{/if}
	{@render children()}
</span>
