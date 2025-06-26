package runnerpool

import (
	"sync"

	"errors"

	"strings"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

var (
	ErrSkippedFailFast       = errors.New("skipped due to fail-fast")
	ErrSkippedAncestorFailed = errors.New("skipped due to ancestor failure")
)

// DagQueue holds Task nodes and tracks their dependency State in a
// concurrency‑safe way. Each child keeps a `RemainingDeps` counter so we never
// rescan the whole map when a parent finishes.
type DagQueue struct {
	Index    map[string]*Entry
	Ordered  []*Entry
	mu       sync.Mutex
	failFast bool
}

func BuildQueue(units []*runbase.Unit, failFast bool) *DagQueue {
	q := &DagQueue{
		Index:    make(map[string]*Entry, len(units)),
		Ordered:  make([]*Entry, 0, len(units)),
		failFast: failFast,
	}

	// 1. create entries
	for _, u := range units {
		e := &Entry{Task: taskFromUnit(u), State: StatusPending}
		q.Index[e.Task.ID()] = e
		q.Ordered = append(q.Ordered, e)
	}

	// 2. wire dependencies and Dependents, set initial status / counters
	for _, e := range q.Ordered {
		parents := e.Task.Parents()
		e.RemainingDeps = len(parents)

		if e.RemainingDeps == 0 {
			e.State = StatusReady
		} else {
			e.State = StatusBlocked
		}

		for _, pid := range parents {
			if p, ok := q.Index[pid]; ok {
				p.Dependents = append(p.Dependents, e)
			}
		}
	}

	return q
}

// GetReady returns up to max ready entries, respecting the original order.
func (q *DagQueue) GetReady() []*Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]*Entry, 0, len(q.Ordered))

	for _, e := range q.Ordered {
		if e.State == StatusReady {
			e.State = StatusRunning
			out = append(out, e)
		}
	}

	return out
}

// MarkDone records the Result, unblocks children, and handles fail-fast.
func (q *DagQueue) MarkDone(e *Entry, failFast bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if e.State != StatusRunning {
		return // double call safeguard
	}

	if e.Result.ExitCode == 0 && e.Result.Err == nil {
		e.State = StatusSucceeded
	} else {
		e.State = StatusFailed

		if failFast {
			for _, n := range q.Ordered {
				switch n.State {
				case StatusPending, StatusBlocked, StatusReady:
					n.State = StatusFailFast
				case StatusRunning, StatusSucceeded, StatusFailed, StatusAncestorFailed, StatusFailFast:
					// no op
					continue
				}
			}
		}
	}

	success := e.State == StatusSucceeded

	for _, child := range e.Dependents {
		switch {
		case success:
			child.RemainingDeps--
			if child.RemainingDeps == 0 && child.State == StatusBlocked {
				child.State = StatusReady
			}
		default:
			if child.State == StatusPending || child.State == StatusBlocked {
				child.State = StatusAncestorFailed
			}
		}
	}
}

// Empty reports when no runnable or running tasks remain.
func (q *DagQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.Ordered {
		if e.State == StatusReady || e.State == StatusRunning {
			return false
		}
	}

	return true
}

// Results returns final Result slice in original unit order.
func (q *DagQueue) Results() []Result {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]Result, len(q.Ordered))

	for i, e := range q.Ordered {
		if e.State == StatusFailFast {
			e.Result.ExitCode = 1 // Use 1 for skipped due to fail-fast
			e.Result.Err = ErrSkippedFailFast
		}

		if e.State == StatusAncestorFailed {
			e.Result.ExitCode = 1 // Use 1 for skipped due to ancestor failure
			// Find all failed ancestors
			var failedAncestors []string

			for _, pid := range e.Task.Parents() {
				if parent, ok := q.Index[pid]; ok && parent.State != StatusSucceeded {
					failedAncestors = append(failedAncestors, pid)
				}
			}

			if len(failedAncestors) > 0 {
				e.Result.Err = errors.New("skipped due to ancestor failure: " + strings.Join(failedAncestors, ", "))
			} else {
				e.Result.Err = ErrSkippedAncestorFailed
			}
		}

		out[i] = e.Result
	}

	return out
}

// String text representation of the status.
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "Pending"
	case StatusBlocked:
		return "Blocked"
	case StatusReady:
		return "Ready"
	case StatusRunning:
		return "Running"
	case StatusSucceeded:
		return "Succeeded"
	case StatusFailed:
		return "Failed"
	case StatusAncestorFailed:
		return "AncestorFailed"
	case StatusFailFast:
		return "FailFast"
	default:
		return "Unknown"
	}
}
