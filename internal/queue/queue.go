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
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
)

// Entry represents a node in the execution queue/DAG. Each Entry corresponds to a single Terragrunt configuration
// and tracks its execution status and relationships to other entries in the queue.
type Entry struct {
	// Component is the Terragrunt configuration associated with this entry. It contains all metadata about the unit/stack,
	// including its path, dependencies, and discovery context (such as the command being run).
	Component component.Component

	// Status represents the current lifecycle state of this entry in the queue. It tracks whether the entry is pending,
	// blocked, ready, running, succeeded, or failed. Status is updated as dependencies are resolved and as execution progresses.
	Status Status
}

// Status represents the lifecycle state of a task in the queue.
type Status byte

const (
	StatusPending Status = iota
	StatusBlocked
	StatusUnsorted
	StatusReady
	StatusRunning
	StatusSucceeded
	StatusFailed
	StatusEarlyExit // Terminal status set on Entries in case of fail fast mode
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
		for _, dep := range e.Component.Dependencies() {
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
		if len(qEntry.Component.Dependencies()) == 0 {
			continue
		}

		if !slices.Contains(qEntry.Component.Dependencies(), e.Component) {
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
	if e.Component.DiscoveryContext() == nil {
		return true
	}

	if e.Component.DiscoveryContext().Cmd == "destroy" {
		return false
	}

	if e.Component.DiscoveryContext().Cmd == "apply" && slices.Contains(e.Component.DiscoveryContext().Args, "-destroy") {
		return false
	}

	if e.Component.DiscoveryContext().Cmd == "plan" && slices.Contains(e.Component.DiscoveryContext().Args, "-destroy") {
		return false
	}

	return true
}

type Queue struct {
	// unitsMap is a map of unit paths to Unit objects, used to check if dependencies not in the queue
	// are assumed already applied or have existing state.
	unitsMap map[string]*component.Unit
	// Entries is a list of entries in the queue.
	Entries Entries
	// mu is a mutex used to synchronize access to the queue.
	mu sync.RWMutex
	// FailFast, if set to true, causes the queue to fail fast if any entry fails.
	FailFast bool
	// IgnoreDependencyOrder, if set to true, causes the queue to ignore dependencies when fetching ready entries.
	// When enabled, GetReadyWithDependencies will return all entries with StatusReady, regardless of dependency status.
	IgnoreDependencyOrder bool
	// IgnoreDependencyErrors, if set to true, allows scheduling and running entries even if their
	// dependencies failed. Additionally, failures will not propagate EarlyExit to dependents/dependencies.
	IgnoreDependencyErrors bool
}

type Entries []*Entry

// Entry returns a given entry from the queue.
func (e Entries) Entry(cfg component.Component) *Entry {
	for _, entry := range e {
		if entry.Component.Path() == cfg.Path() {
			return entry
		}
	}

	return nil
}

// Components returns the queue components.
func (q *Queue) Components() component.Components {
	result := make(component.Components, 0, len(q.Entries))
	for _, entry := range q.Entries {
		result = append(result, entry.Component)
	}

	return result
}

// EntryByPath returns the entry with the given config path, or nil if not found.
func (q *Queue) EntryByPath(path string) *Entry {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.entryByPathUnsafe(path)
}

// entryByPathUnsafe returns the entry with the given config path without locking.
// Should only be called when the caller already holds a lock.
func (q *Queue) entryByPathUnsafe(path string) *Entry {
	for _, entry := range q.Entries {
		if entry.Component.Path() == path {
			return entry
		}
	}

	return nil
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
func NewQueue(discovered component.Components) (*Queue, error) {
	if len(discovered) == 0 {
		return &Queue{
			Entries: Entries{},
		}, nil
	}

	// First, we need to take all the discovered configs
	// and assign them a status of pending.
	entries := make(Entries, 0, len(discovered))

	for _, cfg := range discovered {
		entry := &Entry{
			Component: cfg,
			Status:    StatusPending,
		}
		entries = append(entries, entry)
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
				return entries[i].Component.Path() < entries[j].Component.Path()
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

// GetReadyWithDependencies returns all entries that are ready to run and have all dependencies completed (or no dependencies).
func (q *Queue) GetReadyWithDependencies() []*Entry {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.IgnoreDependencyOrder {
		out := make([]*Entry, 0, len(q.Entries))

		for _, e := range q.Entries {
			if e.Status == StatusReady {
				out = append(out, e)
			}
		}

		return out
	}

	out := make([]*Entry, 0, len(q.Entries))

	for _, e := range q.Entries {
		if e.Status != StatusReady {
			continue
		}

		if e.IsUp() {
			if q.areDependenciesReadyUnsafe(e) {
				out = append(out, e)
			}

			continue
		}

		if q.areDependentsReadyUnsafe(e) {
			out = append(out, e)
		}
	}

	return out
}

// areDependenciesReadyUnsafe checks if all dependencies of an entry are ready for "up" commands.
// For up commands, all dependencies must be in a succeeded state (or terminal if ignoring errors).
// If a dependency is not in the queue, it is assumed to have existing state.
// Should only be called when the caller already holds a read lock.
func (q *Queue) areDependenciesReadyUnsafe(e *Entry) bool {
	for _, dep := range e.Component.Dependencies() {
		depEntry := q.entryByPathUnsafe(dep.Path())
		if depEntry == nil {
			// Dependency not in queue - check if assumed already applied
			if q.unitsMap != nil {
				if unit, ok := q.unitsMap[dep.Path()]; ok {
					if unit.Execution != nil && unit.Execution.AssumeAlreadyApplied {
						// Dependency is assumed already applied, consider it ready
						continue
					}
				}
			}
			// If not in queue and not assumed already applied,
			// assume it has existing state
			continue
		}

		// When ignoring dependency errors, allow scheduling if dependencies are in a terminal state
		// (succeeded OR failed), not just succeeded
		if q.IgnoreDependencyErrors {
			if !isTerminal(depEntry.Status) {
				return false
			}

			continue
		}

		if depEntry.Status != StatusSucceeded {
			return false
		}
	}

	return true
}

// areDependentsReadyUnsafe checks if all dependents of an entry are ready for "down" commands.
// For down commands, all dependents must be in a succeeded state (or terminal if ignoring errors).
// Should only be called when the caller already holds a read lock.
func (q *Queue) areDependentsReadyUnsafe(e *Entry) bool {
	for _, other := range q.Entries {
		if other == e || len(other.Component.Dependencies()) == 0 {
			continue
		}

		for _, dep := range other.Component.Dependencies() {
			if dep.Path() == e.Component.Path() {
				// When ignoring dependency errors, allow scheduling if dependents are in a terminal state
				// (succeeded OR failed), not just succeeded
				if q.IgnoreDependencyErrors {
					if !isTerminal(other.Status) {
						return false
					}

					continue
				}

				if other.Status != StatusSucceeded {
					return false
				}
			}
		}
	}

	return true
}

// SetEntryStatus safely sets the status of an entry with proper synchronization.
func (q *Queue) SetEntryStatus(e *Entry, status Status) {
	q.mu.Lock()
	defer q.mu.Unlock()

	e.Status = status
}

// FailEntry marks the entry as failed and updates related entries if needed.
// For up commands, this marks entries that come after this one as early exit.
// For destroy/down commands, this marks entries that come before this one as early exit.
// Use only for failure transitions. For other status changes, set Status directly.
func (q *Queue) FailEntry(e *Entry) {
	q.mu.Lock()
	defer q.mu.Unlock()

	e.Status = StatusFailed

	// If this entry failed and has dependents/dependencies, we need to propagate the failure.
	if q.FailFast {
		for _, n := range q.Entries {
			if isTerminalOrRunning(n.Status) {
				continue
			}

			n.Status = StatusEarlyExit
		}

		return
	}

	// If ignoring dependency errors, do not propagate early exit to other entries.
	if q.IgnoreDependencyErrors {
		return
	}

	if e.IsUp() {
		q.earlyExitDependents(e)
		return
	}

	q.earlyExitDependencies(e)
}

// earlyExitDependents - Recursively mark all entries that are dependent on this one as early exit.
func (q *Queue) earlyExitDependents(e *Entry) {
	for _, entry := range q.Entries {
		if len(entry.Component.Dependencies()) == 0 {
			continue
		}

		for _, dep := range entry.Component.Dependencies() {
			if dep.Path() == e.Component.Path() {
				if isTerminalOrRunning(entry.Status) {
					continue
				}

				entry.Status = StatusEarlyExit

				q.earlyExitDependents(entry)

				break
			}
		}
	}
}

// earlyExitDependencies - Recursively mark all entries that are dependencies on this one as early exit.
func (q *Queue) earlyExitDependencies(e *Entry) {
	if len(e.Component.Dependencies()) == 0 {
		return
	}

	for _, dep := range e.Component.Dependencies() {
		depEntry := q.entryByPathUnsafe(dep.Path())
		if depEntry == nil {
			continue
		}

		if isTerminalOrRunning(depEntry.Status) {
			continue
		}

		depEntry.Status = StatusEarlyExit
		q.earlyExitDependencies(depEntry)
	}
}

// Finished checks if all entries in the queue are in a terminal state (i.e., not pending, blocked, ready, or running).
func (q *Queue) Finished() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, e := range q.Entries {
		if !isTerminal(e.Status) {
			return false
		}
	}

	return true
}

// RemainingDeps Helper to calculate remaining dependencies for an entry.
func (q *Queue) RemainingDeps(e *Entry) int {
	if e.Component == nil || len(e.Component.Dependencies()) == 0 {
		return 0
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	count := 0

	for _, dep := range e.Component.Dependencies() {
		depEntry := q.entryByPathUnsafe(dep.Path())
		if depEntry == nil || depEntry.Status != StatusSucceeded {
			count++
		}
	}

	return count
}

// isTerminal returns true if the status is terminal.
func isTerminal(status Status) bool {
	switch status {
	case StatusPending, StatusBlocked, StatusUnsorted, StatusReady, StatusRunning:
		return false
	case StatusSucceeded, StatusFailed, StatusEarlyExit:
		return true
	}

	return false
}

// isTerminalOrRunning returns true if the status is terminal or running.
func isTerminalOrRunning(status Status) bool {
	return status == StatusRunning || isTerminal(status)
}

// SetUnitsMap sets the units map for the queue. This map is used to check if dependencies
// not in the queue are assumed already applied or have existing state.
func (q *Queue) SetUnitsMap(unitsMap map[string]*component.Unit) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.unitsMap = unitsMap
}
