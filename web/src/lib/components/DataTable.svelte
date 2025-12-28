<script lang="ts">
	import type { Snippet } from 'svelte';

	/**
	 * DataTable Component
	 * Requirements: 16.6
	 * 
	 * Provides Table component with sorting, loading state, and empty state.
	 * Supports column definitions with headers and sortable columns.
	 */
	
	export type SortDirection = 'asc' | 'desc' | null;
	
	export interface Column<T> {
		key: string;
		header: string;
		sortable?: boolean;
		render?: Snippet<[T]>;
		class?: string;
	}

	interface Props<T> {
		data: T[];
		columns: Column<T>[];
		loading?: boolean;
		emptyMessage?: string;
		onRowClick?: (row: T) => void;
		sortKey?: string | null;
		sortDirection?: SortDirection;
		onSort?: (key: string, direction: SortDirection) => void;
		class?: string;
	}

	let { 
		data = [],
		columns = [],
		loading = false,
		emptyMessage = 'No data available',
		onRowClick,
		sortKey = null,
		sortDirection = null,
		onSort,
		class: className = '',
	}: Props<Record<string, unknown>> = $props();

	function handleSort(column: Column<Record<string, unknown>>) {
		if (!column.sortable || !onSort) return;
		
		let newDirection: SortDirection;
		if (sortKey !== column.key) {
			// New column, start with ascending
			newDirection = 'asc';
		} else if (sortDirection === 'asc') {
			// Toggle to descending
			newDirection = 'desc';
		} else if (sortDirection === 'desc') {
			// Toggle to no sort
			newDirection = null;
		} else {
			// No sort, start with ascending
			newDirection = 'asc';
		}
		
		onSort(column.key, newDirection);
	}

	function getCellValue(row: Record<string, unknown>, key: string): unknown {
		return row[key];
	}

	// Generate skeleton rows for loading state
	const skeletonRows = Array.from({ length: 5 }, (_, i) => i);
</script>

<div class="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--color-border)] bg-[var(--color-surface)] {className}" data-datatable>
	<div class="overflow-x-auto">
		<table class="w-full border-collapse text-[var(--text-sm)]">
			<thead>
				<tr class="border-b border-[var(--color-border)] bg-[var(--color-background-subtle)]">
					{#each columns as column}
						<th 
							class="px-[var(--spacing-4)] py-[var(--spacing-3)] text-left font-semibold text-[var(--color-text-secondary)] {column.class || ''}"
							data-column-key={column.key}
							data-sortable={column.sortable ? 'true' : undefined}
						>
							{#if column.sortable && onSort}
								<button
									type="button"
									class="inline-flex items-center gap-[var(--spacing-1)] hover:text-[var(--color-text)] transition-colors duration-[var(--transition-fast)]"
									onclick={() => handleSort(column)}
									data-sort-button
								>
									{column.header}
									<span class="inline-flex flex-col" aria-hidden="true">
										<svg 
											class="w-3 h-3 {sortKey === column.key && sortDirection === 'asc' ? 'text-[var(--color-primary)]' : 'text-[var(--color-text-muted)]'}" 
											viewBox="0 0 24 24" 
											fill="none" 
											stroke="currentColor" 
											stroke-width="2"
										>
											<path d="M18 15l-6-6-6 6"/>
										</svg>
										<svg 
											class="w-3 h-3 -mt-1 {sortKey === column.key && sortDirection === 'desc' ? 'text-[var(--color-primary)]' : 'text-[var(--color-text-muted)]'}" 
											viewBox="0 0 24 24" 
											fill="none" 
											stroke="currentColor" 
											stroke-width="2"
										>
											<path d="M6 9l6 6 6-6"/>
										</svg>
									</span>
								</button>
							{:else}
								{column.header}
							{/if}
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#if loading}
					{#each skeletonRows as _, rowIndex}
						<tr 
							class="border-b border-[var(--color-border)] last:border-b-0"
							data-skeleton-row={rowIndex}
						>
							{#each columns as column}
								<td class="px-[var(--spacing-4)] py-[var(--spacing-3)] {column.class || ''}">
									<div class="h-4 bg-[var(--color-surface-hover)] rounded animate-pulse w-3/4"></div>
								</td>
							{/each}
						</tr>
					{/each}
				{:else if data.length === 0}
					<tr data-empty-row>
						<td 
							colspan={columns.length} 
							class="px-[var(--spacing-4)] py-[var(--spacing-12)] text-center text-[var(--color-text-muted)]"
						>
							{emptyMessage}
						</td>
					</tr>
				{:else}
					{#each data as row, rowIndex}
						<tr 
							class="border-b border-[var(--color-border)] last:border-b-0 
								{onRowClick ? 'cursor-pointer hover:bg-[var(--color-surface-hover)] transition-colors duration-[var(--transition-fast)]' : ''}"
							onclick={() => onRowClick?.(row)}
							data-row-index={rowIndex}
						>
							{#each columns as column}
								<td class="px-[var(--spacing-4)] py-[var(--spacing-3)] text-[var(--color-text)] {column.class || ''}">
									{#if column.render}
										{@render column.render(row)}
									{:else}
										{getCellValue(row, column.key) ?? '—'}
									{/if}
								</td>
							{/each}
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
	</div>
</div>
