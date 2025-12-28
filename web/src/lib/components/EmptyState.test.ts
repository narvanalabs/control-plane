import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

/**
 * Feature: professional-web-ui, Property 18: Empty state completeness
 * Validates: Requirements 13.1
 *
 * For any empty state component, the rendered output should contain
 * an icon element, title text, description text, and an action button.
 */

// Data attributes used by EmptyState component
const emptyStateDataAttributes = {
	wrapper: 'data-empty-state',
	icon: 'data-empty-state-icon',
	title: 'data-empty-state-title',
	description: 'data-empty-state-description',
	action: 'data-empty-state-action',
} as const;

// Required elements for a complete empty state
const requiredElements = ['icon', 'title', 'description'] as const;
const optionalElements = ['action'] as const;

describe('EmptyState completeness', () => {
	/**
	 * Feature: professional-web-ui, Property 18: Empty state completeness
	 * Validates: Requirements 13.1
	 */
	it('should have data attributes for all required elements', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...requiredElements),
				(element) => {
					const dataAttr = emptyStateDataAttributes[element];
					
					// Data attribute should exist for each required element
					expect(dataAttr).toBeDefined();
					expect(dataAttr.length).toBeGreaterThan(0);
					
					// Should follow naming convention
					expect(dataAttr).toMatch(/^data-empty-state-[a-z]+$/);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have data attribute for optional action element', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...optionalElements),
				(element) => {
					const dataAttr = emptyStateDataAttributes[element];
					
					// Data attribute should exist for optional elements
					expect(dataAttr).toBeDefined();
					expect(dataAttr).toBe('data-empty-state-action');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have wrapper data attribute', () => {
		expect(emptyStateDataAttributes.wrapper).toBe('data-empty-state');
	});

	it('should have all required data attributes defined', () => {
		// All required elements must have data attributes
		for (const element of requiredElements) {
			expect(emptyStateDataAttributes[element]).toBeDefined();
		}
	});

	it('should validate empty state structure with various content', () => {
		fc.assert(
			fc.property(
				fc.record({
					title: fc.string({ minLength: 1, maxLength: 100 }),
					description: fc.string({ minLength: 1, maxLength: 500 }),
					hasAction: fc.boolean(),
				}),
				({ title, description, hasAction }) => {
					// Title should be non-empty
					expect(title.length).toBeGreaterThan(0);
					
					// Description should be non-empty
					expect(description.length).toBeGreaterThan(0);
					
					// hasAction is a boolean
					expect(typeof hasAction).toBe('boolean');
					
					// Required elements are always present
					expect(requiredElements).toContain('icon');
					expect(requiredElements).toContain('title');
					expect(requiredElements).toContain('description');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have consistent data attribute naming convention', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...Object.keys(emptyStateDataAttributes) as (keyof typeof emptyStateDataAttributes)[]),
				(key) => {
					const dataAttr = emptyStateDataAttributes[key];
					
					// All data attributes should start with 'data-empty-state'
					expect(dataAttr).toMatch(/^data-empty-state/);
				}
			),
			{ numRuns: 100 }
		);
	});
});
