<script lang="ts">
	import type { Snippet } from 'svelte';

	/**
	 * Card Component
	 * Requirements: 16.3, 1.1
	 * 
	 * Provides Card component with optional header, footer, and hover states.
	 * Uses white background with subtle border per design system.
	 */
	interface Props {
		class?: string;
		style?: string;
		hover?: boolean;
		padding?: 'none' | 'sm' | 'md' | 'lg';
		children: Snippet;
		header?: Snippet;
		footer?: Snippet;
	}

	let { 
		class: className = '', 
		style = '',
		hover = false, 
		padding = 'md',
		children,
		header,
		footer,
	}: Props = $props();

	// Padding classes using design system tokens
	const paddingClasses: Record<string, string> = {
		none: '',
		sm: 'p-[var(--spacing-3)]',
		md: 'p-[var(--spacing-4)]',
		lg: 'p-[var(--spacing-6)]',
	};

	// Content padding (used when header/footer are present)
	const contentPaddingClasses: Record<string, string> = {
		none: '',
		sm: 'px-[var(--spacing-3)] py-[var(--spacing-3)]',
		md: 'px-[var(--spacing-4)] py-[var(--spacing-4)]',
		lg: 'px-[var(--spacing-6)] py-[var(--spacing-6)]',
	};

	// Header/footer padding
	const sectionPaddingClasses: Record<string, string> = {
		none: '',
		sm: 'px-[var(--spacing-3)] py-[var(--spacing-2)]',
		md: 'px-[var(--spacing-4)] py-[var(--spacing-3)]',
		lg: 'px-[var(--spacing-6)] py-[var(--spacing-4)]',
	};

	const hasHeaderOrFooter = header || footer;
</script>

<div 
	class="rounded-[var(--radius-lg)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-sm)]
		{hover ? 'hover:border-[var(--color-border-strong)] hover:shadow-[var(--shadow-md)] transition-all duration-[var(--transition-normal)] cursor-pointer' : ''}
		{className}"
	{style}
	data-card
	data-hover={hover ? 'true' : undefined}
	data-padding={padding}
>
	{#if header}
		<div 
			class="border-b border-[var(--color-border)] {sectionPaddingClasses[padding]}"
			data-card-header
		>
			{@render header()}
		</div>
	{/if}
	
	<div class="{hasHeaderOrFooter ? contentPaddingClasses[padding] : paddingClasses[padding]}" data-card-content>
		{@render children()}
	</div>
	
	{#if footer}
		<div 
			class="border-t border-[var(--color-border)] {sectionPaddingClasses[padding]}"
			data-card-footer
		>
			{@render footer()}
		</div>
	{/if}
</div>




