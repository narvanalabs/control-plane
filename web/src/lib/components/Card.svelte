<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		class?: string;
		hover?: boolean;
		glow?: boolean;
		children: Snippet;
	}

	let { class: className = '', hover = false, glow = false, children }: Props = $props();
</script>

<div 
	class="rounded-xl border border-[var(--color-narvana-border)] bg-[var(--color-narvana-surface)] 
		{hover ? 'hover:border-[var(--color-narvana-border-bright)] hover:bg-[var(--color-narvana-surface-hover)] transition-all duration-200 cursor-pointer' : ''}
		{glow ? 'glow' : ''}
		{className}"
>
	{@render children()}
</div>



