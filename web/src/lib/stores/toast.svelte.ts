/**
 * Toast Notification Store
 * Requirements: 12.1, 12.2, 12.3
 * 
 * Provides a reactive store for managing toast notifications with:
 * - Add/remove methods
 * - Auto-dismiss for success toasts (5 seconds)
 * - Persistent error toasts until manually dismissed
 */

export type ToastType = 'success' | 'error' | 'info' | 'warning';

export interface Toast {
	id: string;
	type: ToastType;
	title: string;
	description?: string;
	duration?: number;
}

// Default durations in milliseconds
const DEFAULT_DURATION = 5000;
const ERROR_DURATION = 0; // 0 means persist until dismissed

class ToastStore {
	toasts = $state<Toast[]>([]);
	private timeouts = new Map<string, ReturnType<typeof setTimeout>>();

	/**
	 * Generate a unique ID for each toast
	 */
	private generateId(): string {
		return `toast-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
	}

	/**
	 * Add a new toast notification
	 */
	add(toast: Omit<Toast, 'id'>): string {
		const id = this.generateId();
		const newToast: Toast = { ...toast, id };
		
		this.toasts = [...this.toasts, newToast];

		// Determine duration based on type
		// Requirement 12.2: Auto-dismiss success toasts after 5 seconds
		// Requirement 12.3: Persist error toasts until dismissed
		const duration = toast.duration ?? (toast.type === 'error' ? ERROR_DURATION : DEFAULT_DURATION);

		if (duration > 0) {
			const timeout = setTimeout(() => {
				this.remove(id);
			}, duration);
			this.timeouts.set(id, timeout);
		}

		return id;
	}

	/**
	 * Remove a toast by ID
	 */
	remove(id: string): void {
		// Clear any pending timeout
		const timeout = this.timeouts.get(id);
		if (timeout) {
			clearTimeout(timeout);
			this.timeouts.delete(id);
		}

		this.toasts = this.toasts.filter(t => t.id !== id);
	}

	/**
	 * Remove all toasts
	 */
	clear(): void {
		// Clear all timeouts
		for (const timeout of this.timeouts.values()) {
			clearTimeout(timeout);
		}
		this.timeouts.clear();
		this.toasts = [];
	}

	/**
	 * Convenience method for success toasts
	 */
	success(title: string, description?: string): string {
		return this.add({ type: 'success', title, description });
	}

	/**
	 * Convenience method for error toasts
	 */
	error(title: string, description?: string): string {
		return this.add({ type: 'error', title, description });
	}

	/**
	 * Convenience method for info toasts
	 */
	info(title: string, description?: string): string {
		return this.add({ type: 'info', title, description });
	}

	/**
	 * Convenience method for warning toasts
	 */
	warning(title: string, description?: string): string {
		return this.add({ type: 'warning', title, description });
	}
}

// Export singleton instance
export const toastStore = new ToastStore();
