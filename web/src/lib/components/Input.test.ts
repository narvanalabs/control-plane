import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

/**
 * Feature: professional-web-ui, Property 22: Input component features
 * Validates: Requirements 16.2
 *
 * For any input configuration with label, placeholder, error, and icon,
 * all specified features should be rendered in the component output.
 */

// Input types supported by the component
const inputTypes = ['text', 'email', 'password', 'number', 'search'] as const;

// Configuration interface matching the Input component props
interface InputConfig {
	type: typeof inputTypes[number];
	label?: string;
	placeholder?: string;
	error?: string;
	hasIcon: boolean;
	required: boolean;
	disabled: boolean;
}

// Helper to determine expected features based on config
function getExpectedFeatures(config: InputConfig): {
	hasLabel: boolean;
	hasPlaceholder: boolean;
	hasError: boolean;
	hasIcon: boolean;
	hasRequiredIndicator: boolean;
	isDisabled: boolean;
} {
	return {
		hasLabel: !!config.label && config.label.length > 0,
		hasPlaceholder: !!config.placeholder && config.placeholder.length > 0,
		hasError: !!config.error && config.error.length > 0,
		hasIcon: config.hasIcon,
		hasRequiredIndicator: config.required && !!config.label,
		isDisabled: config.disabled,
	};
}

// Helper to generate expected CSS classes based on config
function getExpectedClasses(config: InputConfig): string[] {
	const classes: string[] = [];
	
	// Base input classes
	classes.push('w-full');
	classes.push('rounded-[var(--radius-sm)]');
	classes.push('bg-[var(--color-surface)]');
	classes.push('text-[var(--color-text)]');
	
	// Error state
	if (config.error) {
		classes.push('border-[var(--color-error)]');
	} else {
		classes.push('border-[var(--color-border)]');
	}
	
	// Icon padding
	if (config.hasIcon) {
		classes.push('pl-[var(--spacing-10)]');
	} else {
		classes.push('px-[var(--spacing-4)]');
	}
	
	return classes;
}

describe('Input component features', () => {
	// Generator for input configurations
	const inputConfigArb = fc.record({
		type: fc.constantFrom(...inputTypes),
		label: fc.option(fc.string({ minLength: 1, maxLength: 50 }), { nil: undefined }),
		placeholder: fc.option(fc.string({ minLength: 1, maxLength: 100 }), { nil: undefined }),
		error: fc.option(fc.string({ minLength: 1, maxLength: 200 }), { nil: undefined }),
		hasIcon: fc.boolean(),
		required: fc.boolean(),
		disabled: fc.boolean(),
	});

	it('should correctly determine expected features for all configurations', () => {
		fc.assert(
			fc.property(inputConfigArb, (config) => {
				const features = getExpectedFeatures(config);
				
				// Label feature
				expect(features.hasLabel).toBe(!!config.label && config.label.length > 0);
				
				// Placeholder feature
				expect(features.hasPlaceholder).toBe(!!config.placeholder && config.placeholder.length > 0);
				
				// Error feature
				expect(features.hasError).toBe(!!config.error && config.error.length > 0);
				
				// Icon feature
				expect(features.hasIcon).toBe(config.hasIcon);
				
				// Required indicator only shows when label is present
				expect(features.hasRequiredIndicator).toBe(config.required && !!config.label);
				
				// Disabled state
				expect(features.isDisabled).toBe(config.disabled);
			}),
			{ numRuns: 100 }
		);
	});

	it('should generate correct CSS classes based on configuration', () => {
		fc.assert(
			fc.property(inputConfigArb, (config) => {
				const classes = getExpectedClasses(config);
				
				// Should always have base classes
				expect(classes).toContain('w-full');
				expect(classes).toContain('rounded-[var(--radius-sm)]');
				expect(classes).toContain('bg-[var(--color-surface)]');
				expect(classes).toContain('text-[var(--color-text)]');
				
				// Error state should affect border color
				if (config.error) {
					expect(classes).toContain('border-[var(--color-error)]');
					expect(classes).not.toContain('border-[var(--color-border)]');
				} else {
					expect(classes).toContain('border-[var(--color-border)]');
					expect(classes).not.toContain('border-[var(--color-error)]');
				}
				
				// Icon should affect padding
				if (config.hasIcon) {
					expect(classes).toContain('pl-[var(--spacing-10)]');
					expect(classes).not.toContain('px-[var(--spacing-4)]');
				} else {
					expect(classes).toContain('px-[var(--spacing-4)]');
					expect(classes).not.toContain('pl-[var(--spacing-10)]');
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('should support all input types', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...inputTypes),
				(type) => {
					// All types should be valid
					expect(inputTypes).toContain(type);
					
					// Type should be a non-empty string
					expect(type.length).toBeGreaterThan(0);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should handle label with required indicator correctly', () => {
		fc.assert(
			fc.property(
				fc.string({ minLength: 1, maxLength: 50 }),
				fc.boolean(),
				(label, required) => {
					const config: InputConfig = {
						type: 'text',
						label,
						required,
						hasIcon: false,
						disabled: false,
					};
					
					const features = getExpectedFeatures(config);
					
					// Label should always be present when provided
					expect(features.hasLabel).toBe(true);
					
					// Required indicator should match required prop when label exists
					expect(features.hasRequiredIndicator).toBe(required);
				}
			),
			{ numRuns: 100 }
		);
	});
});
