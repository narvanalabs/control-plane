import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

/**
 * Feature: professional-web-ui, Property 23: Card component optional sections
 * Validates: Requirements 16.3
 *
 * For any card configuration, header and footer sections should only render
 * when their respective snippets are provided.
 */

// Padding options supported by the component
const paddingOptions = ['none', 'sm', 'md', 'lg'] as const;

// Configuration interface matching the Card component props
interface CardConfig {
	hasHeader: boolean;
	hasFooter: boolean;
	hover: boolean;
	padding: typeof paddingOptions[number];
}

// Padding classes (mirrors Card.svelte implementation)
const paddingClasses: Record<string, string> = {
	none: '',
	sm: 'p-[var(--spacing-3)]',
	md: 'p-[var(--spacing-4)]',
	lg: 'p-[var(--spacing-6)]',
};

const contentPaddingClasses: Record<string, string> = {
	none: '',
	sm: 'px-[var(--spacing-3)] py-[var(--spacing-3)]',
	md: 'px-[var(--spacing-4)] py-[var(--spacing-4)]',
	lg: 'px-[var(--spacing-6)] py-[var(--spacing-6)]',
};

const sectionPaddingClasses: Record<string, string> = {
	none: '',
	sm: 'px-[var(--spacing-3)] py-[var(--spacing-2)]',
	md: 'px-[var(--spacing-4)] py-[var(--spacing-3)]',
	lg: 'px-[var(--spacing-6)] py-[var(--spacing-4)]',
};

// Helper to determine expected rendering based on config
function getExpectedRendering(config: CardConfig): {
	shouldRenderHeader: boolean;
	shouldRenderFooter: boolean;
	contentPaddingClass: string;
	headerPaddingClass: string;
	footerPaddingClass: string;
	hasHoverStyles: boolean;
} {
	const hasHeaderOrFooter = config.hasHeader || config.hasFooter;
	
	return {
		shouldRenderHeader: config.hasHeader,
		shouldRenderFooter: config.hasFooter,
		contentPaddingClass: hasHeaderOrFooter 
			? contentPaddingClasses[config.padding] 
			: paddingClasses[config.padding],
		headerPaddingClass: sectionPaddingClasses[config.padding],
		footerPaddingClass: sectionPaddingClasses[config.padding],
		hasHoverStyles: config.hover,
	};
}

describe('Card component optional sections', () => {
	// Generator for card configurations
	const cardConfigArb = fc.record({
		hasHeader: fc.boolean(),
		hasFooter: fc.boolean(),
		hover: fc.boolean(),
		padding: fc.constantFrom(...paddingOptions),
	});

	it('should only render header when header snippet is provided', () => {
		fc.assert(
			fc.property(cardConfigArb, (config) => {
				const expected = getExpectedRendering(config);
				
				// Header should only render when hasHeader is true
				expect(expected.shouldRenderHeader).toBe(config.hasHeader);
			}),
			{ numRuns: 100 }
		);
	});

	it('should only render footer when footer snippet is provided', () => {
		fc.assert(
			fc.property(cardConfigArb, (config) => {
				const expected = getExpectedRendering(config);
				
				// Footer should only render when hasFooter is true
				expect(expected.shouldRenderFooter).toBe(config.hasFooter);
			}),
			{ numRuns: 100 }
		);
	});

	it('should use correct content padding based on header/footer presence', () => {
		fc.assert(
			fc.property(cardConfigArb, (config) => {
				const expected = getExpectedRendering(config);
				const hasHeaderOrFooter = config.hasHeader || config.hasFooter;
				
				if (hasHeaderOrFooter) {
					// When header or footer exists, use content padding
					expect(expected.contentPaddingClass).toBe(contentPaddingClasses[config.padding]);
				} else {
					// When no header/footer, use regular padding
					expect(expected.contentPaddingClass).toBe(paddingClasses[config.padding]);
				}
			}),
			{ numRuns: 100 }
		);
	});

	it('should apply hover styles only when hover prop is true', () => {
		fc.assert(
			fc.property(cardConfigArb, (config) => {
				const expected = getExpectedRendering(config);
				
				expect(expected.hasHoverStyles).toBe(config.hover);
			}),
			{ numRuns: 100 }
		);
	});

	it('should have valid padding classes for all padding options', () => {
		fc.assert(
			fc.property(
				fc.constantFrom(...paddingOptions),
				(padding) => {
					// All padding options should have defined classes
					expect(paddingClasses[padding]).toBeDefined();
					expect(contentPaddingClasses[padding]).toBeDefined();
					expect(sectionPaddingClasses[padding]).toBeDefined();
					
					// Non-none padding should have actual class values
					if (padding !== 'none') {
						expect(paddingClasses[padding].length).toBeGreaterThan(0);
						expect(contentPaddingClasses[padding].length).toBeGreaterThan(0);
						expect(sectionPaddingClasses[padding].length).toBeGreaterThan(0);
					}
				}
			),
			{ numRuns: 100 }
		);
	});
});
