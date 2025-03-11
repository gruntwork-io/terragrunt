// Package queue provides a run queue implementation.
// The queue is a double-ended queue (deque) that allows for efficient adding and removing of elements from both ends.
// The queue is used to manage the order of Terragrunt runs.
//
// The algorithm for populating the queue is as follows:
// 1. Given a list of discovered configurations, start with an empty queue.
// 2. For each discovered configuration, add the configuration to the queue as close to the front as possible.
// 3. When a configuration is added to the queue, check to see if any entry in the queue depends on the configuration.
// 4. If a configuration is found that has a dependency on the configuration being added, add the configuration in front of the configuration being added.
// 5. Repeat step 2 until all configurations are in the queue.
// 6. The queue is now populated with the Terragrunt run order.
//
// During operations like applies, entries will be dequeued from the front of the queue and run.
// During operations like destroys, entries will be dequeued from the back of the queue and run.
// This is to ensure that dependencies are run first for applies, and dependents are run first for destroys.
package queue

import "github.com/gruntwork-io/terragrunt/internal/discovery"

type Queue struct {
	entries []*discovery.DiscoveredConfig
}

// Entries returns the queue entries. Used for testing.
func (q *Queue) Entries() []*discovery.DiscoveredConfig {
	return q.entries
}

// NewQueue creates a new queue from a list of discovered configurations.
// The queue is populated with the correct Terragrunt run order.
func NewQueue(discovered []*discovery.DiscoveredConfig) *Queue {
	entries := make([]*discovery.DiscoveredConfig, 0, len(discovered))

	for _, cfg := range discovered {
		inserted := false
		// Try to insert the config at each position, starting from the front
		for i := 0; i < len(entries); i++ {
			// Check if any existing entry depends on the new config
			// in its ancestry.
			if entries[i].ContainsDependencyInAncestry(cfg.Path) {
				// Insert cfg before the dependent entry
				entries = append(entries[:i], append([]*discovery.DiscoveredConfig{cfg}, entries[i:]...)...)
				inserted = true

				break
			}
		}

		// If no dependencies were found, append to the end
		if !inserted {
			entries = append(entries, cfg)
		}
	}

	return &Queue{
		entries: entries,
	}
}
