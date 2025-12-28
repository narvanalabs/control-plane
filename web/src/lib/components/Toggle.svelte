<script lang="ts">
	import { Switch as SwitchPrimitive } from 'bits-ui';

	/**
	 * Toggle Component (Switch)
	 * Requirements: 16.5
	 * 
	 * Provides Toggle switch component with label and disabled state.
	 * Styled consistently with other form components.
	 */
	interface Props {
		checked?: boolean;
		label?: string;
		description?: string;
		disabled?: boolean;
		class?: string;
		id?: string;
		name?: string;
		onchange?: (checked: boolean) => void;
	}

	let { 
		checked = $bindable(false),
		label,
		description,
		disabled = false,
		class: className = '',
		id,
		name,
		onchange,
	}: Props = $props();

	// Generate a unique ID if not provided
	const toggleId = id || `toggle-${Math.random().toString(36).substring(2, 9)}`;

	function handleCheckedChange(newChecked: boolean) {
		checked = newChecked;
		onchange?.(newChecked);
	}
</script>

<div class="flex items-center justify-between gap-[var(--spacing-3)] {className}" data-toggle-wrapper>
	{#if label || description}
		<div class="flex flex-col gap-[var(--spacing-0-5)]">
			{#if label}
				<label 
					for={toggleId}
					class="text-[var(--text-sm)] font-medium text-[var(--color-text)] select-none
						{disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}"
					data-toggle-label
				>
					{label}
				</label>
			{/if}
			{#if description}
				<p class="text-[var(--text-sm)] text-[var(--color-text-muted)]" data-toggle-description>
					{description}
				</p>
			{/if}
		</div>
	{/if}

	<SwitchPrimitive.Root
		id={toggleId}
		{name}
		{disabled}
		{checked}
		onCheckedChange={handleCheckedChange}
		class="peer relative inline-flex h-[var(--spacing-6)] w-[var(--spacing-11)] shrink-0
			cursor-pointer items-center
			rounded-[var(--radius-full)]
			border-2 border-transparent
			bg-[var(--color-border-strong)]
			transition-colors duration-[var(--transition-normal)]
			focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)] focus:ring-offset-2
			disabled:opacity-50 disabled:cursor-not-allowed
			data-[state=checked]:bg-[var(--color-primary)]"
		data-toggle
	>
		<SwitchPrimitive.Thumb
			class="pointer-events-none block h-[var(--spacing-5)] w-[var(--spacing-5)]
				rounded-[var(--radius-full)]
				bg-[var(--color-surface)]
				shadow-[var(--shadow-sm)]
				ring-0
				transition-transform duration-[var(--transition-normal)]
				data-[state=checked]:translate-x-[var(--spacing-5)]
				data-[state=unchecked]:translate-x-0"
		/>
	</SwitchPrimitive.Root>
</div>
