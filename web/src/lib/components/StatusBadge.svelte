<script lang="ts">
	import type { DeploymentStatus } from '$lib/api';

	/**
	 * StatusBadge Component
	 * Requirements: 16.4
	 * 
	 * Provides StatusBadge for deployment/node statuses with appropriate
	 * color coding and optional pulse animation for active states.
	 */
	interface Props {
		status: DeploymentStatus | 'healthy' | 'unhealthy' | 'unknown';
		size?: 'sm' | 'md';
	}

	let { status, size = 'md' }: Props = $props();

	// Status configuration using design system tokens
	const statusConfig: Record<string, { bg: string; text: string; dot: string; label: string; pulse?: boolean }> = {
		// Deployment statuses
		pending: { 
			bg: 'bg-[var(--color-warning-light)]', 
			text: 'text-[var(--color-warning-foreground)]', 
			dot: 'bg-[var(--color-warning)]',
			label: 'Pending' 
		},
		building: { 
			bg: 'bg-[var(--color-info-light)]', 
			text: 'text-[var(--color-info-foreground)]', 
			dot: 'bg-[var(--color-info)]',
			label: 'Building', 
			pulse: true 
		},
		built: { 
			bg: 'bg-[var(--color-info-light)]', 
			text: 'text-[var(--color-info-foreground)]', 
			dot: 'bg-[var(--color-info)]',
			label: 'Built' 
		},
		scheduled: { 
			bg: 'bg-[var(--color-info-light)]', 
			text: 'text-[var(--color-info-foreground)]', 
			dot: 'bg-[var(--color-info)]',
			label: 'Scheduled' 
		},
		starting: { 
			bg: 'bg-[var(--color-info-light)]', 
			text: 'text-[var(--color-info-foreground)]', 
			dot: 'bg-[var(--color-info)]',
			label: 'Starting', 
			pulse: true 
		},
		running: { 
			bg: 'bg-[var(--color-success-light)]', 
			text: 'text-[var(--color-success-foreground)]', 
			dot: 'bg-[var(--color-success)]',
			label: 'Running' 
		},
		stopping: { 
			bg: 'bg-[var(--color-warning-light)]', 
			text: 'text-[var(--color-warning-foreground)]', 
			dot: 'bg-[var(--color-warning)]',
			label: 'Stopping' 
		},
		stopped: { 
			bg: 'bg-[var(--color-secondary)]', 
			text: 'text-[var(--color-text-secondary)]', 
			dot: 'bg-[var(--color-text-muted)]',
			label: 'Stopped' 
		},
		failed: { 
			bg: 'bg-[var(--color-error-light)]', 
			text: 'text-[var(--color-error-foreground)]', 
			dot: 'bg-[var(--color-error)]',
			label: 'Failed' 
		},
		// Node statuses
		healthy: { 
			bg: 'bg-[var(--color-success-light)]', 
			text: 'text-[var(--color-success-foreground)]', 
			dot: 'bg-[var(--color-success)]',
			label: 'Healthy' 
		},
		unhealthy: { 
			bg: 'bg-[var(--color-error-light)]', 
			text: 'text-[var(--color-error-foreground)]', 
			dot: 'bg-[var(--color-error)]',
			label: 'Unhealthy' 
		},
		unknown: { 
			bg: 'bg-[var(--color-secondary)]', 
			text: 'text-[var(--color-text-secondary)]', 
			dot: 'bg-[var(--color-text-muted)]',
			label: 'Unknown' 
		},
	};

	// Size classes
	const sizeClasses: Record<string, string> = {
		sm: 'px-[var(--spacing-2)] py-[var(--spacing-0-5)] text-[var(--text-xs)]',
		md: 'px-[var(--spacing-2-5)] py-[var(--spacing-1)] text-[var(--text-sm)]',
	};

	// Dot size classes
	const dotSizeClasses: Record<string, string> = {
		sm: 'w-1.5 h-1.5',
		md: 'w-2 h-2',
	};

	const config = $derived(statusConfig[status] || statusConfig.unknown);
</script>

<span 
	class="inline-flex items-center gap-[var(--spacing-1-5)] rounded-[var(--radius-full)] font-medium {config.bg} {config.text} {sizeClasses[size]}"
	data-status-badge
	data-status={status}
	data-size={size}
>
	<span 
		class="rounded-full {config.dot} {dotSizeClasses[size]} {config.pulse ? 'animate-pulse' : ''}" 
		aria-hidden="true"
		data-status-dot
	></span>
	{config.label}
</span>




