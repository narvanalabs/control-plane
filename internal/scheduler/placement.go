package scheduler

import (
	"github.com/narvanalabs/control-plane/internal/models"
)

// ResourceRequirements defines the CPU and memory requirements for each resource tier.
type ResourceRequirements struct {
	CPU    float64 // CPU cores required
	Memory int64   // Memory in bytes required
}

// TierRequirements maps resource tiers to their requirements.
var TierRequirements = map[models.ResourceTier]ResourceRequirements{
	models.ResourceTierNano:   {CPU: 0.25, Memory: 256 << 20},  // 256MB
	models.ResourceTierSmall:  {CPU: 0.5, Memory: 512 << 20},   // 512MB
	models.ResourceTierMedium: {CPU: 1.0, Memory: 1 << 30},     // 1GB
	models.ResourceTierLarge:  {CPU: 2.0, Memory: 2 << 30},     // 2GB
	models.ResourceTierXLarge: {CPU: 4.0, Memory: 4 << 30},     // 4GB
}

// GetTierRequirements returns the resource requirements for a given tier.
func GetTierRequirements(tier models.ResourceTier) ResourceRequirements {
	if req, ok := TierRequirements[tier]; ok {
		return req
	}
	// Default to nano if unknown tier
	return TierRequirements[models.ResourceTierNano]
}

// filterByCapacity returns nodes that have sufficient resources for the given tier.
func (s *Scheduler) filterByCapacity(nodes []*models.Node, tier models.ResourceTier) []*models.Node {
	requirements := GetTierRequirements(tier)
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


// HasSufficientResources checks if a node has enough resources for a given tier.
func HasSufficientResources(node *models.Node, tier models.ResourceTier) bool {
	if node.Resources == nil {
		return false
	}

	requirements := GetTierRequirements(tier)
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
