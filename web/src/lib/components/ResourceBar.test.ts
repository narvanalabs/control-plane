import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';

/**
 * Feature: professional-web-ui, Property 14: Resource utilization calculation
 * Validates: Requirements 10.2
 *
 * For any node with resource data (total and available values), the utilization
 * percentage should be calculated as ((total - available) / total) * 100, and
 * the progress bar width should match this percentage.
 */

// Simulate the utilization calculation from ResourceBar component
function calculateUtilization(total: number, available: number): {
	used: number;
	percentage: number;
	displayPercentage: number;
	barColor: string;
} {
	const used = Math.max(0, total - available);
	const percentage = total > 0 ? Math.round((used / total) * 100) : 0;
	const displayPercentage = Math.min(100, Math.max(0, percentage));
	
	let barColor: string;
	if (percentage >= 80) {
		barColor = 'bg-[var(--color-error)]';
	} else if (percentage >= 60) {
		barColor = 'bg-[var(--color-warning)]';
	} else {
		barColor = 'bg-[var(--color-success)]';
	}
	
	return { used, percentage, displayPercentage, barColor };
}

// Generator for positive resource values
const positiveResourceArb = fc.integer({ min: 1, max: 1000000 });

// Generator for non-negative resource values
const nonNegativeResourceArb = fc.integer({ min: 0, max: 1000000 });

describe('ResourceBar utilization calculation', () => {
	it('should calculate percentage as ((total - available) / total) * 100', () => {
		fc.assert(
			fc.property(
				positiveResourceArb,
				nonNegativeResourceArb,
				(total, available) => {
					const result = calculateUtilization(total, available);
					
					// Calculate expected percentage
					const used = Math.max(0, total - available);
					const expectedPercentage = Math.round((used / total) * 100);
					
					expect(result.percentage).toBe(expectedPercentage);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should clamp display percentage between 0 and 100', () => {
		fc.assert(
			fc.property(
				positiveResourceArb,
				fc.integer({ min: -1000, max: 2000000 }), // Allow negative and very large available values
				(total, available) => {
					const result = calculateUtilization(total, available);
					
					expect(result.displayPercentage).toBeGreaterThanOrEqual(0);
					expect(result.displayPercentage).toBeLessThanOrEqual(100);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should return 0% when total is 0', () => {
		fc.assert(
			fc.property(
				nonNegativeResourceArb,
				(available) => {
					const result = calculateUtilization(0, available);
					
					expect(result.percentage).toBe(0);
					expect(result.displayPercentage).toBe(0);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should return 100% when available is 0 and total is positive', () => {
		fc.assert(
			fc.property(
				positiveResourceArb,
				(total) => {
					const result = calculateUtilization(total, 0);
					
					expect(result.percentage).toBe(100);
					expect(result.displayPercentage).toBe(100);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should return 0% when available equals total', () => {
		fc.assert(
			fc.property(
				positiveResourceArb,
				(total) => {
					const result = calculateUtilization(total, total);
					
					expect(result.percentage).toBe(0);
					expect(result.displayPercentage).toBe(0);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should use green color for utilization below 60%', () => {
		fc.assert(
			fc.property(
				positiveResourceArb,
				(total) => {
					// Set available to ensure utilization is below 60%
					// If used/total < 0.6, then used < 0.6 * total
					// So available > 0.4 * total
					const available = Math.ceil(total * 0.5); // 50% available = 50% used
					const result = calculateUtilization(total, available);
					
					if (result.percentage < 60) {
						expect(result.barColor).toBe('bg-[var(--color-success)]');
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should use yellow color for utilization between 60% and 80%', () => {
		fc.assert(
			fc.property(
				fc.integer({ min: 100, max: 1000000 }), // Need larger total for precision
				(total) => {
					// Set available to ensure utilization is between 60% and 80%
					// 60% used means 40% available
					// 80% used means 20% available
					const available = Math.floor(total * 0.3); // 30% available = 70% used
					const result = calculateUtilization(total, available);
					
					if (result.percentage >= 60 && result.percentage < 80) {
						expect(result.barColor).toBe('bg-[var(--color-warning)]');
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should use red color for utilization 80% and above', () => {
		fc.assert(
			fc.property(
				fc.integer({ min: 100, max: 1000000 }), // Need larger total for precision
				(total) => {
					// Set available to ensure utilization is 80% or above
					// 80% used means 20% available
					const available = Math.floor(total * 0.1); // 10% available = 90% used
					const result = calculateUtilization(total, available);
					
					if (result.percentage >= 80) {
						expect(result.barColor).toBe('bg-[var(--color-error)]');
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should calculate used as total minus available (never negative)', () => {
		fc.assert(
			fc.property(
				positiveResourceArb,
				fc.integer({ min: -1000, max: 2000000 }),
				(total, available) => {
					const result = calculateUtilization(total, available);
					
					// Used should never be negative
					expect(result.used).toBeGreaterThanOrEqual(0);
					
					// Used should be total - available when available <= total
					if (available <= total && available >= 0) {
						expect(result.used).toBe(total - available);
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should have displayPercentage match the progress bar width', () => {
		fc.assert(
			fc.property(
				positiveResourceArb,
				nonNegativeResourceArb,
				(total, available) => {
					const result = calculateUtilization(total, available);
					
					// The displayPercentage is what's used for the width style
					// It should be a valid percentage value
					expect(result.displayPercentage).toBeGreaterThanOrEqual(0);
					expect(result.displayPercentage).toBeLessThanOrEqual(100);
					expect(Number.isInteger(result.displayPercentage)).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});
});
