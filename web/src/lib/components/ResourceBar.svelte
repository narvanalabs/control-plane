<script lang="ts">
	/**
	 * ResourceBar Component
	 * Requirements: 10.2
	 * 
	 * Displays resource utilization as a progress bar with percentage.
	 * Color coding based on utilization level:
	 * - Green (0-60%): Normal utilization
	 * - Yellow (60-80%): Warning level
	 * - Red (80-100%): Critical level
	 */
	interface Props {
		label: string;
		total: number;
		available: number;
		unit?: string;
		showPercentage?: boolean;
		class?: string;
	}

	let { 
		label,
		total,
		available,
		unit = '',
		showPercentage = true,
		class: className = '',
	}: Props = $props();

	// Calculate utilization percentage: ((total - available) / total) * 100
	const used = $derived(Math.max(0, total - available));
	const percentage = $derived(total > 0 ? Math.round((used / total) * 100) : 0);
	
	// Clamp percentage for display (0-100)
	const displayPercentage = $derived(Math.min(100, Math.max(0, percentage)));

	// Color coding based on utilization level
	const barColor = $derived(() => {
		if (percentage >= 80) {
			return 'bg-[var(--color-error)]';
		} else if (percentage >= 60) {
			return 'bg-[var(--color-warning)]';
		} else {
			return 'bg-[var(--color-success)]';
		}
	});

	// Format value with unit
	function formatValue(value: number): string {
		if (unit) {
			return `${value}${unit}`;
		}
		return String(value);
	}
</script>

<div class="space-y-[var(--spacing-1)] {className}" data-resource-bar>
	<div class="flex items-center justify-between text-[var(--text-sm)]">
		<span class="font-medium text-[var(--color-text)]" data-resource-label>{label}</span>
		<span class="text-[var(--color-text-secondary)]" data-resource-value>
			{formatValue(used)} / {formatValue(total)}
			{#if showPercentage}
				<span class="ml-[var(--spacing-1)]" data-resource-percentage>({displayPercentage}%)</span>
			{/if}
		</span>
	</div>
	<div 
		class="h-2 w-full rounded-[var(--radius-full)] bg-[var(--color-surface-hover)] overflow-hidden"
		data-resource-track
	>
		<div 
			class="h-full rounded-[var(--radius-full)] transition-all duration-[var(--transition-normal)] {barColor()}"
			style="width: {displayPercentage}%"
			data-resource-fill
			data-percentage={displayPercentage}
		></div>
	</div>
</div>
