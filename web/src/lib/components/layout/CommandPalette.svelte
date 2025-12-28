<script lang="ts">
	import { goto } from '$app/navigation';

	/**
	 * CommandPalette Component
	 * Requirements: 2.3
	 * 
	 * Keyboard-accessible search and command interface (Ctrl+K / Cmd+K).
	 * Provides quick navigation and actions.
	 */

	interface CommandItem {
		id: string;
		label: string;
		description?: string;
		icon?: string;
		shortcut?: string;
		action: () => void;
		group?: string;
	}

	interface Props {
		open?: boolean;
		onOpenChange?: (open: boolean) => void;
		items?: CommandItem[];
		placeholder?: string;
	}

	let { 
		open = $bindable(false), 
		onOpenChange,
		items = [],
		placeholder = 'Search commands...'
	}: Props = $props();

	let searchQuery = $state('');
	let selectedIndex = $state(0);
	let inputRef: HTMLInputElement | undefined = $state();

	// Default navigation items if none provided
	const defaultItems: CommandItem[] = [
		{
			id: 'dashboard',
			label: 'Go to Dashboard',
			description: 'View your dashboard overview',
			icon: 'home',
			group: 'Navigation',
			action: () => { goto('/dashboard'); close(); }
		},
		{
			id: 'apps',
			label: 'Go to Applications',
			description: 'Manage your applications',
			icon: 'apps',
			group: 'Navigation',
			action: () => { goto('/apps'); close(); }
		},
		{
			id: 'nodes',
			label: 'Go to Infrastructure',
			description: 'View infrastructure nodes',
			icon: 'nodes',
			group: 'Navigation',
			action: () => { goto('/nodes'); close(); }
		},
		{
			id: 'new-app',
			label: 'Create New Application',
			description: 'Start a new application',
			icon: 'plus',
			group: 'Actions',
			action: () => { goto('/apps?new=true'); close(); }
		},
	];

	const allItems = $derived(items.length > 0 ? items : defaultItems);

	// Filter items based on search query
	const filteredItems = $derived(() => {
		if (!searchQuery.trim()) return allItems;
		
		const query = searchQuery.toLowerCase();
		return allItems.filter(item => 
			item.label.toLowerCase().includes(query) ||
			item.description?.toLowerCase().includes(query) ||
			item.group?.toLowerCase().includes(query)
		);
	});

	// Group filtered items
	const groupedItems = $derived(() => {
		const items = filteredItems();
		const groups: Record<string, CommandItem[]> = {};
		
		for (const item of items) {
			const group = item.group || 'Commands';
			if (!groups[group]) groups[group] = [];
			groups[group].push(item);
		}
		
		return groups;
	});

	// Flat list for keyboard navigation
	const flatItems = $derived(() => filteredItems());

	function close() {
		open = false;
		onOpenChange?.(false);
		searchQuery = '';
		selectedIndex = 0;
	}

	function handleKeydown(e: KeyboardEvent) {
		const items = flatItems();
		
		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				selectedIndex = Math.min(selectedIndex + 1, items.length - 1);
				break;
			case 'ArrowUp':
				e.preventDefault();
				selectedIndex = Math.max(selectedIndex - 1, 0);
				break;
			case 'Enter':
				e.preventDefault();
				if (items[selectedIndex]) {
					items[selectedIndex].action();
				}
				break;
			case 'Escape':
				e.preventDefault();
				close();
				break;
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			close();
		}
	}

	// Reset selection when search changes
	$effect(() => {
		searchQuery;
		selectedIndex = 0;
	});

	// Focus input when opened
	$effect(() => {
		if (open && inputRef) {
			inputRef.focus();
		}
	});

	// Icons
	const icons: Record<string, string> = {
		search: 'M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z',
		home: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6',
		apps: 'M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z',
		nodes: 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01',
		plus: 'M12 4v16m8-8H4',
		command: 'M18 3a3 3 0 00-3 3v12a3 3 0 003 3 3 3 0 003-3 3 3 0 00-3-3H6a3 3 0 00-3 3 3 3 0 003 3 3 3 0 003-3V6a3 3 0 00-3-3 3 3 0 00-3 3 3 3 0 003 3h12a3 3 0 003-3 3 3 0 00-3-3z',
	};
</script>

<!-- Global keyboard listener for Ctrl+K / Cmd+K -->
<svelte:window 
	onkeydown={(e) => {
		if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
			e.preventDefault();
			open = !open;
			onOpenChange?.(open);
		}
	}}
/>

{#if open}
	<!-- Backdrop -->
	<div 
		class="fixed inset-0 bg-black/50 z-[var(--z-modal-backdrop)] animate-fade-in"
		onclick={handleBackdropClick}
		role="presentation"
	>
		<!-- Command palette dialog -->
		<div 
			class="fixed top-[20%] left-1/2 -translate-x-1/2 w-full max-w-lg bg-[var(--color-surface)] rounded-[var(--radius-xl)] shadow-[var(--shadow-xl)] border border-[var(--color-border)] overflow-hidden animate-scale-in"
			role="dialog"
			aria-modal="true"
			aria-label="Command palette"
			onkeydown={handleKeydown}
		>
			<!-- Search input -->
			<div class="flex items-center gap-3 px-4 py-3 border-b border-[var(--color-border)]">
				<svg class="w-5 h-5 text-[var(--color-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={icons.search} />
				</svg>
				<input
					bind:this={inputRef}
					bind:value={searchQuery}
					type="text"
					{placeholder}
					class="flex-1 bg-transparent border-none outline-none text-[var(--color-text)] placeholder:text-[var(--color-text-muted)] text-sm"
					aria-label="Search commands"
				/>
				<kbd class="px-1.5 py-0.5 rounded bg-[var(--color-background-subtle)] border border-[var(--color-border)] text-xs text-[var(--color-text-muted)] font-mono">
					ESC
				</kbd>
			</div>

			<!-- Results -->
			<div class="max-h-80 overflow-y-auto py-2">
				{#if flatItems().length === 0}
					<div class="px-4 py-8 text-center text-[var(--color-text-muted)] text-sm">
						No results found for "{searchQuery}"
					</div>
				{:else}
					{#each Object.entries(groupedItems()) as [group, groupItems]}
						<div class="px-2 py-1">
							<div class="px-2 py-1 text-xs font-medium text-[var(--color-text-muted)] uppercase tracking-wider">
								{group}
							</div>
							{#each groupItems as item, index}
								{@const globalIndex = flatItems().indexOf(item)}
								{@const isSelected = globalIndex === selectedIndex}
								<button
									onclick={() => item.action()}
									class="w-full flex items-center gap-3 px-2 py-2 rounded-[var(--radius-md)] text-left transition-colors"
									class:bg-[var(--color-primary)]={isSelected}
									class:text-[var(--color-primary-foreground)]={isSelected}
									class:hover:bg-[var(--color-surface-hover)]={!isSelected}
									role="option"
									aria-selected={isSelected}
								>
									{#if item.icon && icons[item.icon]}
										<svg 
											class="w-5 h-5 shrink-0" 
											class:text-[var(--color-primary-foreground)]={isSelected}
											class:text-[var(--color-text-muted)]={!isSelected}
											fill="none" 
											stroke="currentColor" 
											viewBox="0 0 24 24"
										>
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d={icons[item.icon]} />
										</svg>
									{/if}
									<div class="flex-1 min-w-0">
										<div class="text-sm font-medium truncate">{item.label}</div>
										{#if item.description}
											<div 
												class="text-xs truncate"
												class:opacity-70={isSelected}
												class:text-[var(--color-primary-foreground)]={isSelected}
												class:text-[var(--color-text-muted)]={!isSelected}
											>
												{item.description}
											</div>
										{/if}
									</div>
									{#if item.shortcut}
										<kbd 
											class="px-1.5 py-0.5 rounded text-xs font-mono shrink-0"
											class:opacity-20={isSelected}
											class:bg-[var(--color-primary-foreground)]={isSelected}
											class:text-[var(--color-primary-foreground)]={isSelected}
											class:bg-[var(--color-background-subtle)]={!isSelected}
											class:border={!isSelected}
											class:border-[var(--color-border)]={!isSelected}
											class:text-[var(--color-text-muted)]={!isSelected}
										>
											{item.shortcut}
										</kbd>
									{/if}
								</button>
							{/each}
						</div>
					{/each}
				{/if}
			</div>

			<!-- Footer -->
			<div class="px-4 py-2 border-t border-[var(--color-border)] bg-[var(--color-background-subtle)] flex items-center gap-4 text-xs text-[var(--color-text-muted)]">
				<span class="flex items-center gap-1">
					<kbd class="px-1 py-0.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] font-mono">↑</kbd>
					<kbd class="px-1 py-0.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] font-mono">↓</kbd>
					to navigate
				</span>
				<span class="flex items-center gap-1">
					<kbd class="px-1 py-0.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] font-mono">↵</kbd>
					to select
				</span>
				<span class="flex items-center gap-1">
					<kbd class="px-1 py-0.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] font-mono">esc</kbd>
					to close
				</span>
			</div>
		</div>
	</div>
{/if}
