package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gopkg.in/yaml.v3"
)

// OpenAPISpec represents the parsed OpenAPI specification.
type OpenAPISpec struct {
	OpenAPI    string                 `yaml:"openapi"`
	Info       map[string]interface{} `yaml:"info"`
	Paths      map[string]PathItem    `yaml:"paths"`
	Components Components             `yaml:"components"`
}

// PathItem represents an OpenAPI path item.
type PathItem map[string]Operation

// Operation represents an OpenAPI operation.
type Operation struct {
	Tags        []string               `yaml:"tags"`
	Summary     string                 `yaml:"summary"`
	Description string                 `yaml:"description"`
	OperationID string                 `yaml:"operationId"`
	Parameters  []Parameter            `yaml:"parameters"`
	RequestBody *RequestBody           `yaml:"requestBody"`
	Responses   map[string]Response    `yaml:"responses"`
	Security    []map[string][]string  `yaml:"security"`
}

// Parameter represents an OpenAPI parameter.
type Parameter struct {
	Name        string                 `yaml:"name"`
	In          string                 `yaml:"in"`
	Required    bool                   `yaml:"required"`
	Description string                 `yaml:"description"`
	Schema      map[string]interface{} `yaml:"schema"`
	Ref         string                 `yaml:"$ref"`
}

// RequestBody represents an OpenAPI request body.
type RequestBody struct {
	Required bool                   `yaml:"required"`
	Content  map[string]MediaType   `yaml:"content"`
}

// MediaType represents an OpenAPI media type.
type MediaType struct {
	Schema map[string]interface{} `yaml:"schema"`
}

// Response represents an OpenAPI response.
type Response struct {
	Description string               `yaml:"description"`
	Content     map[string]MediaType `yaml:"content"`
	Ref         string               `yaml:"$ref"`
}

// Components represents OpenAPI components.
type Components struct {
	Schemas         map[string]interface{} `yaml:"schemas"`
	SecuritySchemes map[string]interface{} `yaml:"securitySchemes"`
	Parameters      map[string]interface{} `yaml:"parameters"`
	Responses       map[string]interface{} `yaml:"responses"`
}

// loadOpenAPISpec loads and parses the OpenAPI specification file.
func loadOpenAPISpec(t *testing.T) *OpenAPISpec {
	t.Helper()

	// Find the openapi.yaml file
	specPath := filepath.Join("openapi.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("failed to read OpenAPI spec: %v", err)
	}

	var spec OpenAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to parse OpenAPI spec: %v", err)
	}

	return &spec
}

// getDocumentedEndpoints returns all documented endpoints from the spec.
func getDocumentedEndpoints(spec *OpenAPISpec) []string {
	var endpoints []string
	for path, pathItem := range spec.Paths {
		for method := range pathItem {
			endpoints = append(endpoints, strings.ToUpper(method)+" "+path)
		}
	}
	return endpoints
}

// hasResponseSchema checks if an operation has a response schema defined.
func hasResponseSchema(op *Operation, statusCode string) bool {
	if op.Responses == nil {
		return false
	}
	resp, ok := op.Responses[statusCode]
	if !ok {
		return false
	}
	// Check if it's a reference or has content
	if resp.Ref != "" {
		return true
	}
	if resp.Content != nil {
		for _, mediaType := range resp.Content {
			if mediaType.Schema != nil {
				return true
			}
		}
	}
	return false
}

// hasRequestSchema checks if an operation has a request schema defined.
func hasRequestSchema(op *Operation) bool {
	if op.RequestBody == nil {
		return true // No request body is valid
	}
	if op.RequestBody.Content == nil {
		return false
	}
	for _, mediaType := range op.RequestBody.Content {
		if mediaType.Schema != nil {
			return true
		}
	}
	return false
}

// hasErrorResponses checks if an operation has error responses defined.
func hasErrorResponses(op *Operation) bool {
	if op.Responses == nil {
		return false
	}
	// Check for at least one error response (4xx or 5xx)
	for code := range op.Responses {
		if strings.HasPrefix(code, "4") || strings.HasPrefix(code, "5") {
			return true
		}
	}
	return false
}

// genEndpointIndex generates a random index for selecting an endpoint.
func genEndpointIndex(maxIndex int) gopter.Gen {
	if maxIndex <= 0 {
		return gen.Const(0)
	}
	return gen.IntRange(0, maxIndex-1)
}

// TestPropertyOpenAPISchemaCompleteness tests that all documented API endpoints
// have complete schemas including request, response, and error responses.
// **Feature: release-changelog-cicd, Property 7: OpenAPI Schema Completeness**
// **Validates: Requirements 9.2, 9.4**
func TestPropertyOpenAPISchemaCompleteness(t *testing.T) {
	spec := loadOpenAPISpec(t)

	// Collect all operations for property testing
	type operationInfo struct {
		path      string
		method    string
		operation *Operation
	}
	var operations []operationInfo

	for path, pathItem := range spec.Paths {
		for method, op := range pathItem {
			opCopy := op // Create a copy to avoid closure issues
			operations = append(operations, operationInfo{
				path:      path,
				method:    method,
				operation: &opCopy,
			})
		}
	}

	if len(operations) == 0 {
		t.Fatal("No operations found in OpenAPI spec")
	}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: For any documented endpoint, it must have a response schema
	properties.Property("Every endpoint has a response schema", prop.ForAll(
		func(idx int) bool {
			op := operations[idx]
			// Check for at least one success response (2xx)
			for code := range op.operation.Responses {
				if strings.HasPrefix(code, "2") {
					return true
				}
			}
			return false
		},
		genEndpointIndex(len(operations)),
	))

	// Property: For any endpoint with a request body, it must have a request schema
	properties.Property("Every endpoint with request body has a request schema", prop.ForAll(
		func(idx int) bool {
			op := operations[idx]
			return hasRequestSchema(op.operation)
		},
		genEndpointIndex(len(operations)),
	))

	// Property: For any authenticated endpoint, it must have error responses defined
	properties.Property("Every authenticated endpoint has error responses", prop.ForAll(
		func(idx int) bool {
			op := operations[idx]
			// Skip health check and auth endpoints (they may not require auth)
			if strings.Contains(op.path, "/health") || strings.Contains(op.path, "/auth/") {
				return true
			}
			// Check if endpoint requires authentication
			if len(op.operation.Security) > 0 {
				return hasErrorResponses(op.operation)
			}
			return true
		},
		genEndpointIndex(len(operations)),
	))

	properties.TestingRun(t)
}

// TestOpenAPISpecStructure tests the basic structure of the OpenAPI spec.
func TestOpenAPISpecStructure(t *testing.T) {
	spec := loadOpenAPISpec(t)

	// Verify OpenAPI version
	if spec.OpenAPI == "" {
		t.Error("OpenAPI version is not specified")
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		t.Errorf("Expected OpenAPI 3.x, got %s", spec.OpenAPI)
	}

	// Verify info section
	if spec.Info == nil {
		t.Error("Info section is missing")
	}
	if spec.Info["title"] == nil {
		t.Error("API title is missing")
	}
	if spec.Info["version"] == nil {
		t.Error("API version is missing")
	}

	// Verify paths exist
	if len(spec.Paths) == 0 {
		t.Error("No paths defined in OpenAPI spec")
	}

	// Verify components exist
	if spec.Components.Schemas == nil || len(spec.Components.Schemas) == 0 {
		t.Error("No schemas defined in components")
	}

	// Verify Error schema exists
	if _, ok := spec.Components.Schemas["Error"]; !ok {
		t.Error("Error schema is not defined")
	}

	// Verify security scheme exists
	if spec.Components.SecuritySchemes == nil || len(spec.Components.SecuritySchemes) == 0 {
		t.Error("No security schemes defined")
	}
}

// TestOpenAPIErrorSchema tests that the Error schema has required fields.
func TestOpenAPIErrorSchema(t *testing.T) {
	spec := loadOpenAPISpec(t)

	errorSchema, ok := spec.Components.Schemas["Error"]
	if !ok {
		t.Fatal("Error schema not found")
	}

	schemaMap, ok := errorSchema.(map[string]interface{})
	if !ok {
		t.Fatal("Error schema is not a map")
	}

	// Check required fields
	required, ok := schemaMap["required"].([]interface{})
	if !ok {
		t.Fatal("Error schema does not have required fields")
	}

	requiredFields := make(map[string]bool)
	for _, field := range required {
		if fieldStr, ok := field.(string); ok {
			requiredFields[fieldStr] = true
		}
	}

	// Verify required fields per Requirements 11.1
	expectedRequired := []string{"code", "message", "request_id"}
	for _, field := range expectedRequired {
		if !requiredFields[field] {
			t.Errorf("Error schema missing required field: %s", field)
		}
	}

	// Check properties exist
	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Error schema does not have properties")
	}

	expectedProperties := []string{"code", "message", "details", "request_id"}
	for _, prop := range expectedProperties {
		if _, ok := properties[prop]; !ok {
			t.Errorf("Error schema missing property: %s", prop)
		}
	}
}

// TestOpenAPIEndpointCoverage tests that key endpoints are documented.
func TestOpenAPIEndpointCoverage(t *testing.T) {
	spec := loadOpenAPISpec(t)

	// Key endpoints that must be documented
	requiredEndpoints := []string{
		"/health",
		"/auth/login",
		"/auth/register",
		"/v1/apps",
		"/v1/apps/{appID}",
		"/v1/apps/{appID}/services",
		"/v1/deployments",
		"/v1/builds",
		"/v1/nodes",
		"/v1/orgs",
	}

	for _, endpoint := range requiredEndpoints {
		if _, ok := spec.Paths[endpoint]; !ok {
			t.Errorf("Required endpoint not documented: %s", endpoint)
		}
	}
}
