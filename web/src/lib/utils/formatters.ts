/**
 * Utility functions for formatting data in the UI
 * Requirements: 10.3
 */

/**
 * Format a timestamp as a relative time string
 * - "just now" for <60 seconds
 * - "Xm ago" for <1 hour
 * - "Xh ago" for <24 hours
 * - "Xd ago" for >=24 hours
 * 
 * @param timestamp - ISO 8601 timestamp string
 * @returns Human-readable relative time string
 */
export function formatRelativeTime(timestamp: string): string {
	const date = new Date(timestamp);
	const now = new Date();
	const secondsAgo = Math.floor((now.getTime() - date.getTime()) / 1000);

	// Handle future dates or invalid timestamps
	if (secondsAgo < 0 || isNaN(secondsAgo)) {
		return 'just now';
	}

	if (secondsAgo < 60) {
		return 'just now';
	}

	const minutesAgo = Math.floor(secondsAgo / 60);
	if (minutesAgo < 60) {
		return `${minutesAgo}m ago`;
	}

	const hoursAgo = Math.floor(minutesAgo / 60);
	if (hoursAgo < 24) {
		return `${hoursAgo}h ago`;
	}

	const daysAgo = Math.floor(hoursAgo / 24);
	return `${daysAgo}d ago`;
}

/**
 * Format a timestamp as a localized date string
 * 
 * @param timestamp - ISO 8601 timestamp string
 * @returns Formatted date string (e.g., "Dec 28, 2025")
 */
export function formatDate(timestamp: string): string {
	const date = new Date(timestamp);
	return date.toLocaleDateString('en-US', {
		month: 'short',
		day: 'numeric',
		year: 'numeric'
	});
}

/**
 * Format bytes as a human-readable string
 * 
 * @param bytes - Number of bytes
 * @returns Formatted string (e.g., "1.5 GB")
 */
export function formatBytes(bytes: number): string {
	if (bytes === 0) return '0 B';
	
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	const k = 1024;
	const i = Math.floor(Math.log(bytes) / Math.log(k));
	const value = bytes / Math.pow(k, i);
	
	return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

/**
 * Format CPU millicores as a human-readable string
 * 
 * @param millicores - CPU in millicores (1000m = 1 CPU)
 * @returns Formatted string (e.g., "0.5 CPU" or "2 CPU")
 */
export function formatCPU(millicores: number): string {
	const cpus = millicores / 1000;
	if (cpus >= 1) {
		return `${cpus.toFixed(cpus % 1 === 0 ? 0 : 1)} CPU`;
	}
	return `${millicores}m`;
}
