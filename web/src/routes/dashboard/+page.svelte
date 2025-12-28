<script lang="ts">
	import { apps, nodes, deployments, type App, type Node, type Deployment } from '$lib/api';
	import { Card, StatusBadge, Button, EmptyState, ResourceBar } from '$lib/components';
	import { PageHeader } from '$lib/components/layout';
	import { formatRelativeTime } from '$lib/utils/formatters';
	import { 
		LayoutGrid, 
		Server, 
		Rocket, 
		Activity, 
		Plus, 
		ArrowRight,
		Cpu,
		HardDrive,
		MemoryStick
	} from 'lucide-svelte';

	let appList = $state<App[]>([]);
	let nodeList = $state<Node[]>([]);
	let recentDeployments = $state<Deployment[]>([]);
	let loading = $state(true);

	$effect(() => {
		loadDashboardData();
	});

	async function loadDashboardData() {
		try {
			const [appsResult, nodesResult] = await Promise.all([
				apps.list(),
				nodes.list(),
			]);
			appList = appsResult;
			nodeList = nodesResult;

			// Get recent deployments from first few apps
			const deploymentsPromises = appsResult.slice(0, 5).map(app => 
				deployments.list(app.id).catch(() => [])
			);
			const deploymentsResults = await Promise.all(deploymentsPromises);
			recentDeployments = deploymentsResults
				.flat()
				.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
				.slice(0, 10);
		} catch (err) {
			console.error('Failed to load dashboard data:', err);
		} finally {
			loading = false;
		}
	}

	// Computed stats
	const totalApplications = $derived(appList.length);
	const totalServices = $derived(appList.reduce((sum, app) => sum + (app.services?.length ?? 0), 0));
	const runningDeployments = $derived(recentDeployments.filter(d => d.status === 'running').length);
	const totalNodes = $derived(nodeList.length);
	const healthyNodes = $derived(nodeList.filter(n => n.healthy).length);

	// Aggregate resource utilization from all nodes
	const aggregatedResources = $derived(() => {
		const totals = {
			cpuTotal: 0,
			cpuAvailable: 0,
			memoryTotal: 0,
			memoryAvailable: 0,
			diskTotal: 0,
			diskAvailable: 0,
		};
		
		for (const node of nodeList) {
			if (node.resources) {
				totals.cpuTotal += node.resources.cpu_total;
				totals.cpuAvailable += node.resources.cpu_available;
				totals.memoryTotal += node.resources.memory_total;
				totals.memoryAvailable += node.resources.memory_available;
				totals.diskTotal += node.resources.disk_total;
				totals.diskAvailable += node.resources.disk_available;
			}
		}
		
		return totals;
	});

	// Check if user is new (no apps)
	const isNewUser = $derived(!loading && appList.length === 0);

	function getAppName(appId: string): string {
		const app = appList.find(a => a.id === appId);
		return app?.name || 'Unknown';
	}

	function navigateTo(path: string) {
		window.location.href = path;
	}
</script>


<!-- Page Header with quick actions -->
<PageHeader 
	title="Dashboard" 
	description="Overview of your infrastructure and deployments"
>
	{#snippet actions()}
		<Button variant="outline" size="sm" onclick={() => navigateTo('/nodes')}>
			<Server class="w-4 h-4" />
			View Nodes
		</Button>
		<Button size="sm" onclick={() => navigateTo('/apps/new')}>
			<Plus class="w-4 h-4" />
			New Application
		</Button>
	{/snippet}
</PageHeader>

<div class="p-6">
	{#if loading}
		<!-- Loading skeleton -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
			{#each Array(4) as _}
				<Card padding="md">
					<div class="animate-pulse">
						<div class="flex items-center gap-4">
							<div class="w-12 h-12 rounded-lg bg-[var(--color-surface-hover)]"></div>
							<div class="flex-1">
								<div class="h-6 w-12 bg-[var(--color-surface-hover)] rounded mb-2"></div>
								<div class="h-4 w-20 bg-[var(--color-surface-hover)] rounded"></div>
							</div>
						</div>
					</div>
				</Card>
			{/each}
		</div>
	{:else if isNewUser}
		<!-- Empty state for new users -->
		<EmptyState
			icon={Rocket}
			title="Welcome to Narvana"
			description="Get started by creating your first application. Deploy containers, Nix packages, or any service with ease."
		>
			{#snippet action()}
				<Button onclick={() => navigateTo('/apps/new')}>
					<Plus class="w-4 h-4" />
					Create Your First Application
				</Button>
			{/snippet}
		</EmptyState>
	{:else}
		<!-- Statistics Cards -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6" data-testid="stats-grid">
			<!-- Applications -->
			<Card padding="md" hover onclick={() => navigateTo('/apps')}>
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-[var(--color-primary)]/10 flex items-center justify-center">
						<LayoutGrid class="w-6 h-6 text-[var(--color-primary)]" />
					</div>
					<div>
						<p class="text-2xl font-bold text-[var(--color-text)]" data-testid="stat-applications">{totalApplications}</p>
						<p class="text-sm text-[var(--color-text-secondary)]">Applications</p>
					</div>
				</div>
			</Card>

			<!-- Services -->
			<Card padding="md" hover onclick={() => navigateTo('/apps')}>
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-[var(--color-info)]/10 flex items-center justify-center">
						<Activity class="w-6 h-6 text-[var(--color-info)]" />
					</div>
					<div>
						<p class="text-2xl font-bold text-[var(--color-text)]" data-testid="stat-services">{totalServices}</p>
						<p class="text-sm text-[var(--color-text-secondary)]">Services</p>
					</div>
				</div>
			</Card>

			<!-- Running Deployments -->
			<Card padding="md" hover onclick={() => navigateTo('/apps')}>
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-[var(--color-success)]/10 flex items-center justify-center">
						<Rocket class="w-6 h-6 text-[var(--color-success)]" />
					</div>
					<div>
						<p class="text-2xl font-bold text-[var(--color-text)]" data-testid="stat-deployments">{runningDeployments}</p>
						<p class="text-sm text-[var(--color-text-secondary)]">Running</p>
					</div>
				</div>
			</Card>

			<!-- Nodes -->
			<Card padding="md" hover onclick={() => navigateTo('/nodes')}>
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-[var(--color-warning)]/10 flex items-center justify-center">
						<Server class="w-6 h-6 text-[var(--color-warning)]" />
					</div>
					<div>
						<p class="text-2xl font-bold text-[var(--color-text)]" data-testid="stat-nodes">
							{healthyNodes}<span class="text-[var(--color-text-muted)]">/{totalNodes}</span>
						</p>
						<p class="text-sm text-[var(--color-text-secondary)]">Nodes Healthy</p>
					</div>
				</div>
			</Card>
		</div>

		<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
			<!-- Recent Activity Feed -->
			<div class="lg:col-span-2">
				<Card padding="none">
					{#snippet header()}
						<div class="flex items-center justify-between">
							<h2 class="text-lg font-semibold text-[var(--color-text)]">Recent Activity</h2>
							<Button variant="ghost" size="sm" onclick={() => navigateTo('/apps')}>
								View all
								<ArrowRight class="w-4 h-4" />
							</Button>
						</div>
					{/snippet}
					
					{#if recentDeployments.length === 0}
						<div class="py-12 text-center">
							<Activity class="w-8 h-8 mx-auto mb-3 text-[var(--color-text-muted)]" />
							<p class="text-[var(--color-text-secondary)]">No recent deployments</p>
							<p class="text-sm text-[var(--color-text-muted)] mt-1">Deploy a service to see activity here</p>
						</div>
					{:else}
						<div class="divide-y divide-[var(--color-border)]">
							{#each recentDeployments.slice(0, 5) as deployment}
								<a 
									href="/apps/{deployment.app_id}"
									class="flex items-center justify-between p-4 hover:bg-[var(--color-surface-hover)] transition-colors"
								>
									<div class="flex items-center gap-3 min-w-0">
										<div class="w-10 h-10 rounded-lg bg-[var(--color-background-subtle)] flex items-center justify-center text-sm font-medium text-[var(--color-text-secondary)]">
											{deployment.service_name.charAt(0).toUpperCase()}
										</div>
										<div class="min-w-0">
											<p class="font-medium text-[var(--color-text)] truncate">{getAppName(deployment.app_id)}</p>
											<p class="text-sm text-[var(--color-text-muted)] truncate">
												{deployment.service_name} • {formatRelativeTime(deployment.created_at)}
											</p>
										</div>
									</div>
									<StatusBadge status={deployment.status} size="sm" />
								</a>
							{/each}
						</div>
					{/if}
				</Card>
			</div>

			<!-- Resource Utilization & Infrastructure -->
			<div class="space-y-6">
				<!-- Resource Utilization -->
				{#if nodeList.length > 0 && nodeList.some(n => n.resources)}
					<Card padding="md">
						{#snippet header()}
							<h2 class="text-lg font-semibold text-[var(--color-text)]">Resource Utilization</h2>
						{/snippet}
						
						<div class="space-y-4">
							<ResourceBar 
								label="CPU"
								total={aggregatedResources().cpuTotal}
								available={aggregatedResources().cpuAvailable}
								unit="m"
							/>
							<ResourceBar 
								label="Memory"
								total={Math.round(aggregatedResources().memoryTotal / (1024 * 1024))}
								available={Math.round(aggregatedResources().memoryAvailable / (1024 * 1024))}
								unit=" MB"
							/>
							<ResourceBar 
								label="Disk"
								total={Math.round(aggregatedResources().diskTotal / (1024 * 1024 * 1024))}
								available={Math.round(aggregatedResources().diskAvailable / (1024 * 1024 * 1024))}
								unit=" GB"
							/>
						</div>
					</Card>
				{/if}

				<!-- Infrastructure Nodes -->
				<Card padding="none">
					{#snippet header()}
						<div class="flex items-center justify-between">
							<h2 class="text-lg font-semibold text-[var(--color-text)]">Infrastructure</h2>
							<Button variant="ghost" size="sm" onclick={() => navigateTo('/nodes')}>
								View all
								<ArrowRight class="w-4 h-4" />
							</Button>
						</div>
					{/snippet}
					
					{#if nodeList.length === 0}
						<div class="py-12 text-center">
							<Server class="w-8 h-8 mx-auto mb-3 text-[var(--color-text-muted)]" />
							<p class="text-[var(--color-text-secondary)]">No nodes registered</p>
							<p class="text-sm text-[var(--color-text-muted)] mt-1">Deploy a node agent to get started</p>
						</div>
					{:else}
						<div class="divide-y divide-[var(--color-border)]">
							{#each nodeList.slice(0, 4) as node}
								<div class="flex items-center justify-between p-4">
									<div class="flex items-center gap-3">
										<div class="w-10 h-10 rounded-lg {node.healthy ? 'bg-[var(--color-success)]/10' : 'bg-[var(--color-error)]/10'} flex items-center justify-center">
											<Server class="w-5 h-5 {node.healthy ? 'text-[var(--color-success)]' : 'text-[var(--color-error)]'}" />
										</div>
										<div>
											<p class="font-medium font-mono text-sm text-[var(--color-text)]">{node.hostname}</p>
											<p class="text-xs text-[var(--color-text-muted)]">{node.address}</p>
										</div>
									</div>
									<StatusBadge status={node.healthy ? 'healthy' : 'unhealthy'} size="sm" />
								</div>
							{/each}
						</div>
					{/if}
				</Card>
			</div>
		</div>
	{/if}
</div>
