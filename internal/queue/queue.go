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
	// First sort by path to ensure deterministic ordering of independent items
	sortedConfigs := discovered.Sort()
	entries := make(discovery.DiscoveredConfigs, 0, len(sortedConfigs))

	// Helper to check if all dependencies of a config are in the queue
	hasDependenciesInQueue := func(cfg *discovery.DiscoveredConfig, upToIndex int) bool {
		for _, dep := range cfg.Dependencies {
			found := false

			for i := 0; i <= upToIndex; i++ {
				if entries[i].Path == dep.Path {
					found = true
					break
				}
			}

			if !found {
				return false
			}
		}

		return true
	}

	// Helper to get the index of the last dependency in the queue
	getLastDependencyIndex := func(cfg *discovery.DiscoveredConfig) int {
		lastIndex := -1

		for _, dep := range cfg.Dependencies {
			for i, entry := range entries {
				if entry.Path == dep.Path && i > lastIndex {
					lastIndex = i
				}
			}
		}

		return lastIndex
	}

	// First, add all configs with no dependencies, sorted alphabetically
	var (
		noDeps   discovery.DiscoveredConfigs
		withDeps discovery.DiscoveredConfigs
	)

	for _, cfg := range sortedConfigs {
		if len(cfg.Dependencies) == 0 {
			noDeps = append(noDeps, cfg)
		} else {
			withDeps = append(withDeps, cfg)
		}
	}

	entries = append(entries, noDeps...)

	// Keep processing configs until all are added
	remaining := withDeps
	for len(remaining) > 0 {
		// Find all configs whose dependencies are satisfied
		var (
			nextBatch      discovery.DiscoveredConfigs
			stillRemaining discovery.DiscoveredConfigs
		)

		for _, cfg := range remaining {
			if hasDependenciesInQueue(cfg, len(entries)-1) {
				nextBatch = append(nextBatch, cfg)
			} else {
				stillRemaining = append(stillRemaining, cfg)
			}
		}

		// Sort the next batch by:
		// 1. Last dependency position (primary)
		// 2. Path (secondary, for items with same dependencies)
		sort.SliceStable(nextBatch, func(i, j int) bool {
			iLastDep := getLastDependencyIndex(nextBatch[i])
			jLastDep := getLastDependencyIndex(nextBatch[j])

			if iLastDep != jLastDep {
				return iLastDep < jLastDep
			}

			return nextBatch[i].Path < nextBatch[j].Path
		})

		entries = append(entries, nextBatch...)
		remaining = stillRemaining
	}

	return &Queue{
		entries: entries,
	}
}
