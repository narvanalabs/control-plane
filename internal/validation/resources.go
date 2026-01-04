package validation

import (
	"regexp"

	"github.com/narvanalabs/control-plane/internal/models"
)

// cpuRegex validates CPU format: decimal numbers like "0.5", "1", "2", "0.25", "1.5"
// Accepts: positive decimal numbers with optional decimal places
var cpuRegex = regexp.MustCompile(`^(0\.[0-9]+|[1-9][0-9]*(\.[0-9]+)?|0)$`)

// memoryRegex validates memory format: numbers followed by unit suffix
// Accepts: Ki, Mi, Gi, Ti (binary) or K, M, G, T (decimal)
// Examples: "256Mi", "1Gi", "512M", "2G"
var memoryRegex = regexp.MustCompile(`^[1-9][0-9]*(Ki|Mi|Gi|Ti|K|M|G|T)$`)

// ValidateResourceSpec validates a resource specification.
// Requirements: 12.2
//
// CPU format: decimal numbers like "0.5", "1", "2"
// Memory format: numbers with unit suffix like "256Mi", "1Gi"
func ValidateResourceSpec(spec *models.ResourceSpec) error {
	if spec == nil {
		return nil // nil spec is valid (will use defaults)
	}

	// Validate CPU if specified
	if spec.CPU != "" {
		if !cpuRegex.MatchString(spec.CPU) {
			return &models.ValidationError{
				Field:   "resources.cpu",
				Message: "CPU must be a valid decimal number (e.g., \"0.5\", \"1\", \"2\")",
			}
		}
	}

	// Validate Memory if specified
	if spec.Memory != "" {
		if !memoryRegex.MatchString(spec.Memory) {
			return &models.ValidationError{
				Field:   "resources.memory",
				Message: "memory must be a valid size with unit suffix (e.g., \"256Mi\", \"1Gi\")",
			}
		}
	}

	return nil
}

// ValidateCPU validates a CPU specification string.
func ValidateCPU(cpu string) error {
	if cpu == "" {
		return nil
	}
	if !cpuRegex.MatchString(cpu) {
		return &models.ValidationError{
			Field:   "cpu",
			Message: "CPU must be a valid decimal number (e.g., \"0.5\", \"1\", \"2\")",
		}
	}
	return nil
}

// ValidateMemory validates a memory specification string.
func ValidateMemory(memory string) error {
	if memory == "" {
		return nil
	}
	if !memoryRegex.MatchString(memory) {
		return &models.ValidationError{
			Field:   "memory",
			Message: "memory must be a valid size with unit suffix (e.g., \"256Mi\", \"1Gi\")",
		}
	}
	return nil
}
