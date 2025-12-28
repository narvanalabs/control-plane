<script lang="ts">
	import type { ToastType } from '$lib/stores/toast.svelte';
	import { X, CheckCircle, AlertCircle, Info, AlertTriangle } from 'lucide-svelte';

	/**
	 * Toast Component
	 * Requirements: 12.1, 12.2, 12.3
	 * 
	 * Individual toast notification with success, error, info, warning variants.
	 * Displays icon, title, optional description, and dismiss button.
	 */
	interface Props {
		id: string;
		type: ToastType;
		title: string;
		description?: string;
		onDismiss: (id: string) => void;
	}

	let { id, type, title, description, onDismiss }: Props = $props();

	// Icon components for each type
	const icons = {
		success: CheckCircle,
		error: AlertCircle,
		info: Info,
		warning: AlertTriangle,
	};

	// Styling classes for each toast type using design system tokens
	const typeClasses: Record<ToastType, string> = {
		success: 'bg-[var(--color-success-light)] border-[var(--color-success)] text-[var(--color-success-foreground)]',
		error: 'bg-[var(--color-error-light)] border-[var(--color-error)] text-[var(--color-error-foreground)]',
		info: 'bg-[var(--color-info-light)] border-[var(--color-info)] text-[var(--color-info-foreground)]',
		warning: 'bg-[var(--color-warning-light)] border-[var(--color-warning)] text-[var(--color-warning-foreground)]',
	};

	// Icon color classes
	const iconClasses: Record<ToastType, string> = {
		success: 'text-[var(--color-success)]',
		error: 'text-[var(--color-error)]',
		info: 'text-[var(--color-info)]',
		warning: 'text-[var(--color-warning)]',
	};

	const IconComponent = icons[type];
</script>

<div
	role="alert"
	data-toast-type={type}
	class="flex items-start gap-3 p-4 border rounded-[var(--radius-lg)] shadow-[var(--shadow-md)] min-w-[320px] max-w-[420px] animate-slide-in {typeClasses[type]}"
>
	<span class="flex-shrink-0 {iconClasses[type]}">
		<IconComponent class="w-5 h-5" />
	</span>
	
	<div class="flex-1 min-w-0">
		<p class="font-medium text-[var(--text-sm)]">{title}</p>
		{#if description}
			<p class="mt-1 text-[var(--text-sm)] opacity-90">{description}</p>
		{/if}
	</div>
	
	<button
		type="button"
		onclick={() => onDismiss(id)}
		class="flex-shrink-0 p-1 rounded-[var(--radius-sm)] hover:bg-black/10 transition-colors duration-[var(--transition-fast)]"
		aria-label="Dismiss notification"
	>
		<X class="w-4 h-4" />
	</button>
</div>

<style>
	@keyframes slide-in {
		from {
			transform: translateX(100%);
			opacity: 0;
		}
		to {
			transform: translateX(0);
			opacity: 1;
		}
	}
	
	.animate-slide-in {
		animation: slide-in 0.3s ease-out;
	}
</style>
