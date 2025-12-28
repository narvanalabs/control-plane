<script lang="ts">
	import { Checkbox as CheckboxPrimitive } from 'bits-ui';

	/**
	 * Checkbox Component
	 * Requirements: 16.5
	 * 
	 * Provides Checkbox component with label and disabled state.
	 * Styled consistently with other form components.
	 */
	interface Props {
		checked?: boolean;
		label?: string;
		disabled?: boolean;
		required?: boolean;
		class?: string;
		id?: string;
		name?: string;
		onchange?: (checked: boolean) => void;
	}

	let { 
		checked = $bindable(false),
		label,
		disabled = false,
		required = false,
		class: className = '',
		id,
		name,
		onchange,
	}: Props = $props();

	// Generate a unique ID if not provided
	const checkboxId = id || `checkbox-${Math.random().toString(36).substring(2, 9)}`;

	function handleCheckedChange(newChecked: boolean | 'indeterminate') {
		if (typeof newChecked === 'boolean') {
			checked = newChecked;
			onchange?.(newChecked);
		}
	}
</script>

<div class="flex items-center gap-[var(--spacing-2)] {className}" data-checkbox-wrapper>
	<CheckboxPrimitive.Root
		id={checkboxId}
		{name}
		{disabled}
		{checked}
		onCheckedChange={handleCheckedChange}
		class="peer h-[var(--spacing-5)] w-[var(--spacing-5)] shrink-0
			rounded-[var(--radius-sm)]
			border border-[var(--color-border)]
			bg-[var(--color-surface)]
			transition-colors duration-[var(--transition-fast)]
			focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)] focus:ring-offset-2
			disabled:opacity-50 disabled:cursor-not-allowed
			data-[state=checked]:bg-[var(--color-primary)] data-[state=checked]:border-[var(--color-primary)]
			flex items-center justify-center"
		data-checkbox
	>
		{#if checked}
			<svg class="w-3.5 h-3.5 text-[var(--color-primary-foreground)]" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
			</svg>
		{/if}
	</CheckboxPrimitive.Root>

	{#if label}
		<label 
			for={checkboxId}
			class="text-[var(--text-sm)] text-[var(--color-text)] select-none
				peer-disabled:opacity-50 peer-disabled:cursor-not-allowed"
			data-checkbox-label
		>
			{label}
			{#if required}<span class="text-[var(--color-error)] ml-0.5">*</span>{/if}
		</label>
	{/if}
</div>
