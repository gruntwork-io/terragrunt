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
	"errors"
	"slices"
	"sort"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
)

type Entry struct {
	Config *discovery.DiscoveredConfig
	Status Status
}

type Status byte

const (
	StatusPending Status = iota
	StatusBlocked
	StatusUnsorted
	StatusReady
)

// UpdateBlocked updates the status of the entry to blocked, if it is blocked.
// An entry is blocked if:
//  1. It is an "up" command (none of destroy, apply -destroy or plan -destroy)
//     and it has dependencies that are not ready.
//  2. It is a "down" command (destroy, apply -destroy or plan -destroy)
//     and it has dependents that are not ready.
//
// If the entry isn't blocked, then it is marked as unsorted, and is ready to be sorted.
func (e *Entry) UpdateBlocked(entries Entries) {
	// If the entry is already ready, we can skip the rest of the logic.
	if e.Status == StatusReady {
		return
	}

	if e.IsUp() {
		for _, dep := range e.Config.Dependencies {
			depEntry := entries.Entry(dep)
			if depEntry == nil {
				continue
			}

			if !depEntry.IsUp() {
				continue
			}

			if depEntry.Status != StatusReady {
				e.Status = StatusBlocked
				return
			}
		}

		e.Status = StatusUnsorted

		return
	}

	// If the entry is a "down" command, we need to check if all of its dependents are ready.
	for _, qEntry := range entries {
		if qEntry.Config.Dependencies == nil {
			continue
		}

		if !slices.Contains(qEntry.Config.Dependencies, e.Config) {
			continue
		}

		if qEntry.IsUp() {
			continue
		}

		if qEntry.Status != StatusReady {
			e.Status = StatusBlocked
			return
		}
	}

	e.Status = StatusUnsorted
}

// IsUp returns true if the entry is an "up" command.
func (e *Entry) IsUp() bool {
	// If we don't have a discovery context,
	// we should assume the command is an "up" command.
	if e.Config.DiscoveryContext == nil {
		return true
	}

	if e.Config.DiscoveryContext.Cmd == "destroy" {
		return false
	}

	if e.Config.DiscoveryContext.Cmd == "apply" && slices.Contains(e.Config.DiscoveryContext.Args, "-destroy") {
		return false
	}

	if e.Config.DiscoveryContext.Cmd == "plan" && slices.Contains(e.Config.DiscoveryContext.Args, "-destroy") {
		return false
	}

	return true
}

type Queue struct {
	Entries Entries
}

type Entries []*Entry

// Entry returns a given entry from the queue.
func (e Entries) Entry(cfg *discovery.DiscoveredConfig) *Entry {
	for _, entry := range e {
		if entry.Config.Path == cfg.Path {
			return entry
		}
	}

	return nil
}

// Configs returns the queue configs.
func (q *Queue) Configs() discovery.DiscoveredConfigs {
	result := make(discovery.DiscoveredConfigs, 0, len(q.Entries))
	for _, entry := range q.Entries {
		result = append(result, entry.Config)
	}

	return result
}

// NewQueue creates a new queue from a list of discovered configurations.
// The queue is populated with the correct Terragrunt run order.
//
// Discovered configurations will be sorted based on two criteria:
//
//  1. The discovery context of the configuration:
//     - If the configuration is for an "up" command (none of destroy, apply -destroy or plan -destroy),
//     it will be inserted at the front of the queue, before its dependencies.
//     - Otherwise, it is considered a "down" command, and will be inserted at the back of the queue,
//     after its dependents.
//
//  2. The name of the configuration. Configurations of the same "level" are sorted alphabetically.
//
// Passing configurations that haven't been checked for cycles in their dependency graph is unsafe.
// If any cycles are present, the queue construction will halt after N
// iterations, where N is the number of discovered configs, and throw an error.
func NewQueue(discovered discovery.DiscoveredConfigs) (*Queue, error) {
	if len(discovered) == 0 {
		return &Queue{
			Entries: Entries{},
		}, nil
	}

	// First, we need to take all the discovered configs
	// and assign them a status of pending.
	entries := make(Entries, 0, len(discovered))

	for _, cfg := range discovered {
		entries = append(entries, &Entry{
			Config: cfg,
			Status: StatusPending,
		})
	}

	q := &Queue{
		Entries: entries,
	}

	// readyPending returns the index of the first pending entry if there is one,
	// or -1 if there are no pending entries.
	readyPending := func(entries Entries) int {
		// Next, we need to iterate through the entries
		// and check if any of them are blocked.
		for _, entry := range entries {
			entry.UpdateBlocked(entries)
		}

		// Next, we need to sort the entries by status and path.
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].Status > entries[j].Status {
				return true
			}

			if entries[i].Status == StatusUnsorted && entries[j].Status == StatusUnsorted {
				return entries[i].Config.Path < entries[j].Config.Path
			}

			return false
		})

		// Now, we can mark all unsorted entries as ready,
		// and check if all entries are ready.
		for idx, entry := range entries {
			if entry.Status == StatusUnsorted {
				entry.Status = StatusReady
			}

			if entry.Status != StatusReady {
				return idx
			}
		}

		return -1
	}

	// We need to iterate through the entries until all entries are ready.
	// We can use the length of the entries as a safe upper bound for the number of iterations,
	// because a cycle-free graph has a maximum depth of N, where N is the number of discovered configs.
	maxIterations := len(entries)

	// We keep track of the index of the first pending entry
	// to save us from iterating through the entire list of entries
	// on each iteration.
	firstPending := 0

	for range maxIterations {
		firstPending = readyPending(entries[firstPending:])
		if firstPending == -1 {
			return q, nil
		}
	}

	return q, errors.New("cycle detected during queue construction")
}
