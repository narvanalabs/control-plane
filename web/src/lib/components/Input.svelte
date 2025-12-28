<script lang="ts">
	import type { Snippet } from 'svelte';

	/**
	 * Input Component
	 * Requirements: 16.2
	 * 
	 * Provides Input component with label, placeholder, error state, and icon support.
	 * Supports text, email, password, number, and search types.
	 */
	interface Props {
		type?: 'text' | 'email' | 'password' | 'number' | 'search';
		value?: string;
		placeholder?: string;
		label?: string;
		error?: string;
		disabled?: boolean;
		required?: boolean;
		class?: string;
		id?: string;
		name?: string;
		oninput?: (e: Event) => void;
		onchange?: (e: Event) => void;
		icon?: Snippet;
	}

	let { 
		type = 'text', 
		value = $bindable(''),
		placeholder = '',
		label,
		error,
		disabled = false,
		required = false,
		class: className = '',
		id,
		name,
		oninput,
		onchange,
		icon,
	}: Props = $props();

	// Generate a unique ID if not provided
	const inputId = id || `input-${Math.random().toString(36).substring(2, 9)}`;
</script>

<div class="space-y-[var(--spacing-1-5)] {className}" data-input-wrapper>
	{#if label}
		<label 
			for={inputId} 
			class="block text-[var(--text-sm)] font-medium text-[var(--color-text)]"
			data-input-label
		>
			{label}
			{#if required}<span class="text-[var(--color-error)] ml-0.5">*</span>{/if}
		</label>
	{/if}
	<div class="relative">
		{#if icon}
			<div class="absolute left-[var(--spacing-3)] top-1/2 -translate-y-1/2 text-[var(--color-text-muted)]" data-input-icon>
				{@render icon()}
			</div>
		{/if}
		<input
			id={inputId}
			{name}
			{type}
			bind:value
			{placeholder}
			{disabled}
			{required}
			{oninput}
			{onchange}
			class="w-full py-[var(--spacing-2)] rounded-[var(--radius-sm)]
				bg-[var(--color-surface)]
				border {error ? 'border-[var(--color-error)]' : 'border-[var(--color-border)]'}
				text-[var(--color-text)]
				text-[var(--text-sm)]
				placeholder:text-[var(--color-text-muted)]
				focus:outline-none focus:border-[var(--color-primary)] focus:ring-1 focus:ring-[var(--color-primary)]
				disabled:opacity-50 disabled:cursor-not-allowed disabled:bg-[var(--color-background-subtle)]
				transition-colors duration-[var(--transition-fast)]
				{icon ? 'pl-[var(--spacing-10)] pr-[var(--spacing-4)]' : 'px-[var(--spacing-4)]'}"
			data-input-field
			data-has-error={error ? 'true' : undefined}
			data-has-icon={icon ? 'true' : undefined}
		/>
	</div>
	{#if error}
		<p class="text-[var(--text-sm)] text-[var(--color-error)]" data-input-error>{error}</p>
	{/if}
</div>




