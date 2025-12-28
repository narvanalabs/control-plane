import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { formatRelativeTime } from './formatters';

describe('formatRelativeTime', () => {
	/**
	 * Feature: professional-web-ui, Property 15: Relative time formatting
	 * Validates: Requirements 10.3
	 * 
	 * For any timestamp, the relative time formatter should produce correct
	 * human-readable strings: "just now" for <60s, "Xm ago" for <1h,
	 * "Xh ago" for <24h, "Xd ago" for >=24h.
	 */
	it('should format timestamps correctly based on elapsed time', () => {
		fc.assert(
			fc.property(
				// Generate seconds elapsed from 0 to ~1 year
				fc.integer({ min: 0, max: 86400 * 365 }),
				(secondsAgo) => {
					const timestamp = new Date(Date.now() - secondsAgo * 1000).toISOString();
					const result = formatRelativeTime(timestamp);

					if (secondsAgo < 60) {
						expect(result).toBe('just now');
					} else if (secondsAgo < 3600) {
						// Minutes: 1-59
						const expectedMinutes = Math.floor(secondsAgo / 60);
						expect(result).toBe(`${expectedMinutes}m ago`);
					} else if (secondsAgo < 86400) {
						// Hours: 1-23
						const expectedHours = Math.floor(secondsAgo / 3600);
						expect(result).toBe(`${expectedHours}h ago`);
					} else {
						// Days: 1+
						const expectedDays = Math.floor(secondsAgo / 86400);
						expect(result).toBe(`${expectedDays}d ago`);
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	/**
	 * Edge case: future timestamps should return "just now"
	 */
	it('should handle future timestamps gracefully', () => {
		fc.assert(
			fc.property(
				fc.integer({ min: 1, max: 86400 * 365 }),
				(secondsInFuture) => {
					const timestamp = new Date(Date.now() + secondsInFuture * 1000).toISOString();
					const result = formatRelativeTime(timestamp);
					expect(result).toBe('just now');
				}
			),
			{ numRuns: 50 }
		);
	});
});
