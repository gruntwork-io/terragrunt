// Package queue provides a run queue implementation.
// The queue is a double-ended queue (deque) that allows for efficient adding and removing of elements from both ends.
// The queue is used to manage the order of Terragrunt runs.
//
// The algorithm for populating the queue is as follows:
//  1. Given a list of discovered configurations, start with an empty queue.
//  2. Append every configuration that waits on nothing, sorted alphabetically.
//  3. Repeatedly append, sorted alphabetically, every configuration whose waits are all already in the queue.
//
// The resulting queue will have:
// - Configurations with no dependencies at the front
// - Configurations with dependents are ordered after their dependencies
// - Alphabetical ordering within each level of the dependency graph
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
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/topo"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Entry represents a node in the execution queue/DAG. Each Entry corresponds to a single Terragrunt configuration
// and tracks its execution status and relationships to other entries in the queue.
type Entry struct {
	// Component is the Terragrunt configuration associated with this entry. It contains all metadata about the unit/stack,
	// including its path, dependencies, and discovery context (such as the command being run).
	Component component.Component

	// Status represents the current lifecycle state of this entry in the
	// queue. It tracks whether the entry is pending, blocked, ready,
	// running, succeeded, or failed. Status is updated as dependencies
	// are resolved and as execution progresses.
	Status Status
}

// Status represents the lifecycle state of a task in the queue.
type Status byte

const (
	StatusPending Status = iota
	StatusBlocked
	StatusReady
	StatusRunning
	StatusSucceeded
	StatusFailed
	StatusEarlyExit // Terminal status set on Entries in case of fail fast mode
)

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

	if e.Component.DiscoveryContext().Cmd == "apply" &&
		slices.Contains(e.Component.DiscoveryContext().Args, "-destroy") {
		return false
	}

	if e.Component.DiscoveryContext().Cmd == "plan" &&
		slices.Contains(e.Component.DiscoveryContext().Args, "-destroy") {
		return false
	}

	return true
}

type Queue struct {
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
// An "up" entry waits on its "up" dependencies, and a "down" entry waits on its "down" dependents. Entries of the
// other direction never gate one another.
//
// Returns an error when the entries wait on one another in a cycle. The returned queue then holds the entries that
// could be ordered, as [StatusReady], followed by the rest, as [StatusBlocked], in discovery order.
func NewQueue(discovered component.Components) (*Queue, error) {
	if len(discovered) == 0 {
		return &Queue{
			Entries: Entries{},
		}, nil
	}

	entries := make(Entries, 0, len(discovered))

	for _, cfg := range discovered {
		entries = append(entries, &Entry{
			Component: cfg,
			Status:    StatusPending,
		})
	}

	graph := entryGraph(entries)
	sorted := make(Entries, 0, len(entries))

	for ready := graph.Roots(); len(ready) > 0; {
		slices.SortStableFunc(ready, func(a, b *Entry) int {
			return strings.Compare(a.Component.Path(), b.Component.Path())
		})

		var next Entries

		for _, entry := range ready {
			entry.Status = StatusReady
			sorted = append(sorted, entry)
			next = append(next, graph.Done(entry)...)
		}

		ready = next
	}

	q := &Queue{
		Entries: sorted,
	}

	if len(sorted) == len(entries) {
		return q, nil
	}

	for _, entry := range entries {
		if entry.Status != StatusReady {
			entry.Status = StatusBlocked
			q.Entries = append(q.Entries, entry)
		}
	}

	return q, errors.New("cycle detected during queue construction")
}

// entryGraph returns a graph over entries in which an "up" entry waits on its "up" dependencies and a "down" entry
// waits on its "down" dependents. Dependencies outside entries are not waited on.
func entryGraph(entries Entries) *topo.Graph[*Entry] {
	byPath := make(map[string]*Entry, len(entries))
	byComponent := make(map[component.Component]*Entry, len(entries))

	for _, entry := range entries {
		if _, ok := byPath[entry.Component.Path()]; !ok {
			byPath[entry.Component.Path()] = entry
		}

		if _, ok := byComponent[entry.Component]; !ok {
			byComponent[entry.Component] = entry
		}
	}

	downWaits := map[*Entry]Entries{}

	for _, entry := range entries {
		if entry.IsUp() {
			continue
		}

		for _, dep := range entry.Component.Dependencies() {
			depEntry := byComponent[dep]
			if depEntry != nil && !depEntry.IsUp() {
				downWaits[depEntry] = append(downWaits[depEntry], entry)
			}
		}
	}

	graph := topo.New[*Entry](len(entries))

	var upWaits Entries

	for _, entry := range entries {
		if !entry.IsUp() {
			graph.Add(entry, downWaits[entry]...)
			continue
		}

		upWaits = upWaits[:0]

		for _, dep := range entry.Component.Dependencies() {
			depEntry := byPath[dep.Path()]
			if depEntry != nil && depEntry.IsUp() {
				upWaits = append(upWaits, depEntry)
			}
		}

		graph.Add(entry, upWaits...)
	}

	return graph
}

// GetReadyWithDependencies returns all entries that are ready to run and
// have all dependencies completed (or no dependencies).
func (q *Queue) GetReadyWithDependencies(l log.Logger) []*Entry {
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
			if q.areDependenciesReadyUnsafe(l, e) {
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
// For up commands, only other up-command dependencies must be in a succeeded state (or terminal if
// ignoring errors). Down-command (destroy) dependencies are skipped, mirroring [NewQueue]:
// a unit being destroyed does not gate a unit being applied or planned.
// If a dependency is not in the queue, it is assumed to have existing state.
// Should only be called when the caller already holds a read lock.
func (q *Queue) areDependenciesReadyUnsafe(l log.Logger, e *Entry) bool {
	for _, dep := range e.Component.Dependencies() {
		depEntry := q.entryByPathUnsafe(dep.Path())
		if depEntry == nil {
			l.Debugf("Dependency %s is not in queue, considering it ready", dep.Path())

			continue
		}

		// Skip down-command (destroy) dependencies: they run in reverse order and must
		// not block an up-command entry from proceeding. Mirrors NewQueue.
		if !depEntry.IsUp() {
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
// For down commands, only other down-command dependents must be in a succeeded state (or terminal
// if ignoring errors). Up-command (plan/apply) dependents are skipped, mirroring
// [NewQueue]: a unit being planned or applied does not gate a unit being destroyed.
// Should only be called when the caller already holds a read lock.
func (q *Queue) areDependentsReadyUnsafe(e *Entry) bool {
	for _, other := range q.Entries {
		if other == e || len(other.Component.Dependencies()) == 0 {
			continue
		}

		// Skip up-command (plan/apply) dependents: they run in forward order and must
		// not block a down-command entry from proceeding. Mirrors NewQueue.
		if other.IsUp() {
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
//
// If the entry is already in a terminal state (StatusSucceeded, StatusFailed, or StatusEarlyExit),
// this operation is a no-op. This prevents race conditions where a concurrent success could
// overwrite an early-exit status set by fail-fast mode.
func (q *Queue) SetEntryStatus(e *Entry, status Status) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if isTerminal(e.Status) {
		return
	}

	e.Status = status
}

// ClaimForRunning atomically transitions the entry to StatusRunning under the
// queue lock and returns true. Returns false without changing the status when
// the entry is already in a terminal state, e.g. a concurrent FailEntry set it
// to StatusEarlyExit between the caller's snapshot and this claim.
func (q *Queue) ClaimForRunning(e *Entry) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if isTerminal(e.Status) {
		return false
	}

	e.Status = StatusRunning

	return true
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
	case StatusPending, StatusBlocked, StatusReady, StatusRunning:
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
