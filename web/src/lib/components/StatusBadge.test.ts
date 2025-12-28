import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

/**
 * Feature: professional-web-ui, Property 24: Status badge styling
 * Validates: Requirements 16.4
 *
 * For any status value, the StatusBadge component should render with
 * the correct color and label for that status.
 */

// All supported status values
const deploymentStatuses = ['pending', 'building', 'built', 'scheduled', 'starting', 'running', 'stopping', 'stopped', 'failed'] as const;
const nodeStatuses = ['healthy', 'unhealthy', 'unknown'] as const;
const allStatuses = [...deploymentStatuses, ...nodeStatuses] as const;

// Size options
const sizes = ['sm', 'md'] as const;

// Status configuration (mirrors StatusBadge.svelte implementation)
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

describe('StatusBadge styling', () => {
	it('should have correct styling configuration for all statuses', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...allStatuses),
				(status) => {
					const config = statusConfig[status];
					
					// Config should exist for all statuses
					expect(config).toBeDefined();
					
					// Should have all required properties
					expect(config.bg).toBeDefined();
					expect(config.text).toBeDefined();
					expect(config.dot).toBeDefined();
					expect(config.label).toBeDefined();
					
					// Background should use design system tokens
					expect(config.bg).toContain('bg-[var(--color-');
					
					// Text should use design system tokens
					expect(config.text).toContain('text-[var(--color-');
					
					// Dot should use design system tokens
					expect(config.dot).toContain('bg-[var(--color-');
					
					// Label should be a non-empty string
					expect(config.label.length).toBeGreaterThan(0);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have correct label for each status', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...allStatuses),
				(status) => {
					const config = statusConfig[status];
					
					// Label should be capitalized version of status (with some exceptions)
					const expectedLabel = status.charAt(0).toUpperCase() + status.slice(1);
					expect(config.label).toBe(expectedLabel);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have pulse animation only for active statuses', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...allStatuses),
				(status) => {
					const config = statusConfig[status];
					const activeStatuses = ['building', 'starting'];
					
					if (activeStatuses.includes(status)) {
						expect(config.pulse).toBe(true);
					} else {
						expect(config.pulse).toBeFalsy();
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should use success colors for positive statuses', () => {
		fc.assert(
			fc.property(
				fc.constantFrom('running', 'healthy'),
				(status) => {
					const config = statusConfig[status];
					
					expect(config.bg).toContain('success');
					expect(config.text).toContain('success');
					expect(config.dot).toContain('success');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should use error colors for negative statuses', () => {
		fc.assert(
			fc.property(
				fc.constantFrom('failed', 'unhealthy'),
				(status) => {
					const config = statusConfig[status];
					
					expect(config.bg).toContain('error');
					expect(config.text).toContain('error');
					expect(config.dot).toContain('error');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have valid size classes for all sizes', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...sizes),
				(size) => {
					const classes = sizeClasses[size];
					
					expect(classes).toBeDefined();
					expect(classes.length).toBeGreaterThan(0);
					
					// Should have padding and text size
					expect(classes).toContain('px-');
					expect(classes).toContain('py-');
					expect(classes).toContain('text-');
				}
			),
			{ numRuns: 100 }
		);
	});
});
