<script lang="ts">
	import type { Snippet, Component } from 'svelte';
	import { Package } from 'lucide-svelte';

	/**
	 * EmptyState Component
	 * Requirements: 13.1, 13.3
	 * 
	 * Provides contextual empty states with:
	 * - Icon element
	 * - Title text
	 * - Description text
	 * - Action button slot
	 * - Consistent styling across all uses
	 */
	interface Props {
		icon?: Component;
		title: string;
		description: string;
		action?: Snippet;
		class?: string;
	}

	let { 
		icon: IconComponent = Package,
		title,
		description,
		action,
		class: className = '',
	}: Props = $props();
</script>

<div 
	class="flex flex-col items-center justify-center py-[var(--spacing-12)] px-[var(--spacing-6)] text-center {className}"
	data-empty-state
>
	<!-- Icon -->
	<div 
		class="flex items-center justify-center w-16 h-16 mb-[var(--spacing-4)] rounded-full bg-[var(--color-background-subtle)] text-[var(--color-text-muted)]"
		data-empty-state-icon
	>
		<IconComponent class="w-8 h-8" />
	</div>
	
	<!-- Title -->
	<h3 
		class="text-[var(--text-lg)] font-semibold text-[var(--color-text)] mb-[var(--spacing-2)]"
		data-empty-state-title
	>
		{title}
	</h3>
	
	<!-- Description -->
	<p 
		class="text-[var(--text-sm)] text-[var(--color-text-secondary)] max-w-md mb-[var(--spacing-6)]"
		data-empty-state-description
	>
		{description}
	</p>
	
	<!-- Action Button -->
	{#if action}
		<div data-empty-state-action>
			{@render action()}
		</div>
	{/if}
</div>
