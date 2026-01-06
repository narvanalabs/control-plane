// Package utils provides utility functions for the web UI.
package utils

import (
	"strings"
)

// unescapeValue processes escape sequences in a single pass to handle them correctly.
// This ensures that \\n becomes \n (literal backslash-n) not a newline.
func unescapeValue(s string) string {
	var result strings.Builder
	result.Grow(len(s))
	
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				result.WriteByte('\n')
				i += 2
			case 't':
				result.WriteByte('\t')
				i += 2
			case 'r':
				result.WriteByte('\r')
				i += 2
			case '"':
				result.WriteByte('"')
				i += 2
			case '\\':
				result.WriteByte('\\')
				i += 2
			default:
				// Unknown escape sequence, keep the backslash
				result.WriteByte(s[i])
				i++
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// ParseEnvFile parses a .env file content and returns a map of key-value pairs.
// This implementation mirrors the JavaScript parseEnvFile function in env_form.templ.
// **Validates: Requirements 2.2, 2.3**
func ParseEnvFile(content string) map[string]string {
	vars := make(map[string]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Handle export prefix
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(trimmed[7:])
		}

		// Find the first equals sign
		eqIndex := strings.Index(trimmed, "=")
		if eqIndex <= 0 {
			continue
		}

		key := strings.TrimSpace(trimmed[:eqIndex])
		value := trimmed[eqIndex+1:]

		// Handle quoted values
		value = strings.TrimSpace(value)
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			value = value[1 : len(value)-1]
			// Handle escape sequences in double-quoted strings using single-pass processing
			if strings.Contains(value, "\\") {
				value = unescapeValue(value)
			}
		}

		vars[key] = value
	}

	return vars
}


// SerializeEnvFile serializes a map of key-value pairs to .env file format.
// This is used for round-trip testing of the parser.
// **Validates: Requirements 2.2, 2.3**
func SerializeEnvFile(vars map[string]string) string {
	var lines []string
	for key, value := range vars {
		// Quote values that contain special characters
		needsQuotes := strings.ContainsAny(value, " \t\n\r\"'#=")
		if needsQuotes {
			// Escape special characters
			escaped := value
			escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
			escaped = strings.ReplaceAll(escaped, "\n", "\\n")
			escaped = strings.ReplaceAll(escaped, "\t", "\\t")
			escaped = strings.ReplaceAll(escaped, "\r", "\\r")
			lines = append(lines, key+"=\""+escaped+"\"")
		} else {
			lines = append(lines, key+"="+value)
		}
	}
	return strings.Join(lines, "\n")
}
