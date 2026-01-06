// Package utils provides utility functions for the web UI.
package utils

import (
	"strings"
)

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
			// Handle escape sequences in double-quoted strings
			if strings.Contains(value, "\\") {
				value = strings.ReplaceAll(value, "\\n", "\n")
				value = strings.ReplaceAll(value, "\\t", "\t")
				value = strings.ReplaceAll(value, "\\r", "\r")
				value = strings.ReplaceAll(value, "\\\"", "\"")
				value = strings.ReplaceAll(value, "\\\\", "\\")
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
