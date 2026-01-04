// Package validation provides validation services for the Narvana platform.
package validation

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// DependencyValidator validates service dependencies for cycles and self-references.
type DependencyValidator struct {
	logger *slog.Logger
}

// NewDependencyValidator creates a new DependencyValidator.
func NewDependencyValidator(logger *slog.Logger) *DependencyValidator {
	return &DependencyValidator{logger: logger}
}

// ValidateDependencies checks for cycles and self-dependencies in the service dependency graph.
// It validates that adding newDeps to targetService won't create a cycle.
// Requirements: 9.1, 9.3, 9.4
func (v *DependencyValidator) ValidateDependencies(services []models.ServiceConfig, targetService string, newDeps []string) error {
	// Check for self-dependency
	for _, dep := range newDeps {
		if dep == targetService {
			return &models.ValidationError{
				Field:   "depends_on",
				Message: fmt.Sprintf("service '%s' cannot depend on itself", targetService),
			}
		}
	}

	// Build dependency graph with the proposed changes
	graph := make(map[string][]string)
	for _, svc := range services {
		if svc.Name == targetService {
			graph[svc.Name] = newDeps
		} else {
			graph[svc.Name] = svc.DependsOn
		}
	}

	// If targetService is new (not in services), add it
	if _, exists := graph[targetService]; !exists {
		graph[targetService] = newDeps
	}

	// Detect cycles using DFS
	cycle := v.detectCycle(graph)
	if len(cycle) > 0 {
		return &models.ValidationError{
			Field:   "depends_on",
			Message: fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " -> ")),
		}
	}

	return nil
}

// detectCycle performs a depth-first search to detect cycles in the dependency graph.
// Returns the cycle path if found, or nil if no cycle exists.
func (v *DependencyValidator) detectCycle(graph map[string][]string) []string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string, path []string) []string
	dfs = func(node string, path []string) []string {
		visited[node] = true
		recStack[node] = true
		currentPath := append(path, node)

		for _, neighbor := range graph[node] {
			// Skip dependencies that don't exist in the graph (external or undefined)
			if _, exists := graph[neighbor]; !exists {
				continue
			}

			if !visited[neighbor] {
				if cycle := dfs(neighbor, currentPath); cycle != nil {
					return cycle
				}
			} else if recStack[neighbor] {
				// Found cycle - return path from neighbor to current
				cycleStart := 0
				for i, n := range currentPath {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				return append(currentPath[cycleStart:], neighbor)
			}
		}

		recStack[node] = false
		return nil
	}

	for node := range graph {
		if !visited[node] {
			if cycle := dfs(node, []string{}); cycle != nil {
				return cycle
			}
		}
	}

	return nil
}
