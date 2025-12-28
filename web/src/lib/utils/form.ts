/**
 * Form utility functions
 * Requirements: 6.5
 */

/**
 * Check if a form has unsaved changes by comparing current values to initial values
 * 
 * @param currentValues - Current form field values
 * @param initialValues - Initial form field values when form was opened
 * @returns true if any field has changed, false otherwise
 */
export function isFormDirty<T extends Record<string, unknown>>(
	currentValues: T,
	initialValues: T
): boolean {
	const keys = Object.keys(currentValues) as Array<keyof T>;
	
	for (const key of keys) {
		const currentValue = currentValues[key];
		const initialValue = initialValues[key];
		
		// Handle arrays
		if (Array.isArray(currentValue) && Array.isArray(initialValue)) {
			if (currentValue.length !== initialValue.length) {
				return true;
			}
			// Simple array comparison (works for primitives)
			if (JSON.stringify(currentValue) !== JSON.stringify(initialValue)) {
				return true;
			}
			continue;
		}
		
		// Handle objects (shallow comparison)
		if (
			typeof currentValue === 'object' && 
			currentValue !== null && 
			typeof initialValue === 'object' && 
			initialValue !== null
		) {
			if (JSON.stringify(currentValue) !== JSON.stringify(initialValue)) {
				return true;
			}
			continue;
		}
		
		// Handle primitives
		if (currentValue !== initialValue) {
			return true;
		}
	}
	
	return false;
}

/**
 * Reset form values to initial state
 * 
 * @param initialValues - Initial form field values
 * @returns A copy of the initial values
 */
export function resetForm<T extends Record<string, unknown>>(initialValues: T): T {
	return { ...initialValues };
}

/**
 * Create a snapshot of form values for dirty state comparison
 * 
 * @param values - Current form values
 * @returns A deep copy of the values
 */
export function createFormSnapshot<T extends Record<string, unknown>>(values: T): T {
	return JSON.parse(JSON.stringify(values));
}
