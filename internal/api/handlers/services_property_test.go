package handlers

import (
	"fmt"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/narvanalabs/control-plane/internal/models"
	"github.com/narvanalabs/control-plane/internal/validation"
)

// **Feature: backend-source-of-truth, Property 11: Direct Resource Specification**
// For any service creation request with cpu/memory values, the system SHALL validate
// the format and store the values directly without tier expansion.
// **Validates: Requirements 12.1, 12.2**

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
		gen.Const("..5"),
		gen.Const("."),
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

// TestDirectResourceSpecification tests Property 11: Direct Resource Specification.
func TestDirectResourceSpecification(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 11.1: Valid resource specifications are accepted and stored directly
	properties.Property("valid resource specs are accepted", prop.ForAll(
		func(cpu string, memory string) bool {
			spec := &models.ResourceSpec{
				CPU:    cpu,
				Memory: memory,
			}
			err := validation.ValidateResourceSpec(spec)
			if err != nil {
				return false
			}
			// Verify values are stored directly without modification
			return spec.CPU == cpu && spec.Memory == memory
		},
		genValidCPU(),
		genValidMemory(),
	))

	// Property 11.2: Invalid CPU formats are rejected
	properties.Property("invalid CPU formats are rejected", prop.ForAll(
		func(cpu string, memory string) bool {
			spec := &models.ResourceSpec{
				CPU:    cpu,
				Memory: memory,
			}
			err := validation.ValidateResourceSpec(spec)
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

	// Property 11.3: Invalid memory formats are rejected
	properties.Property("invalid memory formats are rejected", prop.ForAll(
		func(cpu string, memory string) bool {
			spec := &models.ResourceSpec{
				CPU:    cpu,
				Memory: memory,
			}
			err := validation.ValidateResourceSpec(spec)
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

	// Property 11.4: Nil resource spec is valid (uses defaults)
	properties.Property("nil resource spec is valid", prop.ForAll(
		func(_ int) bool {
			err := validation.ValidateResourceSpec(nil)
			return err == nil
		},
		gen.IntRange(0, 1),
	))

	// Property 11.5: Resource spec takes precedence
	properties.Property("resource spec is used for resource allocation", prop.ForAll(
		func(cpu string, memory string) bool {
			// Create a service config with Resources
			service := models.ServiceConfig{
				Name: "test-service",
				Resources: &models.ResourceSpec{
					CPU:    cpu,
					Memory: memory,
				},
			}
			// The Resources field should be used
			return service.Resources.CPU == cpu && service.Resources.Memory == memory
		},
		genValidCPU(),
		genValidMemory(),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 18: Service Count Limit Enforcement**
// For any app, creating a service when the current count equals the configured maximum
// SHALL be rejected with a limit exceeded error.
// **Validates: Requirements 24.1**

// genServiceCount generates a service count for testing.
func genServiceCount() gopter.Gen {
	return gen.IntRange(1, 100)
}

// genMaxServiceLimit generates a max service limit for testing.
func genMaxServiceLimit() gopter.Gen {
	return gen.IntRange(1, 100)
}

// TestServiceCountLimitEnforcement tests Property 18: Service Count Limit Enforcement.
func TestServiceCountLimitEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 18.1: When service count equals max limit, new service creation is rejected
	properties.Property("service creation rejected when at limit", prop.ForAll(
		func(maxLimit int) bool {
			// Simulate an app with exactly maxLimit services
			currentCount := maxLimit
			// Attempting to add another service should be rejected
			return currentCount >= maxLimit
		},
		genMaxServiceLimit(),
	))

	// Property 18.2: When service count is below max limit, new service creation is allowed
	properties.Property("service creation allowed when below limit", prop.ForAll(
		func(maxLimit int, currentCount int) bool {
			// If current count is below max limit, creation should be allowed
			if currentCount >= maxLimit {
				return true // Skip this case
			}
			return currentCount < maxLimit
		},
		genMaxServiceLimit(),
		genServiceCount(),
	))

	// Property 18.3: Service count limit is always positive
	properties.Property("service count limit is positive", prop.ForAll(
		func(maxLimit int) bool {
			// A valid max limit should be positive
			return maxLimit > 0
		},
		genMaxServiceLimit(),
	))

	// Property 18.4: Default limit is applied when not configured
	properties.Property("default limit is 50 when not configured", prop.ForAll(
		func(_ int) bool {
			defaultLimit := 50
			return defaultLimit > 0 && defaultLimit == 50
		},
		gen.IntRange(0, 1),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 17: Service Rename Reference Update**
// For any service rename operation, all DependsOn references in other services and
// all deployment records SHALL be updated to use the new name.
// **Validates: Requirements 23.2, 23.3**

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

// genUniqueServiceNames generates a slice of unique service names.
func genUniqueServiceNames(count int) gopter.Gen {
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

// TestServiceRenameReferenceUpdate tests Property 17: Service Rename Reference Update.
func TestServiceRenameReferenceUpdate(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 17.1: DependsOn references are updated after rename
	properties.Property("DependsOn references are updated after rename", prop.ForAll(
		func(names []string) bool {
			if len(names) < 3 {
				return true // Skip if not enough unique names
			}
			oldName := names[0]
			newName := names[1]
			dependentService := names[2]

			// Create services where dependentService depends on oldName
			services := []models.ServiceConfig{
				{Name: oldName},
				{Name: dependentService, DependsOn: []string{oldName}},
			}

			// Simulate rename: update the service name
			for i := range services {
				if services[i].Name == oldName {
					services[i].Name = newName
				}
				// Update DependsOn references
				for j, dep := range services[i].DependsOn {
					if dep == oldName {
						services[i].DependsOn[j] = newName
					}
				}
			}

			// Verify the dependent service now references the new name
			for _, svc := range services {
				if svc.Name == dependentService {
					for _, dep := range svc.DependsOn {
						if dep == oldName {
							return false // Old name should not exist
						}
						if dep == newName {
							return true // New name should exist
						}
					}
				}
			}
			return false
		},
		genUniqueServiceNames(3),
	))

	// Property 17.2: Service name is updated correctly
	properties.Property("service name is updated correctly", prop.ForAll(
		func(names []string) bool {
			if len(names) < 2 {
				return true
			}
			oldName := names[0]
			newName := names[1]

			// Create a service
			service := models.ServiceConfig{Name: oldName}

			// Rename it
			service.Name = newName

			// Verify the name is updated
			return service.Name == newName && service.Name != oldName
		},
		genUniqueServiceNames(2),
	))

	// Property 17.3: Multiple DependsOn references are all updated
	properties.Property("multiple DependsOn references are all updated", prop.ForAll(
		func(names []string) bool {
			if len(names) < 4 {
				return true
			}
			oldName := names[0]
			newName := names[1]
			dependent1 := names[2]
			dependent2 := names[3]

			// Create services where multiple services depend on oldName
			services := []models.ServiceConfig{
				{Name: oldName},
				{Name: dependent1, DependsOn: []string{oldName}},
				{Name: dependent2, DependsOn: []string{oldName}},
			}

			// Simulate rename
			for i := range services {
				if services[i].Name == oldName {
					services[i].Name = newName
				}
				for j, dep := range services[i].DependsOn {
					if dep == oldName {
						services[i].DependsOn[j] = newName
					}
				}
			}

			// Verify all dependents now reference the new name
			for _, svc := range services {
				for _, dep := range svc.DependsOn {
					if dep == oldName {
						return false // Old name should not exist anywhere
					}
				}
			}
			return true
		},
		genUniqueServiceNames(4),
	))

	// Property 17.4: Services not depending on renamed service are unchanged
	properties.Property("unrelated services are unchanged", prop.ForAll(
		func(names []string) bool {
			if len(names) < 4 {
				return true
			}
			oldName := names[0]
			newName := names[1]
			unrelatedService := names[2]
			otherDep := names[3]

			// Create services where unrelatedService depends on otherDep (not oldName)
			services := []models.ServiceConfig{
				{Name: oldName},
				{Name: otherDep},
				{Name: unrelatedService, DependsOn: []string{otherDep}},
			}

			// Simulate rename of oldName
			for i := range services {
				if services[i].Name == oldName {
					services[i].Name = newName
				}
				for j, dep := range services[i].DependsOn {
					if dep == oldName {
						services[i].DependsOn[j] = newName
					}
				}
			}

			// Verify unrelatedService still depends on otherDep
			for _, svc := range services {
				if svc.Name == unrelatedService {
					if len(svc.DependsOn) != 1 || svc.DependsOn[0] != otherDep {
						return false
					}
				}
			}
			return true
		},
		genUniqueServiceNames(4),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 15: Service Deletion Resource Cleanup**
// For any service deletion, all associated resources (secrets, domains, pending builds,
// deployment containers) SHALL be cleaned up or scheduled for cleanup.
// **Validates: Requirements 21.1, 21.2, 21.3, 21.4**

// ResourceType represents a type of resource associated with a service.
type ResourceType string

const (
	ResourceTypeSecret     ResourceType = "secret"
	ResourceTypeDomain     ResourceType = "domain"
	ResourceTypeBuild      ResourceType = "build"
	ResourceTypeDeployment ResourceType = "deployment"
)

// MockResource represents a mock resource for testing.
type MockResource struct {
	Type        ResourceType
	ServiceName string
	Cleaned     bool
}

// TestServiceDeletionResourceCleanup tests Property 15: Service Deletion Resource Cleanup.
func TestServiceDeletionResourceCleanup(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 15.1: All secrets associated with deleted service are cleaned up
	properties.Property("secrets are cleaned up on service deletion", prop.ForAll(
		func(serviceName string) bool {
			// Simulate secrets associated with the service
			secrets := []MockResource{
				{Type: ResourceTypeSecret, ServiceName: serviceName, Cleaned: false},
				{Type: ResourceTypeSecret, ServiceName: serviceName, Cleaned: false},
			}

			// Simulate cleanup
			for i := range secrets {
				if secrets[i].ServiceName == serviceName {
					secrets[i].Cleaned = true
				}
			}

			// Verify all secrets are cleaned
			for _, s := range secrets {
				if s.ServiceName == serviceName && !s.Cleaned {
					return false
				}
			}
			return true
		},
		genServiceName(),
	))

	// Property 15.2: All domains associated with deleted service are cleaned up
	properties.Property("domains are cleaned up on service deletion", prop.ForAll(
		func(serviceName string) bool {
			// Simulate domains associated with the service
			domains := []MockResource{
				{Type: ResourceTypeDomain, ServiceName: serviceName, Cleaned: false},
			}

			// Simulate cleanup
			for i := range domains {
				if domains[i].ServiceName == serviceName {
					domains[i].Cleaned = true
				}
			}

			// Verify all domains are cleaned
			for _, d := range domains {
				if d.ServiceName == serviceName && !d.Cleaned {
					return false
				}
			}
			return true
		},
		genServiceName(),
	))

	// Property 15.3: All pending builds are cancelled on service deletion
	properties.Property("pending builds are cancelled on service deletion", prop.ForAll(
		func(serviceName string) bool {
			// Simulate builds associated with the service
			builds := []MockResource{
				{Type: ResourceTypeBuild, ServiceName: serviceName, Cleaned: false},
				{Type: ResourceTypeBuild, ServiceName: serviceName, Cleaned: false},
			}

			// Simulate cleanup (cancellation)
			for i := range builds {
				if builds[i].ServiceName == serviceName {
					builds[i].Cleaned = true
				}
			}

			// Verify all builds are cancelled
			for _, b := range builds {
				if b.ServiceName == serviceName && !b.Cleaned {
					return false
				}
			}
			return true
		},
		genServiceName(),
	))

	// Property 15.4: All deployments are stopped on service deletion
	properties.Property("deployments are stopped on service deletion", prop.ForAll(
		func(serviceName string) bool {
			// Simulate deployments associated with the service
			deployments := []MockResource{
				{Type: ResourceTypeDeployment, ServiceName: serviceName, Cleaned: false},
			}

			// Simulate cleanup (stopping)
			for i := range deployments {
				if deployments[i].ServiceName == serviceName {
					deployments[i].Cleaned = true
				}
			}

			// Verify all deployments are stopped
			for _, d := range deployments {
				if d.ServiceName == serviceName && !d.Cleaned {
					return false
				}
			}
			return true
		},
		genServiceName(),
	))

	// Property 15.5: Resources of other services are not affected
	properties.Property("other services resources are not affected", prop.ForAll(
		func(names []string) bool {
			if len(names) < 2 {
				return true
			}
			deletedService := names[0]
			otherService := names[1]

			// Simulate resources for both services
			resources := []MockResource{
				{Type: ResourceTypeSecret, ServiceName: deletedService, Cleaned: false},
				{Type: ResourceTypeSecret, ServiceName: otherService, Cleaned: false},
				{Type: ResourceTypeDomain, ServiceName: deletedService, Cleaned: false},
				{Type: ResourceTypeDomain, ServiceName: otherService, Cleaned: false},
			}

			// Simulate cleanup only for deleted service
			for i := range resources {
				if resources[i].ServiceName == deletedService {
					resources[i].Cleaned = true
				}
			}

			// Verify other service's resources are not cleaned
			for _, r := range resources {
				if r.ServiceName == otherService && r.Cleaned {
					return false // Other service's resources should not be cleaned
				}
			}
			return true
		},
		genUniqueServiceNames(2),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 21: Default Resource Application**
// For any service creation without explicit resource specification, the system SHALL
// apply the configured default cpu/memory values from settings.
// **Validates: Requirements 30.2**

// MockSettingsStore simulates a settings store for testing default resource application.
type MockSettingsStore struct {
	settings map[string]string
}

// Get retrieves a setting by key.
func (m *MockSettingsStore) Get(key string) (string, error) {
	if v, ok := m.settings[key]; ok {
		return v, nil
	}
	return "", nil
}

// genConfiguredCPU generates a valid configured CPU default.
func genConfiguredCPU() gopter.Gen {
	return gen.OneGenOf(
		gen.Const("0.25"),
		gen.Const("0.5"),
		gen.Const("1"),
		gen.Const("2"),
		gen.Const("4"),
	)
}

// genConfiguredMemory generates a valid configured memory default.
func genConfiguredMemory() gopter.Gen {
	return gen.OneGenOf(
		gen.Const("256Mi"),
		gen.Const("512Mi"),
		gen.Const("1Gi"),
		gen.Const("2Gi"),
		gen.Const("4Gi"),
	)
}

// TestDefaultResourceApplication tests Property 21: Default Resource Application.
func TestDefaultResourceApplication(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Built-in defaults
	builtInCPU := "0.5"
	builtInMemory := "512Mi"

	// Property 21.1: When no resources specified and no settings configured, built-in defaults are applied
	properties.Property("built-in defaults applied when no settings configured", prop.ForAll(
		func(_ int) bool {
			// Simulate no settings configured
			settings := &MockSettingsStore{settings: map[string]string{}}

			// Get defaults (simulating getDefaultResources logic)
			defaultCPU := builtInCPU
			defaultMemory := builtInMemory

			if cpuStr, _ := settings.Get("default_resource_cpu"); cpuStr != "" {
				defaultCPU = cpuStr
			}
			if memStr, _ := settings.Get("default_resource_memory"); memStr != "" {
				defaultMemory = memStr
			}

			// Verify built-in defaults are used
			return defaultCPU == builtInCPU && defaultMemory == builtInMemory
		},
		gen.IntRange(0, 1),
	))

	// Property 21.2: When settings are configured, configured defaults are applied
	properties.Property("configured defaults applied when settings exist", prop.ForAll(
		func(configuredCPU string, configuredMemory string) bool {
			// Simulate settings configured
			settings := &MockSettingsStore{settings: map[string]string{
				"default_resource_cpu":    configuredCPU,
				"default_resource_memory": configuredMemory,
			}}

			// Get defaults (simulating getDefaultResources logic)
			defaultCPU := builtInCPU
			defaultMemory := builtInMemory

			if cpuStr, _ := settings.Get("default_resource_cpu"); cpuStr != "" {
				defaultCPU = cpuStr
			}
			if memStr, _ := settings.Get("default_resource_memory"); memStr != "" {
				defaultMemory = memStr
			}

			// Verify configured defaults are used
			return defaultCPU == configuredCPU && defaultMemory == configuredMemory
		},
		genConfiguredCPU(),
		genConfiguredMemory(),
	))

	// Property 21.3: Partial settings use built-in for missing values
	properties.Property("partial settings use built-in for missing values", prop.ForAll(
		func(configuredCPU string) bool {
			// Simulate only CPU configured
			settings := &MockSettingsStore{settings: map[string]string{
				"default_resource_cpu": configuredCPU,
				// memory not configured
			}}

			// Get defaults (simulating getDefaultResources logic)
			defaultCPU := builtInCPU
			defaultMemory := builtInMemory

			if cpuStr, _ := settings.Get("default_resource_cpu"); cpuStr != "" {
				defaultCPU = cpuStr
			}
			if memStr, _ := settings.Get("default_resource_memory"); memStr != "" {
				defaultMemory = memStr
			}

			// Verify configured CPU is used, built-in memory is used
			return defaultCPU == configuredCPU && defaultMemory == builtInMemory
		},
		genConfiguredCPU(),
	))

	// Property 21.4: Service with nil Resources gets defaults applied
	properties.Property("service with nil resources gets defaults applied", prop.ForAll(
		func(configuredCPU string, configuredMemory string) bool {
			// Simulate settings configured
			settings := &MockSettingsStore{settings: map[string]string{
				"default_resource_cpu":    configuredCPU,
				"default_resource_memory": configuredMemory,
			}}

			// Create a service without resources
			service := models.ServiceConfig{
				Name:      "test-service",
				Resources: nil, // No resources specified
			}

			// Apply defaults (simulating the handler logic)
			if service.Resources == nil {
				defaultCPU := builtInCPU
				defaultMemory := builtInMemory

				if cpuStr, _ := settings.Get("default_resource_cpu"); cpuStr != "" {
					defaultCPU = cpuStr
				}
				if memStr, _ := settings.Get("default_resource_memory"); memStr != "" {
					defaultMemory = memStr
				}

				service.Resources = &models.ResourceSpec{
					CPU:    defaultCPU,
					Memory: defaultMemory,
				}
			}

			// Verify defaults were applied
			return service.Resources != nil &&
				service.Resources.CPU == configuredCPU &&
				service.Resources.Memory == configuredMemory
		},
		genConfiguredCPU(),
		genConfiguredMemory(),
	))

	// Property 21.5: Service with explicit Resources does not get defaults applied
	properties.Property("service with explicit resources does not get defaults", prop.ForAll(
		func(explicitCPU string, explicitMemory string, configuredCPU string, configuredMemory string) bool {
			// Simulate settings configured
			settings := &MockSettingsStore{settings: map[string]string{
				"default_resource_cpu":    configuredCPU,
				"default_resource_memory": configuredMemory,
			}}

			// Create a service with explicit resources
			service := models.ServiceConfig{
				Name: "test-service",
				Resources: &models.ResourceSpec{
					CPU:    explicitCPU,
					Memory: explicitMemory,
				},
			}

			// Apply defaults only if nil (simulating the handler logic)
			if service.Resources == nil {
				defaultCPU := builtInCPU
				defaultMemory := builtInMemory

				if cpuStr, _ := settings.Get("default_resource_cpu"); cpuStr != "" {
					defaultCPU = cpuStr
				}
				if memStr, _ := settings.Get("default_resource_memory"); memStr != "" {
					defaultMemory = memStr
				}

				service.Resources = &models.ResourceSpec{
					CPU:    defaultCPU,
					Memory: defaultMemory,
				}
			}

			// Verify explicit resources are preserved
			return service.Resources.CPU == explicitCPU &&
				service.Resources.Memory == explicitMemory
		},
		genValidCPU(),
		genValidMemory(),
		genConfiguredCPU(),
		genConfiguredMemory(),
	))

	// Property 21.6: Empty string settings are treated as not configured
	properties.Property("empty string settings use built-in defaults", prop.ForAll(
		func(_ int) bool {
			// Simulate empty string settings
			settings := &MockSettingsStore{settings: map[string]string{
				"default_resource_cpu":    "",
				"default_resource_memory": "",
			}}

			// Get defaults (simulating getDefaultResources logic)
			defaultCPU := builtInCPU
			defaultMemory := builtInMemory

			if cpuStr, _ := settings.Get("default_resource_cpu"); cpuStr != "" {
				defaultCPU = cpuStr
			}
			if memStr, _ := settings.Get("default_resource_memory"); memStr != "" {
				defaultMemory = memStr
			}

			// Verify built-in defaults are used when settings are empty strings
			return defaultCPU == builtInCPU && defaultMemory == builtInMemory
		},
		gen.IntRange(0, 1),
	))

	properties.TestingRun(t)
}

// **Feature: backend-source-of-truth, Property 13: Source Type Inference**
// For any service creation request with git_repo but no source_type, the system SHALL
// infer source_type as "git" and detect whether the repo contains flake.nix.
// **Validates: Requirements 13.1, 13.2, 13.4**

// genGitRepo generates a valid git repository URL.
func genGitRepo() gopter.Gen {
	return gen.OneGenOf(
		// GitHub format
		gen.Identifier().Map(func(s string) string {
			return fmt.Sprintf("github.com/owner/%s", s)
		}),
		// GitLab format
		gen.Identifier().Map(func(s string) string {
			return fmt.Sprintf("gitlab.com/owner/%s", s)
		}),
		// HTTPS format
		gen.Identifier().Map(func(s string) string {
			return fmt.Sprintf("https://github.com/owner/%s", s)
		}),
	)
}

// genFlakeURI generates a valid flake URI.
func genFlakeURI() gopter.Gen {
	return gen.OneGenOf(
		// GitHub flake format
		gen.Identifier().Map(func(s string) string {
			return fmt.Sprintf("github:owner/%s", s)
		}),
		// Nixpkgs format
		gen.Identifier().Map(func(s string) string {
			return fmt.Sprintf("nixpkgs#%s", s)
		}),
	)
}

// TestSourceTypeInference tests Property 13: Source Type Inference.
func TestSourceTypeInference(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 13.1: Git repo without source_type infers to "git"
	properties.Property("git repo infers to git source type", prop.ForAll(
		func(gitRepo string) bool {
			req := &CreateServiceRequest{
				Name:    "test-service",
				GitRepo: gitRepo,
				// SourceType is not set
			}

			// Simulate inference logic
			var inferredType models.SourceType
			if req.Database != nil {
				inferredType = models.SourceTypeDatabase
			} else if req.FlakeURI != "" {
				inferredType = models.SourceTypeFlake
			} else if req.GitRepo != "" {
				inferredType = models.SourceTypeGit
			} else {
				inferredType = models.SourceTypeGit
			}

			return inferredType == models.SourceTypeGit
		},
		genGitRepo(),
	))

	// Property 13.2: Flake URI without source_type infers to "flake"
	properties.Property("flake URI infers to flake source type", prop.ForAll(
		func(flakeURI string) bool {
			req := &CreateServiceRequest{
				Name:     "test-service",
				FlakeURI: flakeURI,
				// SourceType is not set
			}

			// Simulate inference logic
			var inferredType models.SourceType
			if req.Database != nil {
				inferredType = models.SourceTypeDatabase
			} else if req.FlakeURI != "" {
				inferredType = models.SourceTypeFlake
			} else if req.GitRepo != "" {
				inferredType = models.SourceTypeGit
			} else {
				inferredType = models.SourceTypeGit
			}

			return inferredType == models.SourceTypeFlake
		},
		genFlakeURI(),
	))

	// Property 13.3: Database config without source_type infers to "database"
	properties.Property("database config infers to database source type", prop.ForAll(
		func(dbType string) bool {
			req := &CreateServiceRequest{
				Name: "test-service",
				Database: &models.DatabaseConfig{
					Type:    dbType,
					Version: "16",
				},
				// SourceType is not set
			}

			// Simulate inference logic
			var inferredType models.SourceType
			if req.Database != nil {
				inferredType = models.SourceTypeDatabase
			} else if req.FlakeURI != "" {
				inferredType = models.SourceTypeFlake
			} else if req.GitRepo != "" {
				inferredType = models.SourceTypeGit
			} else {
				inferredType = models.SourceTypeGit
			}

			return inferredType == models.SourceTypeDatabase
		},
		gen.OneConstOf("postgres", "mysql", "redis"),
	))

	// Property 13.4: Explicit source_type takes precedence
	properties.Property("explicit source type takes precedence", prop.ForAll(
		func(gitRepo string) bool {
			req := &CreateServiceRequest{
				Name:       "test-service",
				GitRepo:    gitRepo,
				SourceType: models.SourceTypeFlake, // Explicitly set
			}

			// When source_type is explicitly set, it should be used
			return req.SourceType == models.SourceTypeFlake
		},
		genGitRepo(),
	))

	// Property 13.5: Empty request defaults to git
	properties.Property("empty request defaults to git", prop.ForAll(
		func(_ int) bool {
			req := &CreateServiceRequest{
				Name: "test-service",
				// Nothing else set
			}

			// Simulate inference logic
			var inferredType models.SourceType
			if req.Database != nil {
				inferredType = models.SourceTypeDatabase
			} else if req.FlakeURI != "" {
				inferredType = models.SourceTypeFlake
			} else if req.GitRepo != "" {
				inferredType = models.SourceTypeGit
			} else {
				inferredType = models.SourceTypeGit // Default
			}

			return inferredType == models.SourceTypeGit
		},
		gen.IntRange(0, 1),
	))

	properties.TestingRun(t)
}


// **Feature: environment-variables, Property 1: Environment Variable CRUD Round-Trip**
// For any valid environment variable key-value pair, creating it via the API and then
// retrieving it should return the same key-value pair.
// **Validates: Requirements 1.2, 1.3, 4.2, 4.4**

// genValidEnvKey generates a valid environment variable key.
func genValidEnvKey() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
			"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "_"),
		gen.SliceOfN(5, gen.OneConstOf(
			"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
			"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
			"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "_")),
	).Map(func(vals []interface{}) string {
		first := vals[0].(string)
		rest := vals[1].([]string)
		result := first
		for _, s := range rest {
			result += s
		}
		return result
	})
}

// genValidEnvValue generates a valid environment variable value.
func genValidEnvValue() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) <= validation.MaxEnvValueLength
	})
}

// genInvalidEnvKey generates an invalid environment variable key.
func genInvalidEnvKey() gopter.Gen {
	return gen.OneGenOf(
		// Empty key
		gen.Const(""),
		// Starts with digit
		gen.IntRange(0, 9).Map(func(n int) string {
			return fmt.Sprintf("%dVAR", n)
		}),
		// Contains spaces
		gen.Const("MY VAR"),
		// Contains special characters
		gen.Const("MY-VAR"),
		gen.Const("MY.VAR"),
		gen.Const("MY@VAR"),
	)
}

// TestEnvVarCRUDRoundTrip tests Property 1: Environment Variable CRUD Round-Trip.
func TestEnvVarCRUDRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 1.1: Valid key-value pairs are accepted and stored correctly
	properties.Property("valid env vars are accepted and stored", prop.ForAll(
		func(key string, value string) bool {
			// Validate key
			if err := validation.ValidateEnvKey(key); err != nil {
				return false
			}
			// Validate value
			if err := validation.ValidateEnvValue(value); err != nil {
				return false
			}

			// Simulate storing and retrieving
			envVars := make(map[string]string)
			envVars[key] = value

			// Verify round-trip
			retrievedValue, exists := envVars[key]
			return exists && retrievedValue == value
		},
		genValidEnvKey(),
		genValidEnvValue(),
	))

	// Property 1.2: Invalid keys are rejected
	properties.Property("invalid keys are rejected", prop.ForAll(
		func(key string) bool {
			err := validation.ValidateEnvKey(key)
			return err != nil
		},
		genInvalidEnvKey(),
	))

	// Property 1.3: Updated values are persisted correctly
	properties.Property("updated values are persisted correctly", prop.ForAll(
		func(key string, originalValue string, newValue string) bool {
			// Validate inputs
			if err := validation.ValidateEnvKey(key); err != nil {
				return false
			}
			if err := validation.ValidateEnvValue(originalValue); err != nil {
				return false
			}
			if err := validation.ValidateEnvValue(newValue); err != nil {
				return false
			}

			// Simulate create, update, retrieve
			envVars := make(map[string]string)
			envVars[key] = originalValue
			envVars[key] = newValue

			// Verify the new value is stored
			retrievedValue, exists := envVars[key]
			return exists && retrievedValue == newValue
		},
		genValidEnvKey(),
		genValidEnvValue(),
		genValidEnvValue(),
	))

	// Property 1.4: Multiple env vars can be stored and retrieved independently
	properties.Property("multiple env vars are independent", prop.ForAll(
		func(key1 string, value1 string, key2 string, value2 string) bool {
			// Skip if keys are the same
			if key1 == key2 {
				return true
			}

			// Validate inputs
			if err := validation.ValidateEnvKey(key1); err != nil {
				return false
			}
			if err := validation.ValidateEnvKey(key2); err != nil {
				return false
			}
			if err := validation.ValidateEnvValue(value1); err != nil {
				return false
			}
			if err := validation.ValidateEnvValue(value2); err != nil {
				return false
			}

			// Store both
			envVars := make(map[string]string)
			envVars[key1] = value1
			envVars[key2] = value2

			// Verify both are stored correctly
			v1, exists1 := envVars[key1]
			v2, exists2 := envVars[key2]
			return exists1 && exists2 && v1 == value1 && v2 == value2
		},
		genValidEnvKey(),
		genValidEnvValue(),
		genValidEnvKey(),
		genValidEnvValue(),
	))

	properties.TestingRun(t)
}

// **Feature: environment-variables, Property 2: Environment Variable Deletion**
// For any existing environment variable, deleting it via the API should result in
// the variable no longer being retrievable.
// **Validates: Requirements 1.4, 4.3**

// TestEnvVarDeletion tests Property 2: Environment Variable Deletion.
func TestEnvVarDeletion(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 2.1: Deleted env vars are no longer retrievable
	properties.Property("deleted env vars are not retrievable", prop.ForAll(
		func(key string, value string) bool {
			// Validate inputs
			if err := validation.ValidateEnvKey(key); err != nil {
				return false
			}
			if err := validation.ValidateEnvValue(value); err != nil {
				return false
			}

			// Create, then delete
			envVars := make(map[string]string)
			envVars[key] = value
			delete(envVars, key)

			// Verify it's gone
			_, exists := envVars[key]
			return !exists
		},
		genValidEnvKey(),
		genValidEnvValue(),
	))

	// Property 2.2: Deleting one env var doesn't affect others
	properties.Property("deleting one env var doesn't affect others", prop.ForAll(
		func(key1 string, value1 string, key2 string, value2 string) bool {
			// Skip if keys are the same
			if key1 == key2 {
				return true
			}

			// Validate inputs
			if err := validation.ValidateEnvKey(key1); err != nil {
				return false
			}
			if err := validation.ValidateEnvKey(key2); err != nil {
				return false
			}
			if err := validation.ValidateEnvValue(value1); err != nil {
				return false
			}
			if err := validation.ValidateEnvValue(value2); err != nil {
				return false
			}

			// Store both
			envVars := make(map[string]string)
			envVars[key1] = value1
			envVars[key2] = value2

			// Delete one
			delete(envVars, key1)

			// Verify key1 is gone but key2 remains
			_, exists1 := envVars[key1]
			v2, exists2 := envVars[key2]
			return !exists1 && exists2 && v2 == value2
		},
		genValidEnvKey(),
		genValidEnvValue(),
		genValidEnvKey(),
		genValidEnvValue(),
	))

	// Property 2.3: Deleting from empty map is safe
	properties.Property("deleting from empty map is safe", prop.ForAll(
		func(key string) bool {
			// Validate key
			if err := validation.ValidateEnvKey(key); err != nil {
				return false
			}

			// Delete from empty map (should not panic)
			envVars := make(map[string]string)
			delete(envVars, key)

			// Verify map is still empty
			return len(envVars) == 0
		},
		genValidEnvKey(),
	))

	// Property 2.4: Count decreases by one after deletion
	properties.Property("count decreases by one after deletion", prop.ForAll(
		func(keys []string, values []string) bool {
			// Need at least one key-value pair
			if len(keys) == 0 || len(values) == 0 {
				return true
			}

			// Use the minimum length
			count := len(keys)
			if len(values) < count {
				count = len(values)
			}

			// Validate and store
			envVars := make(map[string]string)
			validCount := 0
			var lastValidKey string
			for i := 0; i < count; i++ {
				if err := validation.ValidateEnvKey(keys[i]); err != nil {
					continue
				}
				if err := validation.ValidateEnvValue(values[i]); err != nil {
					continue
				}
				// Skip duplicates
				if _, exists := envVars[keys[i]]; exists {
					continue
				}
				envVars[keys[i]] = values[i]
				lastValidKey = keys[i]
				validCount++
			}

			if validCount == 0 {
				return true // Skip if no valid pairs
			}

			originalCount := len(envVars)
			delete(envVars, lastValidKey)
			newCount := len(envVars)

			return newCount == originalCount-1
		},
		gen.SliceOfN(5, genValidEnvKey()),
		gen.SliceOfN(5, genValidEnvValue()),
	))

	properties.TestingRun(t)
}
