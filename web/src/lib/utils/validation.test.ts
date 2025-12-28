import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import {
	validateRequired,
	validateEmail,
	validatePassword,
	validatePasswordMatch,
	validateAll
} from './validation';

describe('validateRequired', () => {
	/**
	 * Feature: professional-web-ui, Property 16: Form validation error display
	 * Validates: Requirements 11.3
	 *
	 * For any form field with validation rules and invalid input,
	 * an error message should be displayed below the input field.
	 */
	it('should return error for empty/whitespace values', () => {
		fc.assert(
			fc.property(
				fc.constantFrom('', '   ', '\t', '\n', '  \t\n  '),
				fc.string({ minLength: 1, maxLength: 20 }),
				(emptyValue, fieldName) => {
					const result = validateRequired(emptyValue, fieldName);
					expect(result.valid).toBe(false);
					expect(result.error).toBeDefined();
					expect(result.error).toContain(fieldName);
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should return valid for non-empty values', () => {
		fc.assert(
			fc.property(
				fc.string({ minLength: 1, maxLength: 100 }).filter((s) => s.trim().length > 0),
				fc.string({ minLength: 1, maxLength: 20 }),
				(value, fieldName) => {
					const result = validateRequired(value, fieldName);
					expect(result.valid).toBe(true);
					expect(result.error).toBeUndefined();
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should handle null and undefined', () => {
		expect(validateRequired(null, 'Field').valid).toBe(false);
		expect(validateRequired(undefined, 'Field').valid).toBe(false);
	});
});

describe('validateEmail', () => {
	it('should return error for invalid email formats', () => {
		const invalidEmails = [
			'notanemail',
			'missing@domain',
			'@nodomain.com',
			'spaces in@email.com',
			'double@@at.com'
		];

		for (const email of invalidEmails) {
			const result = validateEmail(email);
			expect(result.valid).toBe(false);
			expect(result.error).toBeDefined();
		}
	});

	it('should return valid for valid email formats', () => {
		// Generator for valid-ish emails
		const localPartArb = fc.array(
			fc.constantFrom(...'abcdefghijklmnopqrstuvwxyz0123456789._-'.split('')),
			{ minLength: 1, maxLength: 20 }
		).map(arr => arr.join(''));

		const domainArb = fc.array(
			fc.constantFrom(...'abcdefghijklmnopqrstuvwxyz0123456789-'.split('')),
			{ minLength: 1, maxLength: 15 }
		).map(arr => arr.join(''));

		const tldArb = fc.constantFrom('com', 'org', 'net', 'io', 'dev');

		fc.assert(
			fc.property(localPartArb, domainArb, tldArb, (local, domain, tld) => {
				const email = `${local}@${domain}.${tld}`;
				const result = validateEmail(email);
				expect(result.valid).toBe(true);
			}),
			{ numRuns: 100 }
		);
	});

	it('should return error for empty email', () => {
		expect(validateEmail('').valid).toBe(false);
		expect(validateEmail('   ').valid).toBe(false);
	});
});

describe('validatePassword', () => {
	it('should return error when password is too short', () => {
		fc.assert(
			fc.property(
				fc.string({ minLength: 1, maxLength: 7 }),
				(shortPassword) => {
					const result = validatePassword(shortPassword, { minLength: 8 });
					expect(result.valid).toBe(false);
					expect(result.error).toContain('8 characters');
				}
			),
			{ numRuns: 50 }
		);
	});

	it('should return error when missing uppercase', () => {
		const result = validatePassword('lowercase123', { requireUppercase: true });
		expect(result.valid).toBe(false);
		expect(result.error).toContain('uppercase');
	});

	it('should return error when missing lowercase', () => {
		const result = validatePassword('UPPERCASE123', { requireLowercase: true });
		expect(result.valid).toBe(false);
		expect(result.error).toContain('lowercase');
	});

	it('should return error when missing number', () => {
		const result = validatePassword('NoNumbersHere', { requireNumber: true });
		expect(result.valid).toBe(false);
		expect(result.error).toContain('number');
	});

	it('should return error when missing special character', () => {
		const result = validatePassword('NoSpecial123', { requireSpecial: true });
		expect(result.valid).toBe(false);
		expect(result.error).toContain('special');
	});

	it('should return valid for password meeting all requirements', () => {
		const result = validatePassword('ValidPass123!', {
			minLength: 8,
			requireUppercase: true,
			requireLowercase: true,
			requireNumber: true,
			requireSpecial: true
		});
		expect(result.valid).toBe(true);
	});
});

describe('validatePasswordMatch', () => {
	it('should return error when passwords do not match', () => {
		fc.assert(
			fc.property(
				fc.string({ minLength: 1, maxLength: 50 }),
				fc.string({ minLength: 1, maxLength: 50 }),
				(password1, password2) => {
					fc.pre(password1 !== password2);
					const result = validatePasswordMatch(password1, password2);
					expect(result.valid).toBe(false);
					expect(result.error).toContain('match');
				}
			),
			{ numRuns: 100 }
		);
	});

	it('should return valid when passwords match', () => {
		fc.assert(
			fc.property(fc.string({ minLength: 0, maxLength: 50 }), (password) => {
				const result = validatePasswordMatch(password, password);
				expect(result.valid).toBe(true);
			}),
			{ numRuns: 100 }
		);
	});
});

describe('validateAll', () => {
	it('should return first error when multiple validations fail', () => {
		const result = validateAll(
			() => validateRequired('', 'Name'),
			() => validateEmail('invalid'),
			() => validatePassword('short')
		);
		expect(result.valid).toBe(false);
		expect(result.error).toContain('Name');
	});

	it('should return valid when all validations pass', () => {
		const result = validateAll(
			() => validateRequired('John', 'Name'),
			() => validateEmail('john@example.com'),
			() => validatePassword('ValidPass123')
		);
		expect(result.valid).toBe(true);
	});
});
