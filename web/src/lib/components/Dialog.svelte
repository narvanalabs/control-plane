<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { X } from 'lucide-svelte';

	/**
	 * Dialog/Modal Component
	 * Requirements: 11.1, 11.2, 11.5
	 * 
	 * Provides modal dialog with:
	 * - Semi-transparent backdrop (11.1)
	 * - Close on outside click and Escape key (11.2)
	 * - Title and description slots
	 * - Footer slot for action buttons (11.5)
	 */
	interface Props {
		open: boolean;
		onOpenChange?: (open: boolean) => void;
		title: string;
		description?: string;
		children: Snippet;
		footer?: Snippet;
		class?: string;
	}

	let { 
		open = $bindable(false),
		onOpenChange,
		title,
		description,
		children,
		footer,
		class: className = '',
	}: Props = $props();

	function handleOpenChange(newOpen: boolean) {
		open = newOpen;
		onOpenChange?.(newOpen);
	}
</script>

<DialogPrimitive.Root {open} onOpenChange={handleOpenChange}>
	<DialogPrimitive.Portal>
		<!-- Backdrop - Requirement 11.1: Semi-transparent backdrop -->
		<DialogPrimitive.Overlay
			class="fixed inset-0 z-[var(--z-modal-backdrop)] bg-black/50 backdrop-blur-sm
				data-[state=open]:animate-in data-[state=open]:fade-in-0
				data-[state=closed]:animate-out data-[state=closed]:fade-out-0"
			data-dialog-overlay
		/>
		
		<!-- Dialog Content - Requirement 11.2: Close on outside click (handled by bits-ui) -->
		<DialogPrimitive.Content
			class="fixed left-1/2 top-1/2 z-[var(--z-modal)] w-full max-w-lg -translate-x-1/2 -translate-y-1/2
				bg-[var(--color-surface)] rounded-[var(--radius-xl)] shadow-[var(--shadow-xl)]
				border border-[var(--color-border)]
				data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%]
				data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%]
				duration-200 {className}"
			data-dialog-content
		>
			<!-- Header -->
			<div class="flex items-start justify-between p-6 border-b border-[var(--color-border)]" data-dialog-header>
				<div class="space-y-[var(--spacing-1-5)]">
					<DialogPrimitive.Title 
						class="text-[var(--text-lg)] font-semibold text-[var(--color-text)]"
						data-dialog-title
					>
						{title}
					</DialogPrimitive.Title>
					{#if description}
						<DialogPrimitive.Description 
							class="text-[var(--text-sm)] text-[var(--color-text-secondary)]"
							data-dialog-description
						>
							{description}
						</DialogPrimitive.Description>
					{/if}
				</div>
				
				<!-- Close button -->
				<DialogPrimitive.Close
					class="p-[var(--spacing-1-5)] rounded-[var(--radius-sm)] text-[var(--color-text-muted)]
						hover:text-[var(--color-text)] hover:bg-[var(--color-surface-hover)]
						transition-colors duration-[var(--transition-fast)]
						focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
					aria-label="Close dialog"
					data-dialog-close
				>
					<X class="w-5 h-5" />
				</DialogPrimitive.Close>
			</div>
			
			<!-- Body -->
			<div class="p-6" data-dialog-body>
				{@render children()}
			</div>
			
			<!-- Footer - Requirement 11.5: Consistent button placement -->
			{#if footer}
				<div 
					class="flex items-center justify-end gap-[var(--spacing-3)] p-6 pt-0"
					data-dialog-footer
				>
					{@render footer()}
				</div>
			{/if}
		</DialogPrimitive.Content>
	</DialogPrimitive.Portal>
</DialogPrimitive.Root>
