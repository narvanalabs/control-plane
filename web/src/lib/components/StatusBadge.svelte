<script lang="ts">
	import type { DeploymentStatus } from '$lib/api';

	interface Props {
		status: DeploymentStatus | 'healthy' | 'unhealthy' | 'unknown';
		size?: 'sm' | 'md';
	}

	let { status, size = 'md' }: Props = $props();

	const statusConfig: Record<string, { color: string; bg: string; label: string; pulse?: boolean }> = {
		// Deployment statuses
		pending: { color: 'text-yellow-400', bg: 'bg-yellow-400/10', label: 'Pending' },
		building: { color: 'text-blue-400', bg: 'bg-blue-400/10', label: 'Building', pulse: true },
		built: { color: 'text-cyan-400', bg: 'bg-cyan-400/10', label: 'Built' },
		scheduled: { color: 'text-purple-400', bg: 'bg-purple-400/10', label: 'Scheduled' },
		starting: { color: 'text-blue-400', bg: 'bg-blue-400/10', label: 'Starting', pulse: true },
		running: { color: 'text-green-400', bg: 'bg-green-400/10', label: 'Running' },
		stopping: { color: 'text-orange-400', bg: 'bg-orange-400/10', label: 'Stopping' },
		stopped: { color: 'text-gray-400', bg: 'bg-gray-400/10', label: 'Stopped' },
		failed: { color: 'text-red-400', bg: 'bg-red-400/10', label: 'Failed' },
		// Node statuses
		healthy: { color: 'text-green-400', bg: 'bg-green-400/10', label: 'Healthy' },
		unhealthy: { color: 'text-red-400', bg: 'bg-red-400/10', label: 'Unhealthy' },
		unknown: { color: 'text-gray-400', bg: 'bg-gray-400/10', label: 'Unknown' },
	};

	const config = $derived(statusConfig[status] || statusConfig.unknown);
	const sizeClasses = $derived(size === 'sm' ? 'px-2 py-0.5 text-xs' : 'px-3 py-1 text-sm');
</script>

<span class="inline-flex items-center gap-1.5 rounded-full font-medium {config.color} {config.bg} {sizeClasses}">
	<span class="w-1.5 h-1.5 rounded-full {config.color.replace('text-', 'bg-')} {config.pulse ? 'animate-pulse' : ''}"></span>
	{config.label}
</span>



