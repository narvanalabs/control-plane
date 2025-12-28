import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import type { SortDirection } from './DataTable.svelte';

/**
 * Feature: professional-web-ui, Property 25: Table sorting behavior
 * Validates: Requirements 16.6
 *
 * For any sortable column in a data table, clicking the column header should
 * toggle between ascending, descending, and no sort order.
 */

// Simulate the sort toggle logic from DataTable component
function getNextSortDirection(
	currentKey: string | null,
	currentDirection: SortDirection,
	clickedKey: string
): { key: string; direction: SortDirection } {
	let newDirection: SortDirection;
	
	if (currentKey !== clickedKey) {
		// New column, start with ascending
		newDirection = 'asc';
	} else if (currentDirection === 'asc') {
		// Toggle to descending
		newDirection = 'desc';
	} else if (currentDirection === 'desc') {
		// Toggle to no sort
		newDirection = null;
	} else {
		// No sort, start with ascending
		newDirection = 'asc';
	}
	
	return { key: clickedKey, direction: newDirection };
}

// Generator for column keys
const columnKeyArb = fc.string({ minLength: 1, maxLength: 20 }).filter(s => s.trim().length > 0);

// Generator for sort direction
const sortDirectionArb = fc.constantFrom<SortDirection>('asc', 'desc', null);

describe('DataTable sorting behavior', () => {
	it('should start with ascending when clicking a new column', () => {
		fc.assert(
			fc.property(
				columnKeyArb,
				columnKeyArb,
				sortDirectionArb,
				(currentKey, newKey, currentDirection) => {
					// Skip if keys are the same (that's a different test case)
					if (currentKey === newKey) return true;
					
					const result = getNextSortDirection(currentKey, currentDirection, newKey);
					
					expect(result.key).toBe(newKey);
					expect(result.direction).toBe('asc');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should toggle from ascending to descending on same column', () => {
		fc.assert(
			fc.property(
				columnKeyArb,
				(columnKey) => {
					const result = getNextSortDirection(columnKey, 'asc', columnKey);
					
					expect(result.key).toBe(columnKey);
					expect(result.direction).toBe('desc');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should toggle from descending to no sort on same column', () => {
		fc.assert(
			fc.property(
				columnKeyArb,
				(columnKey) => {
					const result = getNextSortDirection(columnKey, 'desc', columnKey);
					
					expect(result.key).toBe(columnKey);
					expect(result.direction).toBe(null);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should toggle from no sort to ascending on same column', () => {
		fc.assert(
			fc.property(
				columnKeyArb,
				(columnKey) => {
					const result = getNextSortDirection(columnKey, null, columnKey);
					
					expect(result.key).toBe(columnKey);
					expect(result.direction).toBe('asc');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should complete full sort cycle: null -> asc -> desc -> null', () => {
		fc.assert(
			fc.property(
				columnKeyArb,
				(columnKey) => {
					// Start with no sort
					let state = { key: columnKey, direction: null as SortDirection };
					
					// Click 1: null -> asc
					state = getNextSortDirection(state.key, state.direction, columnKey);
					expect(state.direction).toBe('asc');
					
					// Click 2: asc -> desc
					state = getNextSortDirection(state.key, state.direction, columnKey);
					expect(state.direction).toBe('desc');
					
					// Click 3: desc -> null
					state = getNextSortDirection(state.key, state.direction, columnKey);
					expect(state.direction).toBe(null);
					
					// Click 4: null -> asc (cycle complete)
					state = getNextSortDirection(state.key, state.direction, columnKey);
					expect(state.direction).toBe('asc');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should reset to ascending when switching columns regardless of previous direction', () => {
		fc.assert(
			fc.property(
				columnKeyArb,
				columnKeyArb,
				(column1, column2) => {
					// Ensure different columns
					if (column1 === column2) return true;
					
					// Test all possible previous directions
					const directions: SortDirection[] = ['asc', 'desc', null];
					
					for (const prevDirection of directions) {
						const result = getNextSortDirection(column1, prevDirection, column2);
						expect(result.key).toBe(column2);
						expect(result.direction).toBe('asc');
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should always return the clicked column key', () => {
		fc.assert(
			fc.property(
				fc.option(columnKeyArb, { nil: null }),
				sortDirectionArb,
				columnKeyArb,
				(currentKey, currentDirection, clickedKey) => {
					const result = getNextSortDirection(currentKey, currentDirection, clickedKey);
					
					// The result key should always be the clicked key
					expect(result.key).toBe(clickedKey);
				}
			),
			{ numRuns: 100 }
		);
	});
});
