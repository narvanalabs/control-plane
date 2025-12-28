<script lang="ts">
	import { apps, nodes, deployments, type App, type Node, type Deployment } from '$lib/api';
	import { Card, StatusBadge, Button } from '$lib/components';

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
	const runningDeployments = $derived(recentDeployments.filter(d => d.status === 'running').length);
	const healthyNodes = $derived(nodeList.filter(n => n.healthy).length);
	const totalServices = $derived(appList.reduce((sum, app) => sum + (app.services?.length ?? 0), 0));

	function formatRelativeTime(date: string): string {
		const now = new Date();
		const then = new Date(date);
		const seconds = Math.floor((now.getTime() - then.getTime()) / 1000);

		if (seconds < 60) return 'just now';
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
		return `${Math.floor(seconds / 86400)}d ago`;
	}

	function getAppName(appId: string): string {
		const app = appList.find(a => a.id === appId);
		return app?.name || 'Unknown';
	}
</script>

<div class="p-8">
	<!-- Header -->
	<div class="mb-8">
		<h1 class="text-3xl font-bold mb-2">Dashboard</h1>
		<p class="text-[var(--color-narvana-text-dim)]">Overview of your infrastructure</p>
	</div>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="w-8 h-8 border-2 border-[var(--color-narvana-primary)] border-t-transparent rounded-full animate-spin"></div>
		</div>
	{:else}
		<!-- Stats Grid -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
			<Card class="p-6 stagger-item">
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-[var(--color-narvana-primary)]/10 flex items-center justify-center">
						<span class="text-2xl text-[var(--color-narvana-primary)]">⬡</span>
					</div>
					<div>
						<p class="text-2xl font-bold">{appList.length}</p>
						<p class="text-sm text-[var(--color-narvana-text-dim)]">Applications</p>
					</div>
				</div>
			</Card>

			<Card class="p-6 stagger-item">
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-[var(--color-narvana-secondary)]/10 flex items-center justify-center">
						<span class="text-2xl text-[var(--color-narvana-secondary)]">◇</span>
					</div>
					<div>
						<p class="text-2xl font-bold">{totalServices}</p>
						<p class="text-sm text-[var(--color-narvana-text-dim)]">Services</p>
					</div>
				</div>
			</Card>

			<Card class="p-6 stagger-item">
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-green-500/10 flex items-center justify-center">
						<span class="text-2xl text-green-400">▶</span>
					</div>
					<div>
						<p class="text-2xl font-bold">{runningDeployments}</p>
						<p class="text-sm text-[var(--color-narvana-text-dim)]">Running</p>
					</div>
				</div>
			</Card>

			<Card class="p-6 stagger-item">
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-blue-500/10 flex items-center justify-center">
						<span class="text-2xl text-blue-400">⎔</span>
					</div>
					<div>
						<p class="text-2xl font-bold">{healthyNodes}/{nodeList.length}</p>
						<p class="text-sm text-[var(--color-narvana-text-dim)]">Nodes Healthy</p>
					</div>
				</div>
			</Card>
		</div>

		<div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
			<!-- Recent Deployments -->
			<Card class="p-6">
				<div class="flex items-center justify-between mb-4">
					<h2 class="text-lg font-semibold">Recent Deployments</h2>
					<a href="/apps" class="text-sm text-[var(--color-narvana-primary)] hover:underline">View all</a>
				</div>

				{#if recentDeployments.length === 0}
					<div class="text-center py-8 text-[var(--color-narvana-text-dim)]">
						<p>No deployments yet</p>
						<a href="/apps" class="text-[var(--color-narvana-primary)] hover:underline text-sm">Create your first app</a>
					</div>
				{:else}
					<div class="space-y-3">
						{#each recentDeployments.slice(0, 5) as deployment}
							<a 
								href="/apps/{deployment.app_id}"
								class="flex items-center justify-between p-3 rounded-lg hover:bg-[var(--color-narvana-surface-hover)] transition-colors"
							>
								<div class="flex items-center gap-3 min-w-0">
									<div class="w-8 h-8 rounded-lg bg-[var(--color-narvana-border)] flex items-center justify-center text-sm font-mono">
										{deployment.service_name.charAt(0).toUpperCase()}
									</div>
									<div class="min-w-0">
										<p class="font-medium truncate">{getAppName(deployment.app_id)}</p>
										<p class="text-sm text-[var(--color-narvana-text-muted)] truncate">
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

			<!-- Nodes -->
			<Card class="p-6">
				<div class="flex items-center justify-between mb-4">
					<h2 class="text-lg font-semibold">Infrastructure</h2>
					<a href="/nodes" class="text-sm text-[var(--color-narvana-primary)] hover:underline">View all</a>
				</div>

				{#if nodeList.length === 0}
					<div class="text-center py-8 text-[var(--color-narvana-text-dim)]">
						<p>No nodes registered</p>
						<p class="text-sm mt-1">Deploy a node agent to get started</p>
					</div>
				{:else}
					<div class="space-y-3">
						{#each nodeList as node}
							<div class="flex items-center justify-between p-3 rounded-lg bg-[var(--color-narvana-bg)]">
								<div class="flex items-center gap-3">
									<div class="w-8 h-8 rounded-lg {node.healthy ? 'bg-green-500/10' : 'bg-red-500/10'} flex items-center justify-center">
										<span class="{node.healthy ? 'text-green-400' : 'text-red-400'}">⎔</span>
									</div>
									<div>
										<p class="font-medium font-mono text-sm">{node.hostname}</p>
										<p class="text-xs text-[var(--color-narvana-text-muted)]">{node.address}</p>
									</div>
								</div>
								<StatusBadge status={node.healthy ? 'healthy' : 'unhealthy'} size="sm" />
							</div>
						{/each}
					</div>
				{/if}
			</Card>
		</div>

		<!-- Quick Actions -->
		<Card class="p-6 mt-6">
			<h2 class="text-lg font-semibold mb-4">Quick Actions</h2>
			<div class="flex flex-wrap gap-3">
				<Button onclick={() => window.location.href = '/apps/new'}>
					<span>+</span> New Application
				</Button>
				<Button variant="secondary" onclick={() => window.location.href = '/nodes'}>
					<span>⎔</span> View Nodes
				</Button>
			</div>
		</Card>
	{/if}
</div>


