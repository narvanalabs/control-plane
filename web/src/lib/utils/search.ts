/**
 * Utility functions for search and filtering
 * Requirements: 4.3
 */

/**
 * Check if a string contains a search query (case-insensitive)
 * 
 * @param text - The text to search in
 * @param query - The search query
 * @returns true if text contains query (case-insensitive)
 */
export function containsQuery(text: string | undefined | null, query: string): boolean {
	if (text === null || text === undefined) return false;
	if (!query) return true;
	return text.toLowerCase().includes(query.toLowerCase());
}

/**
 * Filter items by searching across multiple fields (case-insensitive)
 * Returns items where at least one of the specified fields contains the query
 * 
 * @param items - Array of items to filter
 * @param query - The search query
 * @param fields - Array of field names to search in
 * @returns Filtered array of items matching the query
 */
export function filterByFields<T extends Record<string, unknown>>(
	items: T[],
	query: string,
	fields: (keyof T)[]
): T[] {
	if (!query || query.trim() === '') {
		return items;
	}

	const normalizedQuery = query.toLowerCase().trim();

	return items.filter((item) =>
		fields.some((field) => {
			const value = item[field];
			if (typeof value === 'string') {
				return value.toLowerCase().includes(normalizedQuery);
			}
			if (typeof value === 'number') {
				return String(value).includes(normalizedQuery);
			}
			return false;
		})
	);
}

/**
 * Filter applications by name or description
 * Convenience function for the common use case of filtering applications
 * 
 * @param apps - Array of application objects
 * @param query - The search query
 * @returns Filtered array of applications
 */
export function filterApplications<T extends { name: string; description?: string | null }>(
	apps: T[],
	query: string
): T[] {
	return filterByFields(apps, query, ['name', 'description'] as (keyof T)[]);
}
