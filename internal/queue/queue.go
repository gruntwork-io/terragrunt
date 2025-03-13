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
	"github.com/gruntwork-io/terragrunt/internal/errors"
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
// Passing dependencies that that haven't been checked for cycles is unsafe.
// If any cycles are present, the queue construction will halt after N
// iterations, where N is the number of discovered configs, and throw an error.
func NewQueue(discovered discovery.DiscoveredConfigs) (*Queue, error) {
	if len(discovered) == 0 {
		return &Queue{
			entries: discovered,
		}, nil
	}

	// Create a map for O(1) lookups of configs by path
	configMap := make(map[string]*discovery.DiscoveredConfig, len(discovered))
	for _, cfg := range discovered {
		configMap[cfg.Path] = cfg
	}

	// Track if a given config has been processed
	visited := make(map[string]bool, len(discovered))

	// result will store configs in dependency order
	result := make(discovery.DiscoveredConfigs, 0, len(discovered))

	// depthBudget is initially the maximum dependency depth of the queue
	// Given that a cycle-free graph has a maximum depth of N, we can
	// use the length of the discovered configs as a safe upper bound.
	depthBudget := len(discovered)

	// Process nodes level by level, with deterministic ordering within each level
	var processLevel func(configs discovery.DiscoveredConfigs) error
	processLevel = func(configs discovery.DiscoveredConfigs) error {
		// We need to check to see if we've reached
		// the maximum allowed iterations.
		if depthBudget < 0 {
			return errors.New("cycle detected during queue construction")
		}

		depthBudget--

		levelNodes := make([]*discovery.DiscoveredConfig, 0, len(configs))

		for _, cfg := range configs {
			if visited[cfg.Path] {
				continue
			}

			hasUnprocessedDeps := false

			// Only consider dependencies that exist in our discovered configs
			for _, dep := range cfg.Dependencies {
				if _, exists := configMap[dep.Path]; !exists {
					continue // Skip dependencies that don't exist in our discovered configs
				}

				if !visited[dep.Path] {
					hasUnprocessedDeps = true
					break
				}
			}

			if !hasUnprocessedDeps {
				levelNodes = append(levelNodes, cfg)
			}
		}

		// Sort nodes by path for deterministic ordering within each level
		sort.SliceStable(levelNodes, func(i, j int) bool {
			return levelNodes[i].Path < levelNodes[j].Path
		})

		// Add all nodes at this level to result
		for _, node := range levelNodes {
			visited[node.Path] = true

			result = append(result, node)
		}

		// If every node has been visited, we're done
		if len(visited) == len(discovered) {
			return nil
		}

		// Process next level
		return processLevel(configs)
	}

	// Start with all configs
	if err := processLevel(discovered); err != nil {
		return &Queue{
			entries: result,
		}, err
	}

	return &Queue{
		entries: result,
	}, nil
}
