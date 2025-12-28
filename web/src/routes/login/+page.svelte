<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth, APIError } from '$lib/api';
	import { authState } from '$lib/stores.svelte';
	import { Button, Input, Card } from '$lib/components';

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);
	let isRegister = $state(false);
	let setupRequired = $state(false);
	let checkingSetup = $state(true);

	// Check if initial setup is required
	$effect(() => {
		checkSetup();
	});

	async function checkSetup() {
		try {
			const result = await auth.checkSetup();
			setupRequired = !result.setup_complete;
			if (setupRequired) {
				isRegister = true;
			}
		} catch {
			// Ignore setup check errors
		} finally {
			checkingSetup = false;
		}
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			if (isRegister) {
				const result = await auth.register(email, password);
				authState.login({ id: result.user_id, email: result.email });
			} else {
				const result = await auth.login(email, password);
				authState.login({ id: result.user_id, email: result.email });
			}
			goto('/dashboard');
		} catch (err) {
			if (err instanceof APIError) {
				error = err.message;
			} else {
				error = 'An unexpected error occurred';
			}
		} finally {
			loading = false;
		}
	}
</script>

<div class="min-h-screen flex items-center justify-center p-4">
	<div class="w-full max-w-md animate-slide-up">
		<Card class="p-8">
			<!-- Logo -->
			<div class="text-center mb-8">
				<div class="w-16 h-16 mx-auto rounded-xl bg-gradient-to-br from-[var(--color-narvana-primary)] to-[var(--color-narvana-secondary)] flex items-center justify-center text-[var(--color-narvana-bg)] font-bold text-2xl mb-4">
					N
				</div>
				<h1 class="text-2xl font-bold">
					{#if checkingSetup}
						Loading...
					{:else if setupRequired}
						Welcome to Narvana
					{:else if isRegister}
						Create Account
					{:else}
						Sign In
					{/if}
				</h1>
				<p class="text-[var(--color-narvana-text-dim)] mt-2">
					{#if setupRequired}
						Create your admin account to get started
					{:else if isRegister}
						Create a new account
					{:else}
						Sign in to your account
					{/if}
				</p>
			</div>

			{#if setupRequired}
				<div class="mb-6 p-4 rounded-lg bg-[var(--color-narvana-primary-glow)] border border-[var(--color-narvana-primary)]/30">
					<p class="text-sm text-[var(--color-narvana-primary)]">
						✨ Initial setup - create your first admin account
					</p>
				</div>
			{/if}

			<form onsubmit={handleSubmit} class="space-y-4">
				<Input
					type="email"
					label="Email"
					placeholder="admin@example.com"
					bind:value={email}
					required
				/>

				<Input
					type="password"
					label="Password"
					placeholder="Min 8 characters"
					bind:value={password}
					required
				/>

				{#if error}
					<div class="p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-red-400 text-sm">
						{error}
					</div>
				{/if}

				<Button type="submit" class="w-full" {loading}>
					{isRegister ? 'Create Account' : 'Sign In'}
				</Button>
			</form>

			{#if !setupRequired && !checkingSetup}
				<div class="mt-6 text-center">
					<button
						type="button"
						class="text-[var(--color-narvana-primary)] hover:underline text-sm"
						onclick={() => isRegister = !isRegister}
					>
						{isRegister ? 'Already have an account? Sign in' : "Don't have an account? Register"}
					</button>
				</div>
			{/if}
		</Card>
	</div>
</div>



