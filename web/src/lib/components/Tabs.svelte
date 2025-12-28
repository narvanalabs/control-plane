<script lang="ts">
	import { Tabs as TabsPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';

	/**
	 * Tabs Component
	 * Requirements: 16.7
	 * 
	 * Provides Tabs component for sectioned content with tab list,
	 * tab triggers, and tab panels. Includes active tab indicator styling.
	 */
	interface TabItem {
		value: string;
		label: string;
		disabled?: boolean;
	}

	interface Props {
		tabs: TabItem[];
		value?: string;
		class?: string;
		onchange?: (value: string) => void;
		children: Snippet<[{ value: string }]>;
	}

	let { 
		tabs = [],
		value = $bindable(tabs[0]?.value || ''),
		class: className = '',
		onchange,
		children,
	}: Props = $props();

	function handleValueChange(newValue: string) {
		value = newValue;
		onchange?.(newValue);
	}
</script>

<TabsPrimitive.Root
	{value}
	onValueChange={handleValueChange}
	class={className}
	data-tabs-root
>
	<TabsPrimitive.List
		class="flex border-b border-[var(--color-border)] gap-[var(--spacing-1)]"
		data-tabs-list
	>
		{#each tabs as tab (tab.value)}
			<TabsPrimitive.Trigger
				value={tab.value}
				disabled={tab.disabled}
				class="relative px-[var(--spacing-4)] py-[var(--spacing-2-5)]
					text-[var(--text-sm)] font-medium
					text-[var(--color-text-secondary)]
					transition-colors duration-[var(--transition-fast)]
					hover:text-[var(--color-text)]
					focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)] focus-visible:ring-offset-2
					disabled:opacity-50 disabled:cursor-not-allowed
					data-[state=active]:text-[var(--color-text)]
					after:absolute after:bottom-0 after:left-0 after:right-0 after:h-[2px]
					after:bg-transparent after:transition-colors after:duration-[var(--transition-fast)]
					data-[state=active]:after:bg-[var(--color-primary)]"
				data-tabs-trigger
				data-tab-value={tab.value}
			>
				{tab.label}
			</TabsPrimitive.Trigger>
		{/each}
	</TabsPrimitive.List>

	{#each tabs as tab (tab.value)}
		<TabsPrimitive.Content
			value={tab.value}
			class="mt-[var(--spacing-4)] focus:outline-none"
			data-tabs-content
			data-tab-value={tab.value}
		>
			{@render children({ value: tab.value })}
		</TabsPrimitive.Content>
	{/each}
</TabsPrimitive.Root>
