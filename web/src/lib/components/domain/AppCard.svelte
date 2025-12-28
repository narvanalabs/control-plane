<script lang="ts">
	import type { App, Deployment, DeploymentStatus } from '$lib/api';
	import { Card, StatusBadge } from '$lib/components';
	import { formatRelativeTime } from '$lib/utils/formatters';
	import { Layers, Clock, GitBranch } from 'lucide-svelte';

	/**
	 * AppCard Domain Component
	 * Requirements: 4.2, 4.5
	 * 
	 * Displays application card with:
	 * - App name, description, service count
	 * - Health status indicator
	 * - Last deployment info
	 * - Hover state for clickable card
	 */
	interface Props {
		app: App;
		deployments?: Deployment[];
		class?: string;
	}

	let { app, deployments = [], class: className = '' }: Props = $props();

	/**
	 * Calculate application health based on service deployment statuses
	 * Requirements: 4.5
	 * 
	 * - "healthy": All services have running deployments
	 * - "partial": Some services are running
	 * - "failed": None are running or any have failed status
	 * - "unknown": No deployments exist
	 */
	function getAppHealth(app: App, deployments: Deployment[]): 'healthy' | 'unhealthy' | 'unknown' {
		const serviceCount = app.services?.length ?? 0;
		if (serviceCount === 0) return 'unknown';

		// Get latest deployment for each service
		const latestByService = new Map<string, Deployment>();
		for (const deployment of deployments) {
			const existing = latestByService.get(deployment.service_name);
			if (!existing || new Date(deployment.created_at) > new Date(existing.created_at)) {
				latestByService.set(deployment.service_name, deployment);
			}
		}

		if (latestByService.size === 0) return 'unknown';

		const statuses = Array.from(latestByService.values()).map(d => d.status);
		const runningCount = statuses.filter(s => s === 'running').length;
		const failedCount = statuses.filter(s => s === 'failed').length;

		if (failedCount > 0) return 'unhealthy';
		if (runningCount === serviceCount) return 'healthy';
		if (runningCount > 0) return 'unhealthy'; // partial = unhealthy for display
		return 'unknown';
	}

	/**
	 * Get the most recent deployment across all services
	 */
	function getLatestDeployment(deployments: Deployment[]): Deployment | null {
		if (deployments.length === 0) return null;
		return deployments.reduce((latest, current) => 
			new Date(current.created_at) > new Date(latest.created_at) ? current : latest
		);
	}

	const health = $derived(getAppHealth(app, deployments));
	const latestDeployment = $derived(getLatestDeployment(deployments));
	const serviceCount = $derived(app.services?.length ?? 0);
</script>

<a href="/apps/{app.id}" class="block {className}" data-app-card>
	<Card hover padding="lg" class="h-full">
		<div class="flex flex-col h-full">
			<!-- Header with icon and health status -->
			<div class="flex items-start justify-between mb-4">
				<div 
					class="w-12 h-12 rounded-[var(--radius-lg)] bg-[var(--color-background-subtle)] flex items-center justify-center"
					data-app-icon
				>
					<span class="text-xl font-bold text-[var(--color-primary)]">
						{app.name.charAt(0).toUpperCase()}
					</span>
				</div>
				{#if health !== 'unknown'}
					<StatusBadge status={health} size="sm" />
				{/if}
			</div>

			<!-- App name and description -->
			<h3 class="text-lg font-semibold text-[var(--color-text)] mb-1" data-app-name>
				{app.name}
			</h3>
			{#if app.description}
				<p 
					class="text-sm text-[var(--color-text-secondary)] mb-4 line-clamp-2"
					data-app-description
				>
					{app.description}
				</p>
			{:else}
				<p 
					class="text-sm text-[var(--color-text-muted)] mb-4 italic"
					data-app-description
				>
					No description
				</p>
			{/if}

			<!-- Spacer to push footer to bottom -->
			<div class="flex-1"></div>

			<!-- Footer with service count and last deployment -->
			<div class="flex items-center justify-between text-sm text-[var(--color-text-secondary)] pt-4 border-t border-[var(--color-border)]">
				<div class="flex items-center gap-1" data-service-count>
					<Layers class="w-4 h-4" />
					<span>{serviceCount} service{serviceCount !== 1 ? 's' : ''}</span>
				</div>
				
				{#if latestDeployment}
					<div class="flex items-center gap-1" data-last-deployment>
						<Clock class="w-4 h-4" />
						<span>{formatRelativeTime(latestDeployment.created_at)}</span>
					</div>
				{:else}
					<div class="flex items-center gap-1 text-[var(--color-text-muted)]" data-last-deployment>
						<Clock class="w-4 h-4" />
						<span>No deployments</span>
					</div>
				{/if}
			</div>
		</div>
	</Card>
</a>
