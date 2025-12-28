import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { isFormDirty, resetForm, createFormSnapshot } from './form';

/**
 * Feature: professional-web-ui, Property 7: Form dirty state detection
 * Validates: Requirements 6.5
 *
 * For any form with initial values, modifying any field should set the dirty state
 * to true, and resetting to initial values should set dirty state to false.
 */

// Generator for primitive form field values
const primitiveValueArb = fc.oneof(
	fc.string({ minLength: 0, maxLength: 100 }),
	fc.integer({ min: -1000, max: 1000 }),
	fc.boolean(),
	fc.constant(null),
	fc.constant(undefined)
);

// Generator for simple form objects (string keys, primitive values)
const simpleFormArb = fc.dictionary(
	fc.string({ minLength: 1, maxLength: 20 }).filter(s => /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(s)),
	primitiveValueArb,
	{ minKeys: 1, maxKeys: 10 }
);

// Generator for service form (matching the actual form structure)
const serviceFormArb = fc.record({
	name: fc.string({ minLength: 0, maxLength: 50 }),
	git_repo: fc.string({ minLength: 0, maxLength: 200 }),
	git_ref: fc.string({ minLength: 0, maxLength: 50 }),
	build_strategy: fc.constantFrom('auto', 'flake', 'auto-go', 'auto-node', 'auto-python', 'auto-rust', 'dockerfile', 'nixpacks'),
	resource_tier: fc.constantFrom('nano', 'small', 'medium', 'large', 'xlarge'),
	replicas: fc.integer({ min: 1, max: 10 }),
	port: fc.integer({ min: 0, max: 65535 }),
});

// Generator for a modified version of a form (at least one field changed)
function modifiedFormArb<T extends Record<string, unknown>>(originalForm: T) {
	const keys = Object.keys(originalForm) as Array<keyof T>;
	if (keys.length === 0) {
		return fc.constant(originalForm);
	}
	
	// Pick a random key to modify
	return fc.constantFrom(...keys).chain(keyToModify => {
		const originalValue = originalForm[keyToModify];
		
		// Generate a different value for the selected key
		let newValueArb: fc.Arbitrary<unknown>;
		
		if (typeof originalValue === 'string') {
			newValueArb = fc.string({ minLength: 0, maxLength: 100 })
				.filter(v => v !== originalValue);
		} else if (typeof originalValue === 'number') {
			newValueArb = fc.integer({ min: -1000, max: 1000 })
				.filter(v => v !== originalValue);
		} else if (typeof originalValue === 'boolean') {
			newValueArb = fc.constant(!originalValue);
		} else {
			// For null/undefined, change to a string
			newValueArb = fc.string({ minLength: 1, maxLength: 50 });
		}
		
		return newValueArb.map(newValue => ({
			...originalForm,
			[keyToModify]: newValue,
		}));
	});
}

describe('Form dirty state detection', () => {
	/**
	 * Feature: professional-web-ui, Property 7: Form dirty state detection
	 * Validates: Requirements 6.5
	 */
	it('should return false when current values equal initial values', () => {
		fc.assert(
			fc.property(serviceFormArb, (form) => {
				const initialValues = createFormSnapshot(form);
				const currentValues = createFormSnapshot(form);
				
				// When values are identical, form should not be dirty
				expect(isFormDirty(currentValues, initialValues)).toBe(false);
			}),
			{ numRuns: 100 }
		);
	});

	it('should return true when any field is modified', () => {
		fc.assert(
			fc.property(
				serviceFormArb.chain(form => 
					modifiedFormArb(form).map(modified => ({ original: form, modified }))
				),
				({ original, modified }) => {
					const initialValues = createFormSnapshot(original);
					
					// When any field is modified, form should be dirty
					expect(isFormDirty(modified, initialValues)).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should return false after resetting to initial values', () => {
		fc.assert(
			fc.property(
				serviceFormArb.chain(form => 
					modifiedFormArb(form).map(modified => ({ original: form, modified }))
				),
				({ original, modified }) => {
					const initialValues = createFormSnapshot(original);
					
					// First verify it's dirty
					expect(isFormDirty(modified, initialValues)).toBe(true);
					
					// After reset, should not be dirty
					const resetValues = resetForm(initialValues);
					expect(isFormDirty(resetValues, initialValues)).toBe(false);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should detect changes in string fields', () => {
		fc.assert(
			fc.property(
				serviceFormArb,
				fc.string({ minLength: 1, maxLength: 50 }),
				(form, newName) => {
					// Skip if the new name happens to be the same
					fc.pre(newName !== form.name);
					
					const initialValues = createFormSnapshot(form);
					const modifiedValues = { ...form, name: newName };
					
					expect(isFormDirty(modifiedValues, initialValues)).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should detect changes in number fields', () => {
		fc.assert(
			fc.property(
				serviceFormArb,
				fc.integer({ min: 1, max: 10 }),
				(form, newReplicas) => {
					// Skip if the new value happens to be the same
					fc.pre(newReplicas !== form.replicas);
					
					const initialValues = createFormSnapshot(form);
					const modifiedValues = { ...form, replicas: newReplicas };
					
					expect(isFormDirty(modifiedValues, initialValues)).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should detect changes in enum/select fields', () => {
		fc.assert(
			fc.property(
				serviceFormArb,
				fc.constantFrom('nano', 'small', 'medium', 'large', 'xlarge'),
				(form, newTier) => {
					// Skip if the new value happens to be the same
					fc.pre(newTier !== form.resource_tier);
					
					const initialValues = createFormSnapshot(form);
					const modifiedValues = { ...form, resource_tier: newTier };
					
					expect(isFormDirty(modifiedValues, initialValues)).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should handle empty forms correctly', () => {
		const emptyForm = {};
		const initialValues = createFormSnapshot(emptyForm);
		
		// Empty form should not be dirty
		expect(isFormDirty(emptyForm, initialValues)).toBe(false);
	});

	it('should handle forms with array fields', () => {
		fc.assert(
			fc.property(
				fc.record({
					name: fc.string({ minLength: 1, maxLength: 50 }),
					tags: fc.array(fc.string({ minLength: 1, maxLength: 20 }), { minLength: 0, maxLength: 5 }),
				}),
				(form) => {
					const initialValues = createFormSnapshot(form);
					const currentValues = createFormSnapshot(form);
					
					// Identical arrays should not be dirty
					expect(isFormDirty(currentValues, initialValues)).toBe(false);
					
					// Modified array should be dirty
					const modifiedValues = { ...form, tags: [...form.tags, 'new-tag'] };
					expect(isFormDirty(modifiedValues, initialValues)).toBe(true);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should create independent snapshots', () => {
		fc.assert(
			fc.property(serviceFormArb, (form) => {
				const snapshot1 = createFormSnapshot(form);
				const snapshot2 = createFormSnapshot(form);
				
				// Snapshots should be equal but not the same reference
				expect(snapshot1).toEqual(snapshot2);
				expect(snapshot1).not.toBe(snapshot2);
				
				// Modifying one snapshot should not affect the other
				snapshot1.name = 'modified';
				expect(snapshot2.name).toBe(form.name);
			}),
			{ numRuns: 100 }
		);
	});

	it('should correctly reset form to initial values', () => {
		fc.assert(
			fc.property(serviceFormArb, (form) => {
				const initialValues = createFormSnapshot(form);
				const resetValues = resetForm(initialValues);
				
				// Reset values should equal initial values
				expect(resetValues).toEqual(initialValues);
				
				// But should be a different reference
				expect(resetValues).not.toBe(initialValues);
			}),
			{ numRuns: 100 }
		);
	});
});
