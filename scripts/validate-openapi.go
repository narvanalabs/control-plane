//go:build ignore

// Package main provides a script to validate the OpenAPI specification.
// This script is used in CI to ensure the OpenAPI spec is valid and complete.
//
// Usage:
//
//	go run scripts/validate-openapi.go
package main

import (
	"fmt"
	"os"
	"strings"

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
	Required bool                 `yaml:"required"`
	Content  map[string]MediaType `yaml:"content"`
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

func main() {
	specPath := "api/openapi.yaml"
	if len(os.Args) > 1 {
		specPath = os.Args[1]
	}

	fmt.Printf("Validating OpenAPI specification: %s\n", specPath)

	data, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var spec OpenAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	errors := validateSpec(&spec)
	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "\nValidation errors:\n")
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		os.Exit(1)
	}

	fmt.Println("\n✓ OpenAPI specification is valid")
	fmt.Printf("  - Version: %s\n", spec.OpenAPI)
	fmt.Printf("  - Title: %s\n", spec.Info["title"])
	fmt.Printf("  - API Version: %s\n", spec.Info["version"])
	fmt.Printf("  - Paths: %d\n", len(spec.Paths))
	fmt.Printf("  - Schemas: %d\n", len(spec.Components.Schemas))
}

func validateSpec(spec *OpenAPISpec) []string {
	var errors []string

	// Validate OpenAPI version
	if spec.OpenAPI == "" {
		errors = append(errors, "OpenAPI version is not specified")
	} else if !strings.HasPrefix(spec.OpenAPI, "3.") {
		errors = append(errors, fmt.Sprintf("Expected OpenAPI 3.x, got %s", spec.OpenAPI))
	}

	// Validate info section
	if spec.Info == nil {
		errors = append(errors, "Info section is missing")
	} else {
		if spec.Info["title"] == nil {
			errors = append(errors, "API title is missing")
		}
		if spec.Info["version"] == nil {
			errors = append(errors, "API version is missing")
		}
	}

	// Validate paths exist
	if len(spec.Paths) == 0 {
		errors = append(errors, "No paths defined in OpenAPI spec")
	}

	// Validate components
	if spec.Components.Schemas == nil || len(spec.Components.Schemas) == 0 {
		errors = append(errors, "No schemas defined in components")
	}

	// Validate Error schema exists
	if _, ok := spec.Components.Schemas["Error"]; !ok {
		errors = append(errors, "Error schema is not defined")
	}

	// Validate security scheme exists
	if spec.Components.SecuritySchemes == nil || len(spec.Components.SecuritySchemes) == 0 {
		errors = append(errors, "No security schemes defined")
	}

	// Validate each path
	for path, pathItem := range spec.Paths {
		for method, op := range pathItem {
			opErrors := validateOperation(path, method, &op)
			errors = append(errors, opErrors...)
		}
	}

	// Validate required endpoints exist
	requiredEndpoints := []string{
		"/health",
		"/auth/login",
		"/auth/register",
		"/v1/apps",
		"/v1/deployments",
		"/v1/builds",
		"/v1/nodes",
	}

	for _, endpoint := range requiredEndpoints {
		if _, ok := spec.Paths[endpoint]; !ok {
			errors = append(errors, fmt.Sprintf("Required endpoint not documented: %s", endpoint))
		}
	}

	return errors
}

func validateOperation(path, method string, op *Operation) []string {
	var errors []string
	opID := fmt.Sprintf("%s %s", strings.ToUpper(method), path)

	// Check for operationId
	if op.OperationID == "" {
		errors = append(errors, fmt.Sprintf("%s: missing operationId", opID))
	}

	// Check for summary
	if op.Summary == "" {
		errors = append(errors, fmt.Sprintf("%s: missing summary", opID))
	}

	// Check for at least one response
	if op.Responses == nil || len(op.Responses) == 0 {
		errors = append(errors, fmt.Sprintf("%s: no responses defined", opID))
	} else {
		// Check for at least one success response
		hasSuccess := false
		for code := range op.Responses {
			if strings.HasPrefix(code, "2") {
				hasSuccess = true
				break
			}
		}
		if !hasSuccess {
			errors = append(errors, fmt.Sprintf("%s: no success response (2xx) defined", opID))
		}
	}

	// Check authenticated endpoints have error responses
	if len(op.Security) > 0 {
		hasError := false
		for code := range op.Responses {
			if strings.HasPrefix(code, "4") || strings.HasPrefix(code, "5") {
				hasError = true
				break
			}
		}
		if !hasError && !strings.Contains(path, "/health") && !strings.Contains(path, "/auth/") {
			errors = append(errors, fmt.Sprintf("%s: authenticated endpoint missing error responses", opID))
		}
	}

	return errors
}
