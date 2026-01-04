package validation

import (
	"fmt"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: backend-source-of-truth, Property 12: Resource Format Validation**
// For any direct resource specification, CPU values SHALL match the format "0.5", "1", "2" etc.,
// and memory values SHALL match the format "256Mi", "1Gi" etc.
// **Validates: Requirements 12.2**

// genValidCPU generates a valid CPU specification.
func genValidCPU() gopter.Gen {
	return gen.OneGenOf(
		// Integer CPU values: "1", "2", "4", etc.
		gen.IntRange(1, 64).Map(func(n int) string {
			return fmt.Sprintf("%d", n)
		}),
		// Decimal CPU values: "0.5", "0.25", "1.5", etc.
		gen.IntRange(1, 99).Map(func(n int) string {
			return fmt.Sprintf("0.%d", n)
		}),
		// Decimal with integer part: "1.5", "2.25", etc.
		gopter.CombineGens(
			gen.IntRange(1, 16),
			gen.IntRange(1, 99),
		).Map(func(vals []interface{}) string {
			return fmt.Sprintf("%d.%d", vals[0].(int), vals[1].(int))
		}),
	)
}

// genInvalidCPU generates an invalid CPU specification.
func genInvalidCPU() gopter.Gen {
	return gen.OneGenOf(
		// Negative values
		gen.IntRange(-100, -1).Map(func(n int) string {
			return fmt.Sprintf("%d", n)
		}),
		// Invalid format with letters
		gen.Const("1cpu"),
		gen.Const("cpu1"),
		gen.Const("one"),
		// Invalid format with special chars
		gen.Const("1.5.5"),
		gen.Const("1,5"),
		gen.Const("1 5"),
		// Empty is valid (uses defaults), so we test other invalid formats
		gen.Const("..5"),
		gen.Const("."),
	)
}

// genValidMemory generates a valid memory specification.
func genValidMemory() gopter.Gen {
	return gen.OneGenOf(
		// Binary units: Mi, Gi, Ki, Ti
		gopter.CombineGens(
			gen.IntRange(1, 1024),
			gen.OneConstOf("Ki", "Mi", "Gi", "Ti"),
		).Map(func(vals []interface{}) string {
			return fmt.Sprintf("%d%s", vals[0].(int), vals[1].(string))
		}),
		// Decimal units: K, M, G, T
		gopter.CombineGens(
			gen.IntRange(1, 1024),
			gen.OneConstOf("K", "M", "G", "T"),
		).Map(func(vals []interface{}) string {
			return fmt.Sprintf("%d%s", vals[0].(int), vals[1].(string))
		}),
	)
}

// genInvalidMemory generates an invalid memory specification.
func genInvalidMemory() gopter.Gen {
	return gen.OneGenOf(
		// Missing unit
		gen.IntRange(1, 1024).Map(func(n int) string {
			return fmt.Sprintf("%d", n)
		}),
		// Invalid unit
		gen.IntRange(1, 1024).Map(func(n int) string {
			return fmt.Sprintf("%dB", n)
		}),
		gen.IntRange(1, 1024).Map(func(n int) string {
			return fmt.Sprintf("%dmb", n) // lowercase
		}),
		// Zero value
		gen.Const("0Mi"),
		// Negative value
		gen.Const("-256Mi"),
		// Invalid format
		gen.Const("256 Mi"),
		gen.Const("Mi256"),
		gen.Const("256mi"), // lowercase unit
	)
}

// TestResourceFormatValidation tests Property 12: Resource Format Validation.
func TestResourceFormatValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 12.1: Valid CPU formats are accepted
	properties.Property("valid CPU formats are accepted", prop.ForAll(
		func(cpu string) bool {
			err := ValidateCPU(cpu)
			return err == nil
		},
		genValidCPU(),
	))

	// Property 12.2: Invalid CPU formats are rejected
	properties.Property("invalid CPU formats are rejected", prop.ForAll(
		func(cpu string) bool {
			err := ValidateCPU(cpu)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "cpu"
		},
		genInvalidCPU(),
	))

	// Property 12.3: Valid memory formats are accepted
	properties.Property("valid memory formats are accepted", prop.ForAll(
		func(memory string) bool {
			err := ValidateMemory(memory)
			return err == nil
		},
		genValidMemory(),
	))

	// Property 12.4: Invalid memory formats are rejected
	properties.Property("invalid memory formats are rejected", prop.ForAll(
		func(memory string) bool {
			err := ValidateMemory(memory)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "memory"
		},
		genInvalidMemory(),
	))

	// Property 12.5: Empty CPU is valid (uses defaults)
	properties.Property("empty CPU is valid", prop.ForAll(
		func(_ int) bool {
			err := ValidateCPU("")
			return err == nil
		},
		gen.IntRange(0, 1),
	))

	// Property 12.6: Empty memory is valid (uses defaults)
	properties.Property("empty memory is valid", prop.ForAll(
		func(_ int) bool {
			err := ValidateMemory("")
			return err == nil
		},
		gen.IntRange(0, 1),
	))

	// Property 12.7: Nil ResourceSpec is valid
	properties.Property("nil ResourceSpec is valid", prop.ForAll(
		func(_ int) bool {
			err := ValidateResourceSpec(nil)
			return err == nil
		},
		gen.IntRange(0, 1),
	))

	// Property 12.8: Valid ResourceSpec with both fields is accepted
	properties.Property("valid ResourceSpec is accepted", prop.ForAll(
		func(cpu string, memory string) bool {
			spec := &models.ResourceSpec{
				CPU:    cpu,
				Memory: memory,
			}
			err := ValidateResourceSpec(spec)
			return err == nil
		},
		genValidCPU(),
		genValidMemory(),
	))

	// Property 12.9: ResourceSpec with invalid CPU is rejected
	properties.Property("ResourceSpec with invalid CPU is rejected", prop.ForAll(
		func(cpu string, memory string) bool {
			spec := &models.ResourceSpec{
				CPU:    cpu,
				Memory: memory,
			}
			err := ValidateResourceSpec(spec)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "resources.cpu"
		},
		genInvalidCPU(),
		genValidMemory(),
	))

	// Property 12.10: ResourceSpec with invalid memory is rejected
	properties.Property("ResourceSpec with invalid memory is rejected", prop.ForAll(
		func(cpu string, memory string) bool {
			spec := &models.ResourceSpec{
				CPU:    cpu,
				Memory: memory,
			}
			err := ValidateResourceSpec(spec)
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "resources.memory"
		},
		genValidCPU(),
		genInvalidMemory(),
	))

	properties.TestingRun(t)
}
