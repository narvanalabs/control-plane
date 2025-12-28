<script lang="ts">
	import { toastStore } from '$lib/stores/toast.svelte';
	import Toast from './Toast.svelte';

	/**
	 * ToastContainer Component
	 * Requirements: 12.1
	 * 
	 * Container for positioning toast notifications in the top-right corner.
	 * Renders all active toasts from the toast store.
	 */
</script>

<div
	class="fixed top-4 right-4 z-[var(--z-toast)] flex flex-col gap-3 pointer-events-none"
	aria-live="polite"
	aria-label="Notifications"
>
	{#each toastStore.toasts as toast (toast.id)}
		<div class="pointer-events-auto">
			<Toast
				id={toast.id}
				type={toast.type}
				title={toast.title}
				description={toast.description}
				onDismiss={(id) => toastStore.remove(id)}
			/>
		</div>
	{/each}
</div>
