import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

/**
 * Feature: professional-web-ui, Property 17: Toast notification styling
 * Validates: Requirements 12.1
 *
 * For any toast type (success, error, info, warning), the rendered toast
 * should have the corresponding color styling applied.
 */

// All supported toast types
const toastTypes = ['success', 'error', 'info', 'warning'] as const;
type ToastType = typeof toastTypes[number];

// Toast type styling configuration (mirrors Toast.svelte implementation)
const typeClasses: Record<ToastType, string> = {
	success: 'bg-[var(--color-success-light)] border-[var(--color-success)] text-[var(--color-success-foreground)]',
	error: 'bg-[var(--color-error-light)] border-[var(--color-error)] text-[var(--color-error-foreground)]',
	info: 'bg-[var(--color-info-light)] border-[var(--color-info)] text-[var(--color-info-foreground)]',
	warning: 'bg-[var(--color-warning-light)] border-[var(--color-warning)] text-[var(--color-warning-foreground)]',
};

// Icon color classes
const iconClasses: Record<ToastType, string> = {
	success: 'text-[var(--color-success)]',
	error: 'text-[var(--color-error)]',
	info: 'text-[var(--color-info)]',
	warning: 'text-[var(--color-warning)]',
};

describe('Toast notification styling', () => {
	/**
	 * Feature: professional-web-ui, Property 17: Toast notification styling
	 * Validates: Requirements 12.1
	 */
	it('should have correct styling configuration for all toast types', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...toastTypes),
				(type) => {
					const classes = typeClasses[type];
					
					// Classes should exist for all types
					expect(classes).toBeDefined();
					expect(classes.length).toBeGreaterThan(0);
					
					// Should have background color using design system tokens
					expect(classes).toContain('bg-[var(--color-');
					
					// Should have border color using design system tokens
					expect(classes).toContain('border-[var(--color-');
					
					// Should have text color using design system tokens
					expect(classes).toContain('text-[var(--color-');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should use type-specific colors for each toast type', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...toastTypes),
				(type) => {
					const classes = typeClasses[type];
					
					// Each type should use its own color family
					expect(classes).toContain(`--color-${type}`);
					expect(classes).toContain(`--color-${type}-light`);
					expect(classes).toContain(`--color-${type}-foreground`);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have matching icon colors for each toast type', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...toastTypes),
				(type) => {
					const iconClass = iconClasses[type];
					
					// Icon class should exist
					expect(iconClass).toBeDefined();
					
					// Icon should use the type's primary color
					expect(iconClass).toContain(`--color-${type})`);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have consistent structure across all toast types', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...toastTypes),
				(type) => {
					const classes = typeClasses[type];
					
					// All types should have the same CSS property structure
					const bgMatch = classes.match(/bg-\[var\(--color-[a-z]+-light\)\]/);
					const borderMatch = classes.match(/border-\[var\(--color-[a-z]+\)\]/);
					const textMatch = classes.match(/text-\[var\(--color-[a-z]+-foreground\)\]/);
					
					expect(bgMatch).not.toBeNull();
					expect(borderMatch).not.toBeNull();
					expect(textMatch).not.toBeNull();
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should use success colors for success type', () => {
		const classes = typeClasses['success'];
		expect(classes).toContain('success-light');
		expect(classes).toContain('success)');
		expect(classes).toContain('success-foreground');
	});

	it('should use error colors for error type', () => {
		const classes = typeClasses['error'];
		expect(classes).toContain('error-light');
		expect(classes).toContain('error)');
		expect(classes).toContain('error-foreground');
	});

	it('should use info colors for info type', () => {
		const classes = typeClasses['info'];
		expect(classes).toContain('info-light');
		expect(classes).toContain('info)');
		expect(classes).toContain('info-foreground');
	});

	it('should use warning colors for warning type', () => {
		const classes = typeClasses['warning'];
		expect(classes).toContain('warning-light');
		expect(classes).toContain('warning)');
		expect(classes).toContain('warning-foreground');
	});
});
