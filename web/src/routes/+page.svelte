<script lang="ts">
	import { goto } from '$app/navigation';
	import { authState } from '$lib/stores.svelte';

	// Redirect to dashboard if authenticated, login if not
	$effect(() => {
		if (authState.isAuthenticated) {
			goto('/dashboard');
		} else {
			goto('/login');
		}
	});
</script>

<div class="min-h-screen flex items-center justify-center">
	<div class="w-8 h-8 border-2 border-[var(--color-narvana-primary)] border-t-transparent rounded-full animate-spin"></div>
</div>
