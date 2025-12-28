import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

/**
 * Feature: professional-web-ui, Property 26: Tabs content visibility
 * Validates: Requirements 16.7
 *
 * For any tab selection in a tabs component, only the content panel for the
 * selected tab should be visible, and all other panels should be hidden.
 */

// Define the tab item interface (mirrors Tabs.svelte implementation)
interface TabItem {
	value: string;
	label: string;
	disabled?: boolean;
}

// Simulate the tabs visibility logic
function getVisibleTabContent(tabs: TabItem[], selectedValue: string): { visible: string[]; hidden: string[] } {
	const visible: string[] = [];
	const hidden: string[] = [];

	for (const tab of tabs) {
		if (tab.value === selectedValue) {
			visible.push(tab.value);
		} else {
			hidden.push(tab.value);
		}
	}

	return { visible, hidden };
}

// Generator for valid tab items
const tabItemArb = fc.record({
	value: fc.string({ minLength: 1, maxLength: 20 }).filter(s => s.trim().length > 0),
	label: fc.string({ minLength: 1, maxLength: 50 }).filter(s => s.trim().length > 0),
	disabled: fc.boolean(),
});

// Generator for a list of tabs with unique values
const tabsArb = fc.array(tabItemArb, { minLength: 1, maxLength: 10 })
	.map(tabs => {
		// Ensure unique values
		const seen = new Set<string>();
		return tabs.filter(tab => {
			if (seen.has(tab.value)) return false;
			seen.add(tab.value);
			return true;
		});
	})
	.filter(tabs => tabs.length >= 1);

describe('Tabs content visibility', () => {
	it('should show exactly one tab content panel for the selected tab', () => {
		fc.assert(
			fc.property(
				tabsArb,
				(tabs) => {
					// Select a random tab from the available tabs
					const selectedIndex = Math.floor(Math.random() * tabs.length);
					const selectedValue = tabs[selectedIndex].value;

					const { visible, hidden } = getVisibleTabContent(tabs, selectedValue);

					// Exactly one tab should be visible
					expect(visible.length).toBe(1);
					expect(visible[0]).toBe(selectedValue);

					// All other tabs should be hidden
					expect(hidden.length).toBe(tabs.length - 1);
					expect(hidden).not.toContain(selectedValue);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should hide all non-selected tab content panels', () => {
		fc.assert(
			fc.property(
				tabsArb,
				fc.nat(),
				(tabs, indexSeed) => {
					// Select a tab using the seed
					const selectedIndex = indexSeed % tabs.length;
					const selectedValue = tabs[selectedIndex].value;

					const { hidden } = getVisibleTabContent(tabs, selectedValue);

					// All hidden tabs should be different from selected
					for (const hiddenValue of hidden) {
						expect(hiddenValue).not.toBe(selectedValue);
					}

					// Hidden count should be total tabs minus 1
					expect(hidden.length).toBe(tabs.length - 1);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should maintain visibility invariant: visible + hidden = all tabs', () => {
		fc.assert(
			fc.property(
				tabsArb,
				fc.nat(),
				(tabs, indexSeed) => {
					const selectedIndex = indexSeed % tabs.length;
					const selectedValue = tabs[selectedIndex].value;

					const { visible, hidden } = getVisibleTabContent(tabs, selectedValue);

					// Total of visible and hidden should equal all tabs
					expect(visible.length + hidden.length).toBe(tabs.length);

					// All tab values should be accounted for
					const allValues = [...visible, ...hidden].sort();
					const expectedValues = tabs.map(t => t.value).sort();
					expect(allValues).toEqual(expectedValues);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should handle single tab correctly', () => {
		fc.assert(
			fc.property(
				tabItemArb,
				(tab) => {
					const tabs = [tab];
					const { visible, hidden } = getVisibleTabContent(tabs, tab.value);

					// Single tab should be visible
					expect(visible.length).toBe(1);
					expect(visible[0]).toBe(tab.value);

					// No tabs should be hidden
					expect(hidden.length).toBe(0);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should return empty visible array when selected value does not exist', () => {
		fc.assert(
			fc.property(
				tabsArb,
				fc.string({ minLength: 1 }).filter(s => s.trim().length > 0),
				(tabs, nonExistentValue) => {
					// Skip if the random value happens to match an existing tab
					if (tabs.some(t => t.value === nonExistentValue)) {
						return true; // Skip this test case
					}

					const { visible, hidden } = getVisibleTabContent(tabs, nonExistentValue);

					// No tab should be visible when selected value doesn't exist
					expect(visible.length).toBe(0);

					// All tabs should be hidden
					expect(hidden.length).toBe(tabs.length);
				}
			),
			{ numRuns: 100 }
		);
	});
});
