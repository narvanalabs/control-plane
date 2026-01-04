package validation

import (
	"log/slog"
	"os"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/narvanalabs/control-plane/internal/models"
)

// **Feature: backend-source-of-truth, Property 8: Circular Dependency Detection**
// For any service dependency graph, if adding or updating dependencies would create
// a cycle, the operation SHALL be rejected with an error listing the cycle path.
// **Validates: Requirements 9.1**

// genServiceName generates a valid service name.
func genServiceName() gopter.Gen {
	return gen.Identifier().Map(func(s string) string {
		// Convert to lowercase for DNS-compatible names
		result := make([]byte, 0, len(s))
		for _, c := range s {
			if c >= 'A' && c <= 'Z' {
				result = append(result, byte(c-'A'+'a'))
			} else if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
				result = append(result, byte(c))
			}
		}
		if len(result) == 0 {
			return "svc"
		}
		// Ensure starts with letter
		if result[0] >= '0' && result[0] <= '9' {
			result = append([]byte{'s'}, result...)
		}
		if len(result) > 20 {
			result = result[:20]
		}
		return string(result)
	}).SuchThat(func(s string) bool {
		return len(s) >= 1 && len(s) <= 20
	})
}

// genServiceNames generates a slice of unique service names.
func genServiceNames(count int) gopter.Gen {
	return gen.SliceOfN(count, genServiceName()).SuchThat(func(names []string) bool {
		seen := make(map[string]bool)
		for _, name := range names {
			if seen[name] || name == "" {
				return false
			}
			seen[name] = true
		}
		return true
	})
}

// TestCircularDependencyDetection tests Property 8: Circular Dependency Detection.
func TestCircularDependencyDetection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewDependencyValidator(logger)

	// Property 8.1: Self-dependency is always rejected
	properties.Property("self-dependency is rejected", prop.ForAll(
		func(serviceName string) bool {
			services := []models.ServiceConfig{
				{Name: serviceName},
			}
			err := validator.ValidateDependencies(services, serviceName, []string{serviceName})
			if err == nil {
				return false
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "depends_on" &&
				containsSubstring(validationErr.Message, "cannot depend on itself")
		},
		genServiceName(),
	))

	// Property 8.2: Direct circular dependency (A -> B -> A) is detected
	properties.Property("direct circular dependency is detected", prop.ForAll(
		func(names []string) bool {
			if len(names) < 2 {
				return true // Skip if not enough unique names
			}
			serviceA := names[0]
			serviceB := names[1]

			// Create services where A depends on B
			services := []models.ServiceConfig{
				{Name: serviceA, DependsOn: []string{serviceB}},
				{Name: serviceB},
			}

			// Try to make B depend on A (creating a cycle)
			err := validator.ValidateDependencies(services, serviceB, []string{serviceA})
			if err == nil {
				return false // Should have detected cycle
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "depends_on" &&
				containsSubstring(validationErr.Message, "circular dependency detected")
		},
		genServiceNames(2),
	))

	// Property 8.3: Transitive circular dependency (A -> B -> C -> A) is detected
	properties.Property("transitive circular dependency is detected", prop.ForAll(
		func(names []string) bool {
			if len(names) < 3 {
				return true // Skip if not enough unique names
			}
			serviceA := names[0]
			serviceB := names[1]
			serviceC := names[2]

			// Create services: A -> B -> C
			services := []models.ServiceConfig{
				{Name: serviceA, DependsOn: []string{serviceB}},
				{Name: serviceB, DependsOn: []string{serviceC}},
				{Name: serviceC},
			}

			// Try to make C depend on A (creating a cycle)
			err := validator.ValidateDependencies(services, serviceC, []string{serviceA})
			if err == nil {
				return false // Should have detected cycle
			}
			validationErr, ok := err.(*models.ValidationError)
			if !ok {
				return false
			}
			return validationErr.Field == "depends_on" &&
				containsSubstring(validationErr.Message, "circular dependency detected")
		},
		genServiceNames(3),
	))

	// Property 8.4: Acyclic dependencies are accepted
	properties.Property("acyclic dependencies are accepted", prop.ForAll(
		func(names []string) bool {
			if len(names) < 3 {
				return true // Skip if not enough unique names
			}
			serviceA := names[0]
			serviceB := names[1]
			serviceC := names[2]

			// Create linear dependency chain: A -> B -> C (no cycle)
			services := []models.ServiceConfig{
				{Name: serviceA, DependsOn: []string{serviceB}},
				{Name: serviceB, DependsOn: []string{serviceC}},
				{Name: serviceC},
			}

			// Adding a new dependency that doesn't create a cycle should succeed
			err := validator.ValidateDependencies(services, serviceA, []string{serviceB, serviceC})
			return err == nil
		},
		genServiceNames(3),
	))

	// Property 8.5: Empty dependencies are always valid
	properties.Property("empty dependencies are valid", prop.ForAll(
		func(serviceName string) bool {
			services := []models.ServiceConfig{
				{Name: serviceName},
			}
			err := validator.ValidateDependencies(services, serviceName, []string{})
			return err == nil
		},
		genServiceName(),
	))

	// Property 8.6: Dependencies on non-existent services don't cause false cycles
	properties.Property("dependencies on non-existent services are allowed", prop.ForAll(
		func(names []string) bool {
			if len(names) < 2 {
				return true
			}
			existingService := names[0]
			nonExistentService := names[1]

			services := []models.ServiceConfig{
				{Name: existingService},
			}

			// Depending on a non-existent service should not cause an error
			// (the service might be external or defined elsewhere)
			err := validator.ValidateDependencies(services, existingService, []string{nonExistentService})
			return err == nil
		},
		genServiceNames(2),
	))

	properties.TestingRun(t)
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
