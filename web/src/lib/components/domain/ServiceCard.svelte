<script lang="ts">
	import type { ServiceConfig, Deployment } from '$lib/api';
	import { Card, Button, StatusBadge } from '$lib/components';
	import { Play, Edit, Eye, Trash2, GitBranch, Box, Cpu } from 'lucide-svelte';

	/**
	 * ServiceCard Domain Component
	 * Requirements: 5.5, 6.1, 6.3, 6.4
	 * 
	 * Displays service card with:
	 * - Service name, status, resource tier
	 * - Quick action buttons (Deploy, Edit, Preview)
	 * - Git repo and branch info
	 */
	interface Props {
		service: ServiceConfig;
		deployment?: Deployment;
		onDeploy?: () => void;
		onEdit?: () => void;
		onPreview?: () => void;
		onDelete?: () => void;
		class?: string;
	}

	let { 
		service, 
		deployment, 
		onDeploy, 
		onEdit, 
		onPreview, 
		onDelete,
		class: className = '' 
	}: Props = $props();

	// Get resource tier display info
	function getResourceTierInfo(tier: string): { label: string; memory: string } {
		const tiers: Record<string, { label: string; memory: string }> = {
			nano: { label: 'Nano', memory: '256MB' },
			small: { label: 'Small', memory: '512MB' },
			medium: { label: 'Medium', memory: '1GB' },
			large: { label: 'Large', memory: '2GB' },
			xlarge: { label: 'XLarge', memory: '4GB' },
		};
		return tiers[tier] || { label: tier, memory: 'Unknown' };
	}

	// Get source display
	function getSourceDisplay(service: ServiceConfig): string {
		if (service.git_repo) return service.git_repo;
		if (service.flake_uri) return service.flake_uri;
		if (service.image) return service.image;
		return 'No source configured';
	}

	const tierInfo = $derived(getResourceTierInfo(service.resource_tier));
	const sourceDisplay = $derived(getSourceDisplay(service));
</script>

<Card padding="lg" class={className} data-service-card data-service-name={service.name}>
	<div class="flex items-start justify-between">
		<!-- Service info -->
		<div class="flex items-start gap-4">
			<div 
				class="w-10 h-10 rounded-[var(--radius-lg)] bg-[var(--color-info-light)] flex items-center justify-center text-[var(--color-info)]"
				data-service-icon
			>
				<Box class="w-5 h-5" />
			</div>
			<div>
				<div class="flex items-center gap-2 mb-1">
					<h3 class="font-semibold text-[var(--color-text)]" data-service-name-text>{service.name}</h3>
					{#if deployment}
						<StatusBadge status={deployment.status} size="sm" />
					{/if}
				</div>
				<p class="text-sm text-[var(--color-text-muted)] font-mono truncate max-w-md" data-service-source>
					{sourceDisplay}
				</p>
			</div>
		</div>

		<!-- Action buttons -->
		<div class="flex items-center gap-2" data-service-actions>
			{#if onPreview}
				<Button size="sm" variant="ghost" onclick={onPreview} title="Preview build">
					<Eye class="w-4 h-4" />
				</Button>
			{/if}
			{#if onEdit}
				<Button size="sm" variant="ghost" onclick={onEdit} title="Edit service">
					<Edit class="w-4 h-4" />
				</Button>
			{/if}
			{#if onDeploy}
				<Button size="sm" variant="secondary" onclick={onDeploy} title="Deploy service">
					<Play class="w-4 h-4" />
					Deploy
				</Button>
			{/if}
			{#if onDelete}
				<Button size="sm" variant="ghost" onclick={onDelete} title="Delete service">
					<Trash2 class="w-4 h-4" />
				</Button>
			{/if}
		</div>
	</div>

	<!-- Service details grid -->
	<div class="mt-4 grid grid-cols-2 md:grid-cols-4 gap-4 text-sm" data-service-details>
		<div>
			<span class="text-[var(--color-text-muted)] block mb-0.5">Resource Tier</span>
			<div class="flex items-center gap-1.5">
				<Cpu class="w-3.5 h-3.5 text-[var(--color-text-secondary)]" />
				<span class="font-medium text-[var(--color-text)]" data-resource-tier>{tierInfo.label}</span>
				<span class="text-[var(--color-text-muted)]">({tierInfo.memory})</span>
			</div>
		</div>
		<div>
			<span class="text-[var(--color-text-muted)] block mb-0.5">Replicas</span>
			<p class="font-medium text-[var(--color-text)]" data-replicas>{service.replicas}</p>
		</div>
		<div>
			<span class="text-[var(--color-text-muted)] block mb-0.5">Git Ref</span>
			<div class="flex items-center gap-1.5">
				<GitBranch class="w-3.5 h-3.5 text-[var(--color-text-secondary)]" />
				<span class="font-medium font-mono text-[var(--color-text)]" data-git-ref>{service.git_ref || 'main'}</span>
			</div>
		</div>
		<div>
			<span class="text-[var(--color-text-muted)] block mb-0.5">Build Strategy</span>
			<p class="font-medium text-[var(--color-text)]" data-build-strategy>{service.build_strategy || 'flake'}</p>
		</div>
	</div>

	<!-- Port badges -->
	{#if service.ports && service.ports.length > 0}
		<div class="mt-3 flex gap-2" data-service-ports>
			{#each service.ports as port}
				<span class="px-2 py-1 rounded-[var(--radius-sm)] bg-[var(--color-background-subtle)] text-xs font-mono text-[var(--color-text-secondary)]">
					:{port.container_port}/{port.protocol || 'tcp'}
				</span>
			{/each}
		</div>
	{/if}
</Card>
