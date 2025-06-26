package runnerpool

import (
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// dagQueue holds task nodes and tracks their dependency state in a
// concurrency‑safe way. Each child keeps a `remainingDeps` counter so we never
// rescan the whole map when a parent finishes.
type dagQueue struct {
	mu       sync.Mutex
	entries  map[string]*entry // lookup by ID
	ordered  []*entry          // preserved insertion order
	failFast bool
}

// buildQueue initialises the DAG from the immutable runbase.Unit slice.
func buildQueue(units []*runbase.Unit, failFast bool) *dagQueue {
	q := &dagQueue{entries: make(map[string]*entry), failFast: failFast}

	// 1. Create entries (all start as pending)
	for _, u := range units {
		e := &entry{task: &Task{Unit: u}, state: StatusPending}
		q.entries[e.task.ID()] = e
		q.ordered = append(q.ordered, e)
	}

	// 2. Wire parent/child edges and set starting state.
	for _, e := range q.entries {
		for _, parentPath := range e.task.Parents() {
			if p, ok := q.entries[parentPath]; ok {
				e.blockedBy = append(e.blockedBy, p)
				p.dependents = append(p.dependents, e)
			}
		}
		e.remainingDeps = len(e.blockedBy)
		if e.remainingDeps == 0 {
			e.state = StatusReady
		} else {
			e.state = StatusBlocked
		}
	}
	return q
}

// getReady returns up to `max` entries that are ready to run and marks them
// as Running to prevent double scheduling.
func (q *dagQueue) getReady(max int) []*entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	var out []*entry
	for _, e := range q.ordered {
		if len(out) >= max {
			break
		}
		if e.state == StatusReady {
			e.state = StatusRunning
			out = append(out, e)
		}
	}
	return out
}

// markDone updates an entry's state, cascades success or failure,
// and (optionally) flips every still-pending node to StatusFailFast.
func (q *dagQueue) markDone(e *entry, failFast bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 1. set this node’s final state
	switch e.state {
	case StatusRunning:
		if e.result.ExitCode == 0 && e.result.Err == nil {
			e.state = StatusSucceeded
		} else {
			e.state = StatusFailed
			if failFast {
				// 3a. fail-fast: skip everything not started yet
				for _, n := range q.ordered {
					if n.state == StatusPending ||
						n.state == StatusBlocked ||
						n.state == StatusReady {
						n.state = StatusFailFast
					}
				}
			}
		}
	}

	success := e.state == StatusSucceeded

	// 2. walk children and update their counters / states
	for _, child := range e.dependents {
		if success {
			child.remainingDeps--
			if child.remainingDeps == 0 && child.state == StatusBlocked {
				child.state = StatusReady
			}
		} else {
			if child.state == StatusPending || child.state == StatusBlocked {
				child.state = StatusAncestorFailed
			}
		}
	}
}

// empty reports whether any tasks are still runnable or running.
func (q *dagQueue) empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.entries {
		switch e.state {
		case StatusPending, StatusBlocked, StatusReady, StatusRunning:
			return false
		}
	}
	return true
}

// results returns all task results in undefined order (call after empty()==true).
func (q *dagQueue) results() []Result {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]Result, 0, len(q.entries))
	for _, e := range q.entries {
		out = append(out, e.result)
	}
	return out
}
