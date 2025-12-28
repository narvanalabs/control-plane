<script lang="ts">
	import type { Snippet } from 'svelte';

	/**
	 * PageHeader Component
	 * Requirements: 5.1
	 * 
	 * Consistent page headers with title, description,
	 * action buttons slot, and optional tabs for sub-navigation.
	 */

	interface Tab {
		id: string;
		label: string;
		href?: string;
	}

	interface Props {
		title: string;
		description?: string;
		actions?: Snippet;
		tabs?: Tab[];
		activeTab?: string;
		onTabChange?: (tabId: string) => void;
	}

	let { 
		title, 
		description, 
		actions,
		tabs,
		activeTab,
		onTabChange
	}: Props = $props();
</script>

<div class="border-b border-[var(--color-border)] bg-[var(--color-surface)]" data-testid="page-header">
	<div class="px-6 py-5">
		<div class="flex items-start justify-between gap-4">
			<!-- Title and description -->
			<div class="min-w-0">
				<h1 class="text-2xl font-semibold text-[var(--color-text)] truncate">{title}</h1>
				{#if description}
					<p class="mt-1 text-sm text-[var(--color-text-secondary)]">{description}</p>
				{/if}
			</div>

			<!-- Action buttons -->
			{#if actions}
				<div class="flex items-center gap-2 shrink-0">
					{@render actions()}
				</div>
			{/if}
		</div>
	</div>

	<!-- Tabs for sub-navigation -->
	{#if tabs && tabs.length > 0}
		<div class="px-6">
			<nav class="flex gap-6 -mb-px" aria-label="Tabs">
				{#each tabs as tab}
					{@const isActive = activeTab === tab.id}
					{#if tab.href}
						<a
							href={tab.href}
							class="py-3 text-sm font-medium border-b-2 transition-colors"
							class:border-[var(--color-primary)]={isActive}
							class:text-[var(--color-text)]={isActive}
							class:border-transparent={!isActive}
							class:text-[var(--color-text-secondary)]={!isActive}
							class:hover:text-[var(--color-text)]={!isActive}
							class:hover:border-[var(--color-border-strong)]={!isActive}
							aria-current={isActive ? 'page' : undefined}
						>
							{tab.label}
						</a>
					{:else}
						<button
							onclick={() => onTabChange?.(tab.id)}
							class="py-3 text-sm font-medium border-b-2 transition-colors"
							class:border-[var(--color-primary)]={isActive}
							class:text-[var(--color-text)]={isActive}
							class:border-transparent={!isActive}
							class:text-[var(--color-text-secondary)]={!isActive}
							class:hover:text-[var(--color-text)]={!isActive}
							class:hover:border-[var(--color-border-strong)]={!isActive}
							aria-current={isActive ? 'page' : undefined}
						>
							{tab.label}
						</button>
					{/if}
				{/each}
			</nav>
		</div>
	{/if}
</div>
