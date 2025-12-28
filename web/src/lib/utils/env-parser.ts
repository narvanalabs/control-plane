/**
 * Utility functions for parsing .env file format
 * Requirements: 9.5
 */

/**
 * Parse .env file content into a key-value record
 * 
 * Handles:
 * - KEY=value format
 * - Quoted values (single and double quotes)
 * - Comments (lines starting with #)
 * - Empty lines
 * - Inline comments after values
 * 
 * @param content - The .env file content as a string
 * @returns Record of key-value pairs
 */
export function parseEnvFile(content: string): Record<string, string> {
	const result: Record<string, string> = {};
	const lines = content.split(/\r?\n/);

	for (const line of lines) {
		const trimmed = line.trim();

		// Skip empty lines and comments
		if (!trimmed || trimmed.startsWith('#')) {
			continue;
		}

		// Find the first = sign
		const equalIndex = trimmed.indexOf('=');
		if (equalIndex === -1) {
			continue;
		}

		const key = trimmed.slice(0, equalIndex).trim();
		let value = trimmed.slice(equalIndex + 1);

		// Skip if key is empty
		if (!key) {
			continue;
		}

		// Handle quoted values
		value = value.trim();
		if ((value.startsWith('"') && value.endsWith('"')) || 
		    (value.startsWith("'") && value.endsWith("'"))) {
			// Remove quotes
			value = value.slice(1, -1);
		} else {
			// Remove inline comments for unquoted values
			const commentIndex = value.indexOf('#');
			if (commentIndex !== -1) {
				value = value.slice(0, commentIndex).trim();
			}
		}

		result[key] = value;
	}

	return result;
}

/**
 * Serialize a key-value record to .env file format
 * 
 * @param env - Record of key-value pairs
 * @returns .env file content as a string
 */
export function serializeEnvFile(env: Record<string, string>): string {
	const lines: string[] = [];

	for (const [key, value] of Object.entries(env)) {
		// Quote values that contain spaces, #, or quotes
		const needsQuotes = /[\s#"']/.test(value);
		const formattedValue = needsQuotes ? `"${value.replace(/"/g, '\\"')}"` : value;
		lines.push(`${key}=${formattedValue}`);
	}

	return lines.join('\n');
}
