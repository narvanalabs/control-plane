import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { filterByFields, filterApplications, containsQuery } from './search';

describe('filterByFields', () => {
	/**
	 * Feature: professional-web-ui, Property 4: Application search filters correctly
	 * Validates: Requirements 4.3
	 *
	 * For any search query string and list of applications, the filtered results
	 * should only include applications where the name or description contains
	 * the search query (case-insensitive).
	 */
	it('should only return items where at least one field contains the query (case-insensitive)', () => {
		// Generator for application-like objects
		const appArbitrary = fc.record({
			name: fc.string({ minLength: 1, maxLength: 50 }),
			description: fc.option(fc.string({ maxLength: 100 }), { nil: undefined })
		});

		fc.assert(
			fc.property(
				fc.array(appArbitrary, { minLength: 0, maxLength: 20 }),
				fc.string({ minLength: 0, maxLength: 20 }),
				(apps, query) => {
					const results = filterByFields(apps, query, ['name', 'description']);

					// Property 1: All results should contain the query in at least one field
					const normalizedQuery = query.toLowerCase().trim();
					if (normalizedQuery !== '') {
						for (const result of results) {
							const nameMatches = result.name.toLowerCase().includes(normalizedQuery);
							const descMatches =
								result.description?.toLowerCase().includes(normalizedQuery) ?? false;
							expect(nameMatches || descMatches).toBe(true);
						}
					}

					// Property 2: No items that match should be excluded
					for (const app of apps) {
						const nameMatches = app.name.toLowerCase().includes(normalizedQuery);
						const descMatches = app.description?.toLowerCase().includes(normalizedQuery) ?? false;
						const shouldMatch = normalizedQuery === '' || nameMatches || descMatches;
						const isInResults = results.includes(app);
						expect(isInResults).toBe(shouldMatch);
					}
				}
			),
			{ numRuns: 100 }
		);
	});

	/**
	 * Empty query should return all items
	 */
	it('should return all items when query is empty', () => {
		const appArbitrary = fc.record({
			name: fc.string({ minLength: 1, maxLength: 50 }),
			description: fc.option(fc.string({ maxLength: 100 }), { nil: undefined })
		});

		fc.assert(
			fc.property(fc.array(appArbitrary, { minLength: 0, maxLength: 20 }), (apps) => {
				const resultsEmpty = filterByFields(apps, '', ['name', 'description']);
				const resultsWhitespace = filterByFields(apps, '   ', ['name', 'description']);

				expect(resultsEmpty).toEqual(apps);
				expect(resultsWhitespace).toEqual(apps);
			}),
			{ numRuns: 50 }
		);
	});
});

describe('filterApplications', () => {
	/**
	 * Convenience function should behave the same as filterByFields with name/description
	 */
	it('should filter applications by name and description', () => {
		const appArbitrary = fc.record({
			name: fc.string({ minLength: 1, maxLength: 50 }),
			description: fc.option(fc.string({ maxLength: 100 }), { nil: undefined })
		});

		fc.assert(
			fc.property(
				fc.array(appArbitrary, { minLength: 0, maxLength: 20 }),
				fc.string({ minLength: 0, maxLength: 20 }),
				(apps, query) => {
					const results = filterApplications(apps, query);
					const expected = filterByFields(apps, query, ['name', 'description']);
					expect(results).toEqual(expected);
				}
			),
			{ numRuns: 100 }
		);
	});
});

describe('containsQuery', () => {
	it('should be case-insensitive', () => {
		fc.assert(
			fc.property(fc.string({ minLength: 1, maxLength: 50 }), (text) => {
				// A string should always contain itself regardless of case
				expect(containsQuery(text, text.toLowerCase())).toBe(true);
				expect(containsQuery(text, text.toUpperCase())).toBe(true);
				expect(containsQuery(text.toLowerCase(), text)).toBe(true);
				expect(containsQuery(text.toUpperCase(), text)).toBe(true);
			}),
			{ numRuns: 100 }
		);
	});

	it('should handle null/undefined text', () => {
		expect(containsQuery(null, 'test')).toBe(false);
		expect(containsQuery(undefined, 'test')).toBe(false);
	});

	it('should return true for empty query', () => {
		fc.assert(
			fc.property(fc.string({ maxLength: 50 }), (text) => {
				expect(containsQuery(text, '')).toBe(true);
			}),
			{ numRuns: 50 }
		);
	});
});
