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

// dagQueue holds task nodes and tracks their dependency state in a
// concurrency‑safe way. Each child keeps a `remainingDeps` counter so we never
// rescan the whole map when a parent finishes.
type dagQueue struct {
	mu       sync.Mutex
	index    map[string]*entry // canonical ID → entry
	ordered  []*entry          // slice preserves caller order
	failFast bool
}

func buildQueue(units []*runbase.Unit, failFast bool) *dagQueue {
	q := &dagQueue{
		index:    make(map[string]*entry, len(units)),
		ordered:  make([]*entry, 0, len(units)),
		failFast: failFast,
	}

	// 1. create entries
	for _, u := range units {
		e := &entry{task: taskFromUnit(u), state: StatusPending}
		q.index[e.task.ID()] = e
		q.ordered = append(q.ordered, e)
	}

	// 2. wire dependencies and dependents, set initial status / counters
	for _, e := range q.ordered {
		parents := e.task.Parents()
		e.remainingDeps = len(parents)

		if e.remainingDeps == 0 {
			e.state = StatusReady
		} else {
			e.state = StatusBlocked
		}

		for _, pid := range parents {
			if p, ok := q.index[pid]; ok {
				p.dependents = append(p.dependents, e)
			}
		}
	}
	return q
}

// -----------------------------------------------------------------------------
// Scheduling helpers
// -----------------------------------------------------------------------------

// getReady returns up to max ready entries, respecting the original order.
func (q *dagQueue) getReady() []*entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]*entry, 0, len(q.ordered))
	for _, e := range q.ordered {
		if e.state == StatusReady {
			e.state = StatusRunning
			out = append(out, e)
		}
	}
	return out
}

// markDone records the result, unblocks children, and handles fail-fast.
func (q *dagQueue) markDone(e *entry, failFast bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if e.state != StatusRunning {
		return // double call safeguard
	}

	if e.result.ExitCode == 0 && e.result.Err == nil {
		e.state = StatusSucceeded
	} else {
		e.state = StatusFailed
		if failFast {
			for _, n := range q.ordered {
				switch n.state {
				case StatusPending, StatusBlocked, StatusReady:
					n.state = StatusFailFast
				}
			}
		}
	}

	success := e.state == StatusSucceeded

	for _, child := range e.dependents {
		switch {
		case success:
			child.remainingDeps--
			if child.remainingDeps == 0 && child.state == StatusBlocked {
				child.state = StatusReady
			}
		default:
			if child.state == StatusPending || child.state == StatusBlocked {
				child.state = StatusAncestorFailed
			}
		}
	}
}

// empty reports when no runnable or running tasks remain.
func (q *dagQueue) empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.ordered {
		if e.state == StatusReady || e.state == StatusRunning {
			return false
		}
	}
	return true
}

// results returns final Result slice in original unit order.
func (q *dagQueue) results() []Result {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]Result, len(q.ordered))
	for i, e := range q.ordered {
		res := e.result
		// If the task was skipped due to fail-fast or ancestor failure, set ExitCode and Err
		switch e.state {
		case StatusFailFast:
			if res.ExitCode == 0 && res.Err == nil {
				res.ExitCode = 1 // Use 1 for skipped due to fail-fast
				res.Err = ErrSkippedFailFast
			}
		case StatusAncestorFailed:
			if res.ExitCode == 0 && res.Err == nil {
				res.ExitCode = 1 // Use 1 for skipped due to ancestor failure
				// Find all failed ancestors
				var failedAncestors []string
				for _, pid := range e.task.Parents() {
					if parent, ok := q.index[pid]; ok && parent.state != StatusSucceeded {
						failedAncestors = append(failedAncestors, pid)
					}
				}
				if len(failedAncestors) > 0 {
					res.Err = errors.New("skipped due to ancestor failure: " + strings.Join(failedAncestors, ", "))
				} else {
					res.Err = ErrSkippedAncestorFailed
				}
			}
		}
		out[i] = res
	}
	return out
}

func (q *dagQueue) summarizeStates() string {
	q.mu.Lock()
	defer q.mu.Unlock()

	counts := make(map[Status]int)
	ids := make(map[Status][]string)
	for _, e := range q.ordered {
		counts[e.state]++
		if e.state != StatusSucceeded {
			ids[e.state] = append(ids[e.state], e.task.ID())
		}
	}
	// Build a summary string
	summary := ""
	for s, c := range counts {
		summary += s.String() + ":" + itoa(c)
		if len(ids[s]) > 0 {
			summary += " [" + strings.Join(ids[s], ",") + "]"
		}
		summary += ", "
	}
	return summary
}

// Helper to convert int to string without fmt
func itoa(i int) string {
	return string('0' + i)
}

// Add String() method for Status for readable output
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
