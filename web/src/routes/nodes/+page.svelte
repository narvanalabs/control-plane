<script lang="ts">
	import { nodes, type Node } from '$lib/api';
	import { Card, StatusBadge } from '$lib/components';

	let nodeList = $state<Node[]>([]);
	let loading = $state(true);

	$effect(() => {
		loadNodes();
	});

	async function loadNodes() {
		try {
			nodeList = await nodes.list();
		} catch (err) {
			console.error('Failed to load nodes:', err);
		} finally {
			loading = false;
		}
	}

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
	}

	function formatCPU(cores: number): string {
		return `${cores.toFixed(1)} cores`;
	}

	function getUsagePercent(used: number, total: number): number {
		if (total === 0) return 0;
		return Math.round(((total - used) / total) * 100);
	}

	function getUsageColor(percent: number): string {
		if (percent > 80) return 'bg-red-500';
		if (percent > 60) return 'bg-yellow-500';
		return 'bg-green-500';
	}

	function formatRelativeTime(date: string): string {
		const seconds = Math.floor((Date.now() - new Date(date).getTime()) / 1000);
		if (seconds < 60) return 'just now';
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
		return `${Math.floor(seconds / 86400)}d ago`;
	}

	const healthyCount = $derived(nodeList.filter(n => n.healthy).length);
</script>

<div class="p-8">
	<!-- Header -->
	<div class="mb-8">
		<h1 class="text-3xl font-bold mb-2">Infrastructure</h1>
		<p class="text-[var(--color-narvana-text-dim)]">Monitor your compute nodes</p>
	</div>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="w-8 h-8 border-2 border-[var(--color-narvana-primary)] border-t-transparent rounded-full animate-spin"></div>
		</div>
	{:else if nodeList.length === 0}
		<!-- Empty State -->
		<Card class="p-12 text-center">
			<div class="w-16 h-16 mx-auto rounded-xl bg-blue-500/10 flex items-center justify-center mb-4">
				<span class="text-3xl text-blue-400">⎔</span>
			</div>
			<h2 class="text-xl font-semibold mb-2">No nodes registered</h2>
			<p class="text-[var(--color-narvana-text-dim)] mb-6 max-w-md mx-auto">
				Deploy the Narvana node agent on your servers to start running workloads.
			</p>
			<div class="p-4 rounded-lg bg-[var(--color-narvana-bg)] font-mono text-sm text-left max-w-md mx-auto">
				<p class="text-[var(--color-narvana-text-muted)]"># Install and run the node agent</p>
				<p class="text-[var(--color-narvana-primary)]">./narvana-agent --control-plane your-server:50051</p>
			</div>
		</Card>
	{:else}
		<!-- Stats -->
		<div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
			<Card class="p-6">
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-blue-500/10 flex items-center justify-center">
						<span class="text-2xl text-blue-400">⎔</span>
					</div>
					<div>
						<p class="text-2xl font-bold">{nodeList.length}</p>
						<p class="text-sm text-[var(--color-narvana-text-dim)]">Total Nodes</p>
					</div>
				</div>
			</Card>

			<Card class="p-6">
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg bg-green-500/10 flex items-center justify-center">
						<span class="text-2xl text-green-400">✓</span>
					</div>
					<div>
						<p class="text-2xl font-bold">{healthyCount}</p>
						<p class="text-sm text-[var(--color-narvana-text-dim)]">Healthy</p>
					</div>
				</div>
			</Card>

			<Card class="p-6">
				<div class="flex items-center gap-4">
					<div class="w-12 h-12 rounded-lg {nodeList.length - healthyCount > 0 ? 'bg-red-500/10' : 'bg-gray-500/10'} flex items-center justify-center">
						<span class="text-2xl {nodeList.length - healthyCount > 0 ? 'text-red-400' : 'text-gray-400'}">!</span>
					</div>
					<div>
						<p class="text-2xl font-bold">{nodeList.length - healthyCount}</p>
						<p class="text-sm text-[var(--color-narvana-text-dim)]">Unhealthy</p>
					</div>
				</div>
			</Card>
		</div>

		<!-- Nodes List -->
		<div class="space-y-4">
			{#each nodeList as node, i}
				<Card class="p-6 stagger-item" style="animation-delay: {i * 50}ms">
					<div class="flex items-start justify-between mb-4">
						<div class="flex items-center gap-4">
							<div class="w-12 h-12 rounded-lg {node.healthy ? 'bg-green-500/10' : 'bg-red-500/10'} flex items-center justify-center">
								<span class="text-xl {node.healthy ? 'text-green-400' : 'text-red-400'}">⎔</span>
							</div>
							<div>
								<h3 class="text-lg font-semibold font-mono">{node.hostname}</h3>
								<p class="text-sm text-[var(--color-narvana-text-muted)]">
									{node.address}:{node.grpc_port}
								</p>
							</div>
						</div>
						<div class="flex items-center gap-3">
							<span class="text-sm text-[var(--color-narvana-text-muted)]">
								Last seen {formatRelativeTime(node.last_heartbeat)}
							</span>
							<StatusBadge status={node.healthy ? 'healthy' : 'unhealthy'} />
						</div>
					</div>

					{#if node.resources}
						<div class="grid grid-cols-1 md:grid-cols-3 gap-6 mt-4">
							<!-- CPU -->
							<div>
								<div class="flex justify-between text-sm mb-1">
									<span class="text-[var(--color-narvana-text-dim)]">CPU</span>
									<span class="font-mono">
										{formatCPU(node.resources.cpu_available)} / {formatCPU(node.resources.cpu_total)}
									</span>
								</div>
								<div class="h-2 rounded-full bg-[var(--color-narvana-bg)] overflow-hidden">
									<div 
										class="h-full rounded-full transition-all duration-500 {getUsageColor(getUsagePercent(node.resources.cpu_available, node.resources.cpu_total))}"
										style="width: {getUsagePercent(node.resources.cpu_available, node.resources.cpu_total)}%"
									></div>
								</div>
							</div>

							<!-- Memory -->
							<div>
								<div class="flex justify-between text-sm mb-1">
									<span class="text-[var(--color-narvana-text-dim)]">Memory</span>
									<span class="font-mono">
										{formatBytes(node.resources.memory_available)} / {formatBytes(node.resources.memory_total)}
									</span>
								</div>
								<div class="h-2 rounded-full bg-[var(--color-narvana-bg)] overflow-hidden">
									<div 
										class="h-full rounded-full transition-all duration-500 {getUsageColor(getUsagePercent(node.resources.memory_available, node.resources.memory_total))}"
										style="width: {getUsagePercent(node.resources.memory_available, node.resources.memory_total)}%"
									></div>
								</div>
							</div>

							<!-- Disk -->
							<div>
								<div class="flex justify-between text-sm mb-1">
									<span class="text-[var(--color-narvana-text-dim)]">Disk</span>
									<span class="font-mono">
										{formatBytes(node.resources.disk_available)} / {formatBytes(node.resources.disk_total)}
									</span>
								</div>
								<div class="h-2 rounded-full bg-[var(--color-narvana-bg)] overflow-hidden">
									<div 
										class="h-full rounded-full transition-all duration-500 {getUsageColor(getUsagePercent(node.resources.disk_available, node.resources.disk_total))}"
										style="width: {getUsagePercent(node.resources.disk_available, node.resources.disk_total)}%"
									></div>
								</div>
							</div>
						</div>
					{:else}
						<p class="text-sm text-[var(--color-narvana-text-muted)] mt-4">
							Resource information not available
						</p>
					{/if}

					{#if node.cached_paths && node.cached_paths.length > 0}
						<div class="mt-4 pt-4 border-t border-[var(--color-narvana-border)]">
							<p class="text-sm text-[var(--color-narvana-text-dim)] mb-2">Cached Nix paths: {node.cached_paths.length}</p>
							<div class="flex flex-wrap gap-2">
								{#each node.cached_paths.slice(0, 5) as path}
									<span class="px-2 py-1 rounded bg-[var(--color-narvana-bg)] text-xs font-mono text-[var(--color-narvana-text-muted)]">
										{path.split('/').pop()}
									</span>
								{/each}
								{#if node.cached_paths.length > 5}
									<span class="px-2 py-1 rounded bg-[var(--color-narvana-bg)] text-xs text-[var(--color-narvana-text-muted)]">
										+{node.cached_paths.length - 5} more
									</span>
								{/if}
							</div>
						</div>
					{/if}
				</Card>
			{/each}
		</div>
	{/if}
</div>

