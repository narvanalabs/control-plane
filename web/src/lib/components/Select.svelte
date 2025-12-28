<script lang="ts">
	import { Select as SelectPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';

	/**
	 * Select Component
	 * Requirements: 16.5
	 * 
	 * Provides Select component with dropdown options, label, and error state support.
	 * Styled consistently with Input component.
	 */
	interface SelectOption {
		value: string;
		label: string;
		disabled?: boolean;
	}

	interface Props {
		options: SelectOption[];
		value?: string;
		placeholder?: string;
		label?: string;
		error?: string;
		disabled?: boolean;
		required?: boolean;
		class?: string;
		id?: string;
		name?: string;
		onchange?: (value: string) => void;
	}

	let { 
		options = [],
		value = $bindable(''),
		placeholder = 'Select an option',
		label,
		error,
		disabled = false,
		required = false,
		class: className = '',
		id,
		name,
		onchange,
	}: Props = $props();

	// Generate a unique ID if not provided
	const selectId = id || `select-${Math.random().toString(36).substring(2, 9)}`;

	// Find the selected option label
	const selectedLabel = $derived(options.find(opt => opt.value === value)?.label || '');

	function handleValueChange(newValue: string | undefined) {
		if (newValue !== undefined) {
			value = newValue;
			onchange?.(newValue);
		}
	}
</script>

<div class="space-y-[var(--spacing-1-5)] {className}" data-select-wrapper>
	{#if label}
		<label 
			for={selectId} 
			class="block text-[var(--text-sm)] font-medium text-[var(--color-text)]"
			data-select-label
		>
			{label}
			{#if required}<span class="text-[var(--color-error)] ml-0.5">*</span>{/if}
		</label>
	{/if}
	
	<SelectPrimitive.Root 
		type="single"
		{name}
		{disabled}
		{value}
		onValueChange={handleValueChange}
	>
		<SelectPrimitive.Trigger
			id={selectId}
			class="w-full flex items-center justify-between py-[var(--spacing-2)] px-[var(--spacing-4)] rounded-[var(--radius-sm)]
				bg-[var(--color-surface)]
				border {error ? 'border-[var(--color-error)]' : 'border-[var(--color-border)]'}
				text-[var(--color-text)]
				text-[var(--text-sm)]
				focus:outline-none focus:border-[var(--color-primary)] focus:ring-1 focus:ring-[var(--color-primary)]
				disabled:opacity-50 disabled:cursor-not-allowed disabled:bg-[var(--color-background-subtle)]
				transition-colors duration-[var(--transition-fast)]
				data-[placeholder]:text-[var(--color-text-muted)]"
			data-select-trigger
			data-has-error={error ? 'true' : undefined}
		>
			<span class={!selectedLabel ? 'text-[var(--color-text-muted)]' : ''}>
				{selectedLabel || placeholder}
			</span>
			<svg 
				class="w-4 h-4 text-[var(--color-text-muted)] transition-transform duration-[var(--transition-fast)]" 
				fill="none" 
				stroke="currentColor" 
				viewBox="0 0 24 24"
				aria-hidden="true"
			>
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
			</svg>
		</SelectPrimitive.Trigger>

		<SelectPrimitive.Portal>
			<SelectPrimitive.Content
				class="z-[var(--z-dropdown)] min-w-[8rem] overflow-hidden rounded-[var(--radius-lg)]
					bg-[var(--color-surface)] border border-[var(--color-border)]
					shadow-[var(--shadow-lg)]
					animate-in fade-in-0 zoom-in-95"
				sideOffset={4}
				data-select-content
			>
				<SelectPrimitive.Viewport class="p-[var(--spacing-1)]">
					{#each options as option (option.value)}
						<SelectPrimitive.Item
							value={option.value}
							disabled={option.disabled}
							class="relative flex items-center py-[var(--spacing-2)] px-[var(--spacing-3)] pr-[var(--spacing-8)]
								text-[var(--text-sm)] text-[var(--color-text)]
								rounded-[var(--radius-sm)]
								cursor-pointer select-none
								outline-none
								data-[highlighted]:bg-[var(--color-surface-hover)]
								data-[disabled]:opacity-50 data-[disabled]:cursor-not-allowed"
							data-select-item
						>
							{option.label}
							{#if value === option.value}
								<span class="absolute right-[var(--spacing-2)]">
									<svg class="w-4 h-4 text-[var(--color-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
								</span>
							{/if}
						</SelectPrimitive.Item>
					{/each}
				</SelectPrimitive.Viewport>
			</SelectPrimitive.Content>
		</SelectPrimitive.Portal>
	</SelectPrimitive.Root>

	{#if error}
		<p class="text-[var(--text-sm)] text-[var(--color-error)]" data-select-error>{error}</p>
	{/if}
</div>
