package scheduler

import (
	"fmt"
	"strings"

	"github.com/narvanalabs/control-plane/internal/models"
)

// ResourceRequirements defines the CPU and memory requirements.
type ResourceRequirements struct {
	CPU    float64 // CPU cores required
	Memory int64   // Memory in bytes required
}

// GetResourceRequirements returns the resource requirements from a ResourceSpec.
// If spec is nil, returns default requirements (0.5 CPU, 512MB).
func GetResourceRequirements(spec *models.ResourceSpec) ResourceRequirements {
	if spec == nil {
		return ResourceRequirements{CPU: 0.5, Memory: 512 << 20}
	}

	req := ResourceRequirements{CPU: 0.5, Memory: 512 << 20}

	// Parse CPU
	if spec.CPU != "" {
		var cpu float64
		fmt.Sscanf(spec.CPU, "%f", &cpu)
		if cpu > 0 {
			req.CPU = cpu
		}
	}

	// Parse memory
	if spec.Memory != "" {
		req.Memory = parseMemoryToBytes(spec.Memory)
	}

	return req
}

// parseMemoryToBytes parses a memory string to bytes.
func parseMemoryToBytes(mem string) int64 {
	mem = strings.TrimSpace(mem)
	if mem == "" {
		return 512 << 20
	}

	// Handle Gi suffix
	if strings.HasSuffix(mem, "Gi") {
		var val float64
		fmt.Sscanf(mem, "%fGi", &val)
		return int64(val * float64(1<<30))
	}

	// Handle Mi suffix
	if strings.HasSuffix(mem, "Mi") {
		var val int64
		fmt.Sscanf(mem, "%dMi", &val)
		return val << 20
	}

	// Handle G suffix
	if strings.HasSuffix(mem, "G") {
		var val float64
		fmt.Sscanf(mem, "%fG", &val)
		return int64(val * float64(1<<30))
	}

	// Handle M suffix
	if strings.HasSuffix(mem, "M") {
		var val int64
		fmt.Sscanf(mem, "%dM", &val)
		return val << 20
	}

	// Try parsing as plain number (assume bytes)
	var val int64
	fmt.Sscanf(mem, "%d", &val)
	if val > 0 {
		return val
	}

	return 512 << 20 // default 512MB
}

// filterByCapacity returns nodes that have sufficient resources for the given spec.
func (s *Scheduler) filterByCapacity(nodes []*models.Node, spec *models.ResourceSpec) []*models.Node {
	requirements := GetResourceRequirements(spec)
	var capable []*models.Node

	for _, node := range nodes {
		if node.Resources == nil {
			continue
		}

		if node.Resources.CPUAvailable >= requirements.CPU &&
			node.Resources.MemoryAvailable >= requirements.Memory {
			capable = append(capable, node)
		}
	}

	return capable
}

// HasSufficientResources checks if a node has enough resources for a given spec.
func HasSufficientResources(node *models.Node, spec *models.ResourceSpec) bool {
	if node.Resources == nil {
		return false
	}

	requirements := GetResourceRequirements(spec)
	return node.Resources.CPUAvailable >= requirements.CPU &&
		node.Resources.MemoryAvailable >= requirements.Memory
}

// findNodeWithClosure returns the first node that has the given store path cached.
// Returns nil if no node has the closure cached.
func (s *Scheduler) findNodeWithClosure(nodes []*models.Node, storePath string) *models.Node {
	for _, node := range nodes {
		for _, cached := range node.CachedPaths {
			if cached == storePath {
				return node
			}
		}
	}
	return nil
}

// NodeHasClosure checks if a node has a specific store path cached.
func NodeHasClosure(node *models.Node, storePath string) bool {
	for _, cached := range node.CachedPaths {
		if cached == storePath {
			return true
		}
	}
	return false
}

// selectByCapacity returns the node with the highest available capacity.
// Capacity is calculated as a weighted combination of available CPU and memory.
func (s *Scheduler) selectByCapacity(nodes []*models.Node) *models.Node {
	if len(nodes) == 0 {
		return nil
	}

	var best *models.Node
	var bestScore float64

	for _, node := range nodes {
		score := CalculateCapacityScore(node)
		if best == nil || score > bestScore {
			best = node
			bestScore = score
		}
	}

	return best
}

// CalculateCapacityScore calculates a capacity score for a node.
// Higher scores indicate more available capacity.
// The score is a weighted combination of CPU and memory availability.
func CalculateCapacityScore(node *models.Node) float64 {
	if node.Resources == nil {
		return 0
	}

	// Normalize CPU (assume max 64 cores) and memory (assume max 256GB)
	cpuScore := node.Resources.CPUAvailable / 64.0
	memScore := float64(node.Resources.MemoryAvailable) / float64(256<<30)

	// Weight CPU and memory equally
	return (cpuScore + memScore) / 2.0
}

// SelectBestNode selects the best node from a list based on deployment requirements.
// This is a convenience function that combines all placement strategies.
func SelectBestNode(nodes []*models.Node, deployment *models.Deployment) *models.Node {
	if len(nodes) == 0 {
		return nil
	}

	// For pure Nix deployments, prefer nodes with cached closure
	if deployment.BuildType == models.BuildTypePureNix && deployment.Artifact != "" {
		for _, node := range nodes {
			if NodeHasClosure(node, deployment.Artifact) {
				return node
			}
		}
	}

	// Fall back to capacity-based selection
	var best *models.Node
	var bestScore float64

	for _, node := range nodes {
		score := CalculateCapacityScore(node)
		if best == nil || score > bestScore {
			best = node
			bestScore = score
		}
	}

	return best
}
