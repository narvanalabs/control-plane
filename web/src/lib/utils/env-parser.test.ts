import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import { parseEnvFile, serializeEnvFile } from './env-parser';

describe('parseEnvFile', () => {
	/**
	 * Feature: professional-web-ui, Property 12: Environment file parsing
	 * Validates: Requirements 9.5
	 *
	 * For any valid .env file content with KEY=value lines, parsing should
	 * produce a record where each key maps to its corresponding value,
	 * handling quoted values and comments correctly.
	 */
	it('should round-trip parse and serialize env records', () => {
		// Generator for valid env keys (alphanumeric + underscore, starting with letter/underscore)
		const envKeyArb = fc
			.tuple(
				fc.constantFrom('A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', '_'),
				fc.array(
					fc.constantFrom(
						...'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_'.split('')
					),
					{ minLength: 0, maxLength: 20 }
				).map(arr => arr.join(''))
			)
			.map(([first, rest]) => first + rest);

		// Generator for env values (avoid newlines and problematic characters)
		const envValueArb = fc.array(
			fc.constantFrom(
				...'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-_/=+:@'.split('')
			),
			{ minLength: 0, maxLength: 50 }
		).map(arr => arr.join(''));

		// Generator for env records
		const envRecordArb = fc.dictionary(envKeyArb, envValueArb, { minKeys: 0, maxKeys: 10 });

		fc.assert(
			fc.property(envRecordArb, (env) => {
				const serialized = serializeEnvFile(env);
				const parsed = parseEnvFile(serialized);

				// Round-trip should preserve all key-value pairs
				expect(parsed).toEqual(env);
			}),
			{ numRuns: 100 }
		);
	});

	it('should skip comment lines', () => {
		fc.assert(
			fc.property(
				fc.array(fc.constantFrom(...'abcdefghijklmnopqrstuvwxyz '.split('')), {
					minLength: 1,
					maxLength: 30
				}).map(arr => arr.join('')),
				(comment) => {
					const content = `# ${comment}\nKEY=value`;
					const result = parseEnvFile(content);
					expect(result).toEqual({ KEY: 'value' });
				}
			),
			{ numRuns: 50 }
		);
	});

	it('should skip empty lines', () => {
		const content = `KEY1=value1\n\n\nKEY2=value2`;
		const result = parseEnvFile(content);
		expect(result).toEqual({ KEY1: 'value1', KEY2: 'value2' });
	});

	it('should handle quoted values with spaces', () => {
		const content = `KEY="hello world"`;
		const result = parseEnvFile(content);
		expect(result).toEqual({ KEY: 'hello world' });
	});

	it('should handle single-quoted values', () => {
		const content = `KEY='hello world'`;
		const result = parseEnvFile(content);
		expect(result).toEqual({ KEY: 'hello world' });
	});

	it('should handle inline comments for unquoted values', () => {
		const content = `KEY=value # this is a comment`;
		const result = parseEnvFile(content);
		expect(result).toEqual({ KEY: 'value' });
	});

	it('should preserve # in quoted values', () => {
		const content = `KEY="value # not a comment"`;
		const result = parseEnvFile(content);
		expect(result).toEqual({ KEY: 'value # not a comment' });
	});
});

describe('serializeEnvFile', () => {
	it('should quote values containing spaces', () => {
		const env = { KEY: 'hello world' };
		const result = serializeEnvFile(env);
		expect(result).toBe('KEY="hello world"');
	});

	it('should quote values containing #', () => {
		const env = { KEY: 'value#hash' };
		const result = serializeEnvFile(env);
		expect(result).toBe('KEY="value#hash"');
	});

	it('should not quote simple values', () => {
		const env = { KEY: 'simplevalue' };
		const result = serializeEnvFile(env);
		expect(result).toBe('KEY=simplevalue');
	});
});
