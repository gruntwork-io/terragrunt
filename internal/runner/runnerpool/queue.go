package runnerpool

import (
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// queue keeps DAG state; NO goroutines here.
type queue struct {
	entries  []*entry
	byID     map[string]*entry
	failFast bool
	mu       sync.Mutex
}

func buildQueue(units []*runbase.Unit, failFast bool) *queue {
	q := &queue{byID: map[string]*entry{}, failFast: failFast}

	// 1. Wrap each runbase.Unit as Task → entry
	for _, u := range units {
		t := &Task{Unit: u}
		e := &entry{task: t, status: StatusPending}
		q.entries = append(q.entries, e)
		q.byID[t.ID()] = e
	}

	// 2. Wire dependencies
	for _, e := range q.entries {
		for _, depID := range e.task.Parents() {
			if p, ok := q.byID[depID]; ok {
				e.blockedBy = append(e.blockedBy, p)
			}
		}
		if len(e.blockedBy) > 0 {
			e.status = StatusBlocked
		}
	}
	// 3. Roots become ready
	for _, e := range q.entries {
		if e.status == StatusPending {
			e.status = StatusReady
		}
	}
	return q
}

// getReady returns up to n ready entries (non‑blocking).
func (q *queue) getReady(n int) []*entry {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []*entry
	for _, e := range q.entries {
		if e.status == StatusReady {
			out = append(out, e)
			if len(out) == n {
				break
			}
		}
	}
	return out
}

func (q *queue) markRunning(e *entry) {
	q.mu.Lock()
	e.status = StatusRunning
	q.mu.Unlock()
}

func (q *queue) done(e *entry, res Result) {
	q.mu.Lock()
	e.result = res
	if res.ExitCode == 0 && res.Err == nil {
		e.status = StatusSucceeded
	} else {
		e.status = StatusFailed
	}

	// Global fail‑fast toggles all remaining entries
	if q.failFast && e.status == StatusFailed {
		for _, other := range q.entries {
			switch other.status {
			case StatusPending, StatusReady, StatusBlocked:
				other.status = StatusFailFast
			}
		}
		q.mu.Unlock()
		return
	}

	// Unblock children / mark ancestor failures
	for _, child := range q.entries {
		if child.status != StatusBlocked {
			continue
		}
		ready := true
		for _, p := range child.blockedBy {
			if p.status != StatusSucceeded {
				ready = false
				if p.status == StatusFailed || p.status == StatusAncestorFailed || p.status == StatusFailFast {
					child.status = StatusAncestorFailed
				}
				break
			}
		}
		if ready {
			child.status = StatusReady
		}
	}
	q.mu.Unlock()
}

func (q *queue) empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.entries {
		switch e.status {
		case StatusPending, StatusBlocked, StatusReady, StatusRunning:
			return false
		}
	}
	return true
}
