// Package queue provides a run queue implementation.
// The queue is a double-ended queue (deque) that allows for efficient adding and removing of elements from both ends.
// The queue is used to manage the order of Terragrunt runs.
//
// The algorithm for populating the queue is as follows:
//  1. Given a list of discovered configurations, start with an empty queue.
//  2. Sort configurations alphabetically to ensure deterministic ordering of independent items.
//  3. For each discovered configuration:
//     a. If the configuration has no dependencies, append it to the queue.
//     b. Otherwise, find the position after its last dependency.
//     c. Among items that depend on the same dependency, maintain alphabetical order.
//
// The resulting queue will have:
// - Configurations with no dependencies at the front
// - Configurations with dependents are ordered after their dependencies
// - Alphabetical ordering only between items that share the same dependencies
//
// During operations like applies, entries will be dequeued from the front of the queue and run.
// During operations like destroys, entries will be dequeued from the back of the queue and run.
// This ensures that dependencies are satisfied in both cases:
// - For applies: Dependencies (front) are run before their dependents (back)
// - For destroys: Dependents (back) are run before their dependencies (front)
package queue

import (
	"sort"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
)

const (
	notVisited = iota
	processed
)

type Queue struct {
	entries discovery.DiscoveredConfigs
}

// Entries returns the queue entries. Used for testing.
func (q *Queue) Entries() []*discovery.DiscoveredConfig {
	return q.entries
}

// NewQueue creates a new queue from a list of discovered configurations.
// The queue is populated with the correct Terragrunt run order.
// Dependencies are guaranteed to come before their dependents.
// Items with the same dependencies are sorted alphabetically.
func NewQueue(discovered discovery.DiscoveredConfigs) *Queue {
	// Create a map for O(1) lookups of configs by path
	configMap := make(map[string]*discovery.DiscoveredConfig, len(discovered))
	for _, cfg := range discovered {
		configMap[cfg.Path] = cfg
	}

	// Track processed nodes and their states:
	// 0 = not visited
	// 1 = processed
	visited := make(map[string]int, len(discovered))

	// Result will store configs in dependency order
	result := make(discovery.DiscoveredConfigs, 0, len(discovered))

	// Process nodes level by level, with deterministic ordering within each level
	var processLevel func(configs discovery.DiscoveredConfigs)
	processLevel = func(configs discovery.DiscoveredConfigs) {
		// Group configs by their dependency depth
		type nodeInfo struct {
			config *discovery.DiscoveredConfig
			depth  int
		}

		// Calculate max dependency depth for each config
		levelNodes := make([]nodeInfo, 0, len(configs))

		for _, cfg := range configs {
			if visited[cfg.Path] == processed {
				continue
			}

			maxDepth := 0

			for _, dep := range cfg.Dependencies {
				if visited[dep.Path] != processed {
					// Skip nodes whose dependencies haven't been processed
					maxDepth = -1
					break
				}
			}

			if maxDepth >= 0 {
				levelNodes = append(levelNodes, nodeInfo{cfg, maxDepth})
			}
		}

		// If no nodes can be processed at this level, we're done
		if len(levelNodes) == 0 {
			return
		}

		// Sort nodes by depth (primary) and path (secondary) for deterministic ordering
		sort.SliceStable(levelNodes, func(i, j int) bool {
			if levelNodes[i].depth != levelNodes[j].depth {
				return levelNodes[i].depth < levelNodes[j].depth
			}

			return levelNodes[i].config.Path < levelNodes[j].config.Path
		})

		// Add all nodes at this level to result
		for _, node := range levelNodes {
			visited[node.config.Path] = processed

			result = append(result, node.config)
		}

		// Process next level
		processLevel(configs)
	}

	// Start with all configs
	processLevel(discovered)

	return &Queue{
		entries: result,
	}
}
