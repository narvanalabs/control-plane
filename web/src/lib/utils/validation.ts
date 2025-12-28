/**
 * Form validation utilities
 * Requirements: 11.3
 */

export interface ValidationResult {
	valid: boolean;
	error?: string;
}

/**
 * Validate that a field is not empty
 * 
 * @param value - The value to validate
 * @param fieldName - The name of the field for error messages
 * @returns ValidationResult with error message if invalid
 */
export function validateRequired(value: string | undefined | null, fieldName: string): ValidationResult {
	if (value === null || value === undefined || value.trim() === '') {
		return { valid: false, error: `${fieldName} is required` };
	}
	return { valid: true };
}

/**
 * Validate email format
 * 
 * @param email - The email to validate
 * @returns ValidationResult with error message if invalid
 */
export function validateEmail(email: string): ValidationResult {
	if (!email || email.trim() === '') {
		return { valid: false, error: 'Email is required' };
	}

	// Basic email regex pattern
	const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
	if (!emailPattern.test(email.trim())) {
		return { valid: false, error: 'Please enter a valid email address' };
	}

	return { valid: true };
}

/**
 * Password requirements configuration
 */
export interface PasswordRequirements {
	minLength?: number;
	requireUppercase?: boolean;
	requireLowercase?: boolean;
	requireNumber?: boolean;
	requireSpecial?: boolean;
}

const DEFAULT_PASSWORD_REQUIREMENTS: PasswordRequirements = {
	minLength: 8,
	requireUppercase: true,
	requireLowercase: true,
	requireNumber: true,
	requireSpecial: false
};

/**
 * Validate password meets requirements
 * 
 * @param password - The password to validate
 * @param requirements - Optional custom requirements
 * @returns ValidationResult with error message if invalid
 */
export function validatePassword(
	password: string,
	requirements: PasswordRequirements = DEFAULT_PASSWORD_REQUIREMENTS
): ValidationResult {
	const reqs = { ...DEFAULT_PASSWORD_REQUIREMENTS, ...requirements };

	if (!password) {
		return { valid: false, error: 'Password is required' };
	}

	if (reqs.minLength && password.length < reqs.minLength) {
		return { valid: false, error: `Password must be at least ${reqs.minLength} characters` };
	}

	if (reqs.requireUppercase && !/[A-Z]/.test(password)) {
		return { valid: false, error: 'Password must contain at least one uppercase letter' };
	}

	if (reqs.requireLowercase && !/[a-z]/.test(password)) {
		return { valid: false, error: 'Password must contain at least one lowercase letter' };
	}

	if (reqs.requireNumber && !/\d/.test(password)) {
		return { valid: false, error: 'Password must contain at least one number' };
	}

	if (reqs.requireSpecial && !/[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]/.test(password)) {
		return { valid: false, error: 'Password must contain at least one special character' };
	}

	return { valid: true };
}

/**
 * Validate that two passwords match
 * 
 * @param password - The password
 * @param confirmPassword - The confirmation password
 * @returns ValidationResult with error message if they don't match
 */
export function validatePasswordMatch(password: string, confirmPassword: string): ValidationResult {
	if (password !== confirmPassword) {
		return { valid: false, error: 'Passwords do not match' };
	}
	return { valid: true };
}

/**
 * Run multiple validators and return the first error
 * 
 * @param validators - Array of validation functions to run
 * @returns ValidationResult with first error found, or valid if all pass
 */
export function validateAll(...validators: (() => ValidationResult)[]): ValidationResult {
	for (const validator of validators) {
		const result = validator();
		if (!result.valid) {
			return result;
		}
	}
	return { valid: true };
}
