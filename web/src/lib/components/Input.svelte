<script lang="ts">
	interface Props {
		type?: 'text' | 'email' | 'password' | 'number' | 'url';
		value?: string;
		placeholder?: string;
		label?: string;
		error?: string;
		disabled?: boolean;
		required?: boolean;
		class?: string;
		id?: string;
		oninput?: (e: Event) => void;
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
		oninput,
	}: Props = $props();
</script>

<div class="space-y-1.5 {className}">
	{#if label}
		<label for={id} class="block text-sm font-medium text-[var(--color-narvana-text-dim)]">
			{label}
			{#if required}<span class="text-red-400">*</span>{/if}
		</label>
	{/if}
	<input
		{id}
		{type}
		bind:value
		{placeholder}
		{disabled}
		{required}
		{oninput}
		class="w-full px-4 py-2.5 rounded-lg 
			bg-[var(--color-narvana-bg)] 
			border {error ? 'border-red-500' : 'border-[var(--color-narvana-border)]'} 
			text-[var(--color-narvana-text)] 
			placeholder:text-[var(--color-narvana-text-muted)]
			focus:outline-none focus:border-[var(--color-narvana-primary)] focus:ring-1 focus:ring-[var(--color-narvana-primary)]
			disabled:opacity-50 disabled:cursor-not-allowed
			transition-colors duration-200"
	/>
	{#if error}
		<p class="text-sm text-red-400">{error}</p>
	{/if}
</div>




