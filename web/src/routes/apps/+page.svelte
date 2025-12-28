<script lang="ts">
	import { goto } from '$app/navigation';
	import { apps, deployments as deploymentsApi, type App, type Deployment } from '$lib/api';
	import { 
		Button, 
		Input, 
		Dialog, 
		EmptyState, 
		PageHeader,
		AppCard 
	} from '$lib/components';
	import { toastStore } from '$lib/stores/toast.svelte';
	import { filterApplications } from '$lib/utils/search';
	import { Plus, Search, LayoutGrid, List, Package } from 'lucide-svelte';

	/**
	 * Applications List Page
	 * Requirements: 4.1, 4.2, 4.3, 4.4
	 * 
	 * Displays applications in a card grid or list view with:
	 * - Search and filter functionality
	 * - New Application modal
	 * - Empty state for no applications
	 */

	let appList = $state<App[]>([]);
	let deploymentsByApp = $state<Record<string, Deployment[]>>({});
	let loading = $state(true);
	let searchQuery = $state('');
	let viewMode = $state<'grid' | 'list'>('grid');
	
	// New app modal state
	let showNewAppModal = $state(false);
	let newAppName = $state('');
	let newAppDescription = $state('');
	let creating = $state(false);
	let formErrors = $state<{ name?: string; description?: string }>({});

	$effect(() => {
		loadApps();
	});

	async function loadApps() {
		try {
			const loadedApps = await apps.list();
			appList = loadedApps;
			
			// Load deployments for each app to show health status
			const deploymentsMap: Record<string, Deployment[]> = {};
			await Promise.all(
				loadedApps.map(async (app) => {
					try {
						deploymentsMap[app.id] = await deploymentsApi.list(app.id);
					} catch {
						deploymentsMap[app.id] = [];
					}
				})
			);
			deploymentsByApp = deploymentsMap;
		} catch (err) {
			console.error('Failed to load apps:', err);
			toastStore.error('Failed to load applications');
		} finally {
			loading = false;
		}
	}

	/**
	 * Validate form fields
	 * Requirements: 11.3
	 */
	function validateForm(): boolean {
		const errors: { name?: string; description?: string } = {};
		
		if (!newAppName.trim()) {
			errors.name = 'Application name is required';
		} else if (newAppName.length < 2) {
			errors.name = 'Name must be at least 2 characters';
		} else if (!/^[a-zA-Z0-9-_]+$/.test(newAppName)) {
			errors.name = 'Name can only contain letters, numbers, hyphens, and underscores';
		}
		
		formErrors = errors;
		return Object.keys(errors).length === 0;
	}

	/**
	 * Create new application
	 * Requirements: 4.4, 11.4
	 */
	async function createApp(e: Event) {
		e.preventDefault();
		
		if (!validateForm()) return;
		
		creating = true;

		try {
			const app = await apps.create(newAppName.trim(), newAppDescription.trim() || undefined);
			toastStore.success('Application created', `${app.name} has been created successfully`);
			closeModal();
			goto(`/apps/${app.id}`);
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Failed to create application';
			toastStore.error('Failed to create application', message);
		} finally {
			creating = false;
		}
	}

	function openModal() {
		showNewAppModal = true;
		newAppName = '';
		newAppDescription = '';
		formErrors = {};
	}

	function closeModal() {
		showNewAppModal = false;
		newAppName = '';
		newAppDescription = '';
		formErrors = {};
	}

	// Filter applications based on search query
	// Requirements: 4.3
	const filteredApps = $derived(filterApplications(appList, searchQuery));
</script>

<!-- Page Header with New Application button -->
<!-- Requirements: 4.1, 4.4 -->
<PageHeader 
	title="Applications" 
	description="Manage your deployed applications"
>
	{#snippet actions()}
		<Button onclick={openModal}>
			<Plus class="w-4 h-4" />
			New Application
		</Button>
	{/snippet}
</PageHeader>

<div class="p-6">
	{#if loading}
		<!-- Loading state -->
		<div class="flex items-center justify-center h-64">
			<div class="w-8 h-8 border-2 border-[var(--color-primary)] border-t-transparent rounded-full animate-spin"></div>
		</div>
	{:else if appList.length === 0}
		<!-- Empty state for no applications -->
		<!-- Requirements: 4.1 -->
		<EmptyState
			icon={Package}
			title="No applications yet"
			description="Create your first application to start deploying services to your infrastructure."
		>
			{#snippet action()}
				<Button onclick={openModal}>
					<Plus class="w-4 h-4" />
					Create Application
				</Button>
			{/snippet}
		</EmptyState>
	{:else}
		<!-- Search and view toggle -->
		<!-- Requirements: 4.3 -->
		<div class="flex items-center justify-between gap-4 mb-6">
			<div class="flex-1 max-w-md">
				<Input
					type="search"
					placeholder="Search applications..."
					bind:value={searchQuery}
				>
					{#snippet icon()}
						<Search class="w-4 h-4" />
					{/snippet}
				</Input>
			</div>
			
			<!-- View mode toggle -->
			<!-- Requirements: 4.1 -->
			<div class="flex items-center gap-1 p-1 rounded-[var(--radius-md)] bg-[var(--color-background-subtle)]">
				<button
					onclick={() => viewMode = 'grid'}
					class="p-2 rounded-[var(--radius-sm)] transition-colors {viewMode === 'grid' 
						? 'bg-[var(--color-surface)] shadow-sm text-[var(--color-text)]' 
						: 'text-[var(--color-text-muted)] hover:text-[var(--color-text)]'}"
					aria-label="Grid view"
					aria-pressed={viewMode === 'grid'}
				>
					<LayoutGrid class="w-4 h-4" />
				</button>
				<button
					onclick={() => viewMode = 'list'}
					class="p-2 rounded-[var(--radius-sm)] transition-colors {viewMode === 'list' 
						? 'bg-[var(--color-surface)] shadow-sm text-[var(--color-text)]' 
						: 'text-[var(--color-text-muted)] hover:text-[var(--color-text)]'}"
					aria-label="List view"
					aria-pressed={viewMode === 'list'}
				>
					<List class="w-4 h-4" />
				</button>
			</div>
		</div>

		{#if filteredApps.length === 0}
			<!-- No search results -->
			<EmptyState
				icon={Search}
				title="No applications found"
				description="No applications match your search. Try a different search term."
			>
				{#snippet action()}
					<Button variant="secondary" onclick={() => searchQuery = ''}>
						Clear search
					</Button>
				{/snippet}
			</EmptyState>
		{:else if viewMode === 'grid'}
			<!-- Card grid view -->
			<!-- Requirements: 4.1, 4.2 -->
			<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
				{#each filteredApps as app (app.id)}
					<AppCard 
						{app} 
						deployments={deploymentsByApp[app.id] ?? []}
					/>
				{/each}
			</div>
		{:else}
			<!-- List view -->
			<!-- Requirements: 4.1 -->
			<div class="space-y-3">
				{#each filteredApps as app (app.id)}
					<AppCard 
						{app} 
						deployments={deploymentsByApp[app.id] ?? []}
						class="max-w-none"
					/>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<!-- New Application Modal -->
<!-- Requirements: 4.4, 11.3, 11.4 -->
<Dialog
	bind:open={showNewAppModal}
	title="Create New Application"
	description="Create a new application to organize and deploy your services."
>
	<form onsubmit={createApp} class="space-y-4">
		<Input
			label="Application Name"
			placeholder="my-awesome-app"
			bind:value={newAppName}
			error={formErrors.name}
			required
		/>

		<Input
			label="Description"
			placeholder="Optional description for your application"
			bind:value={newAppDescription}
			error={formErrors.description}
		/>
	</form>

	{#snippet footer()}
		<Button variant="ghost" onclick={closeModal} disabled={creating}>
			Cancel
		</Button>
		<Button onclick={() => { const form = document.querySelector('form'); if (form) form.requestSubmit(); }} loading={creating}>
			Create Application
		</Button>
	{/snippet}
</Dialog>
