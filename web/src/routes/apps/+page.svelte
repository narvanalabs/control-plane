<script lang="ts">
	import { goto } from '$app/navigation';
	import { apps, type App } from '$lib/api';
	import { Card, Button, Input } from '$lib/components';

	let appList = $state<App[]>([]);
	let loading = $state(true);
	let showNewAppModal = $state(false);
	let newAppName = $state('');
	let newAppDescription = $state('');
	let creating = $state(false);
	let error = $state('');

	$effect(() => {
		loadApps();
	});

	async function loadApps() {
		try {
			appList = await apps.list();
		} catch (err) {
			console.error('Failed to load apps:', err);
		} finally {
			loading = false;
		}
	}

	async function createApp(e: Event) {
		e.preventDefault();
		error = '';
		creating = true;

		try {
			const app = await apps.create(newAppName, newAppDescription || undefined);
			showNewAppModal = false;
			newAppName = '';
			newAppDescription = '';
			goto(`/apps/${app.id}`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create app';
		} finally {
			creating = false;
		}
	}

	function formatDate(date: string): string {
		return new Date(date).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
		});
	}
</script>

<div class="p-8">
	<!-- Header -->
	<div class="flex items-center justify-between mb-8">
		<div>
			<h1 class="text-3xl font-bold mb-2">Applications</h1>
			<p class="text-[var(--color-narvana-text-dim)]">Manage your deployed applications</p>
		</div>
		<Button onclick={() => showNewAppModal = true}>
			<span>+</span> New Application
		</Button>
	</div>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="w-8 h-8 border-2 border-[var(--color-narvana-primary)] border-t-transparent rounded-full animate-spin"></div>
		</div>
	{:else if appList.length === 0}
		<!-- Empty State -->
		<Card class="p-12 text-center">
			<div class="w-16 h-16 mx-auto rounded-xl bg-[var(--color-narvana-primary)]/10 flex items-center justify-center mb-4">
				<span class="text-3xl text-[var(--color-narvana-primary)]">⬡</span>
			</div>
			<h2 class="text-xl font-semibold mb-2">No applications yet</h2>
			<p class="text-[var(--color-narvana-text-dim)] mb-6 max-w-md mx-auto">
				Create your first application to start deploying services to your infrastructure.
			</p>
			<Button onclick={() => showNewAppModal = true}>
				<span>+</span> Create Application
			</Button>
		</Card>
	{:else}
		<!-- Apps Grid -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
			{#each appList as app, i}
				<a href="/apps/{app.id}" class="stagger-item" style="animation-delay: {i * 50}ms">
					<Card hover class="p-6 h-full">
						<div class="flex items-start justify-between mb-4">
							<div class="w-12 h-12 rounded-xl bg-gradient-to-br from-[var(--color-narvana-primary)]/20 to-[var(--color-narvana-secondary)]/20 flex items-center justify-center">
								<span class="text-xl font-bold text-[var(--color-narvana-primary)]">
									{app.name.charAt(0).toUpperCase()}
								</span>
							</div>
							<span class="text-xs text-[var(--color-narvana-text-muted)]">
								{formatDate(app.created_at)}
							</span>
						</div>

						<h3 class="text-lg font-semibold mb-1">{app.name}</h3>
						{#if app.description}
							<p class="text-sm text-[var(--color-narvana-text-dim)] mb-4 line-clamp-2">
								{app.description}
							</p>
						{:else}
							<p class="text-sm text-[var(--color-narvana-text-muted)] mb-4 italic">
								No description
							</p>
						{/if}

						<div class="flex items-center gap-4 text-sm text-[var(--color-narvana-text-dim)]">
							<span class="flex items-center gap-1">
								<span class="text-[var(--color-narvana-secondary)]">◇</span>
								{app.services?.length ?? 0} service{(app.services?.length ?? 0) !== 1 ? 's' : ''}
							</span>
						</div>
					</Card>
				</a>
			{/each}
		</div>
	{/if}
</div>

<!-- New App Modal -->
{#if showNewAppModal}
	<div 
		class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4 animate-fade-in"
		onclick={(e) => { if (e.target === e.currentTarget) showNewAppModal = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showNewAppModal = false; }}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<Card class="w-full max-w-md p-6 animate-slide-up">
			<h2 class="text-xl font-semibold mb-4">Create New Application</h2>
			
			<form onsubmit={createApp} class="space-y-4">
				<Input
					label="Application Name"
					placeholder="my-awesome-app"
					bind:value={newAppName}
					required
				/>

				<Input
					label="Description"
					placeholder="Optional description"
					bind:value={newAppDescription}
				/>

				{#if error}
					<div class="p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-red-400 text-sm">
						{error}
					</div>
				{/if}

				<div class="flex gap-3 pt-2">
					<Button variant="ghost" class="flex-1" onclick={() => showNewAppModal = false}>
						Cancel
					</Button>
					<Button type="submit" class="flex-1" loading={creating}>
						Create
					</Button>
				</div>
			</form>
		</Card>
	</div>
{/if}


