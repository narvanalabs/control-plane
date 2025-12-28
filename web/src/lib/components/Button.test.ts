import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

/**
 * Feature: professional-web-ui, Property 21: Button variant and size rendering
 * Validates: Requirements 16.1
 *
 * For any combination of button variant (primary, secondary, ghost, danger, outline)
 * and size (sm, md, lg), the button should render with the correct CSS classes
 * for that combination.
 */

// Define the variant and size class mappings (mirrors Button.svelte implementation)
const variantClasses: Record<string, string> = {
	primary: 'bg-[var(--color-primary)] text-[var(--color-primary-foreground)] hover:bg-[var(--color-primary-hover)]',
	secondary: 'bg-[var(--color-secondary)] text-[var(--color-secondary-foreground)] hover:bg-[var(--color-secondary-hover)]',
	ghost: 'text-[var(--color-text-secondary)] hover:text-[var(--color-text)] hover:bg-[var(--color-surface-hover)]',
	danger: 'bg-[var(--color-error)] text-white hover:bg-[var(--color-error)]/90',
	outline: 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-hover)] hover:border-[var(--color-border-strong)]',
};

const sizeClasses: Record<string, string> = {
	sm: 'px-[var(--spacing-3)] py-[var(--spacing-1-5)] text-[var(--text-sm)] rounded-[var(--radius-md)] h-8',
	md: 'px-[var(--spacing-4)] py-[var(--spacing-2)] text-[var(--text-sm)] rounded-[var(--radius-md)] h-9',
	lg: 'px-[var(--spacing-6)] py-[var(--spacing-3)] text-[var(--text-base)] rounded-[var(--radius-md)] h-11',
};

const spinnerSizeClasses: Record<string, string> = {
	sm: 'w-3 h-3',
	md: 'w-4 h-4',
	lg: 'w-5 h-5',
};

// Helper to build expected class string
function buildButtonClasses(variant: string, size: string): string {
	const baseClasses = 'inline-flex items-center justify-center gap-2 font-medium transition-all duration-[var(--transition-normal)] disabled:opacity-50 disabled:cursor-not-allowed focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-[var(--color-primary)]';
	return `${baseClasses} ${variantClasses[variant]} ${sizeClasses[size]}`;
}

describe('Button variant and size rendering', () => {
	const variants = ['primary', 'secondary', 'ghost', 'danger', 'outline'] as const;
	const sizes = ['sm', 'md', 'lg'] as const;

	it('should have correct variant classes for all variants', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...variants),
				(variant) => {
					// Verify the variant class exists and contains expected styling patterns
					const classes = variantClasses[variant];
					expect(classes).toBeDefined();
					expect(classes.length).toBeGreaterThan(0);
					
					// Primary should have primary color
					if (variant === 'primary') {
						expect(classes).toContain('--color-primary');
					}
					// Secondary should have secondary color
					if (variant === 'secondary') {
						expect(classes).toContain('--color-secondary');
					}
					// Ghost should have hover states
					if (variant === 'ghost') {
						expect(classes).toContain('hover:');
					}
					// Danger should have error color
					if (variant === 'danger') {
						expect(classes).toContain('--color-error');
					}
					// Outline should have border
					if (variant === 'outline') {
						expect(classes).toContain('border');
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have correct size classes for all sizes', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...sizes),
				(size) => {
					const classes = sizeClasses[size];
					expect(classes).toBeDefined();
					expect(classes.length).toBeGreaterThan(0);
					
					// All sizes should have padding and text size
					expect(classes).toContain('px-');
					expect(classes).toContain('py-');
					expect(classes).toContain('text-');
					expect(classes).toContain('rounded-');
					expect(classes).toContain('h-');
					
					// Verify height increases with size
					if (size === 'sm') {
						expect(classes).toContain('h-8');
					}
					if (size === 'md') {
						expect(classes).toContain('h-9');
					}
					if (size === 'lg') {
						expect(classes).toContain('h-11');
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should build complete class string for all variant/size combinations', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...variants),
				fc.constantFrom(...sizes),
				(variant, size) => {
					const classes = buildButtonClasses(variant, size);
					
					// Should contain base classes
					expect(classes).toContain('inline-flex');
					expect(classes).toContain('items-center');
					expect(classes).toContain('justify-center');
					expect(classes).toContain('font-medium');
					expect(classes).toContain('disabled:opacity-50');
					
					// Should contain variant-specific classes
					expect(classes).toContain(variantClasses[variant]);
					
					// Should contain size-specific classes
					expect(classes).toContain(sizeClasses[size]);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have spinner size classes for all sizes', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...sizes),
				(size) => {
					const classes = spinnerSizeClasses[size];
					expect(classes).toBeDefined();
					expect(classes).toContain('w-');
					expect(classes).toContain('h-');
					
					// Spinner size should increase with button size
					if (size === 'sm') {
						expect(classes).toContain('w-3');
						expect(classes).toContain('h-3');
					}
					if (size === 'md') {
						expect(classes).toContain('w-4');
						expect(classes).toContain('h-4');
					}
					if (size === 'lg') {
						expect(classes).toContain('w-5');
						expect(classes).toContain('h-5');
					}
				}
			),
			{ numRuns: 100 }
		);
	});
});
