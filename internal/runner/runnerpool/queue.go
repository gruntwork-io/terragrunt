package runnerpool

import (
	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
	"sync"
)

// dagQueue holds all entries and updates their states safely.
type dagQueue struct {
	mu       sync.Mutex
	entries  map[string]*entry // key = TaskID
	failFast bool
}

func buildQueue(units []*runbase.Unit, failFast bool) *dagQueue {
	q := &dagQueue{entries: make(map[string]*entry), failFast: failFast}
	// first pass: create entries
	for _, u := range units {
		e := &entry{task: &Task{Unit: u}, state: statusPending}
		q.entries[e.task.ID()] = e
	}
	// second pass: wire dependencies
	for _, e := range q.entries {
		for _, pid := range e.task.Parents() {
			if parent, ok := q.entries[pid]; ok {
				e.blockedBy = append(e.blockedBy, parent)
			}
		}
		if len(e.blockedBy) == 0 {
			e.state = statusReady
		} else {
			e.state = statusBlocked
		}
	}
	return q
}

func (q *dagQueue) getReady(n int) []*entry {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []*entry
	for _, e := range q.entries {
		if len(out) == n {
			break
		}
		if e.state == statusReady {
			e.state = statusRunning
			out = append(out, e)
		}
	}
	return out
}

func (q *dagQueue) markDone(e *entry, res Result) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if res.Err != nil || res.ExitCode != 0 {
		e.state = statusFailed
	} else {
		e.state = statusSucceeded
	}
	e.result = res

	// propagate
	for _, child := range q.entries {
		if child.state == statusBlocked {
			allDone := true
			for _, p := range child.blockedBy {
				if p.state != statusSucceeded {
					allDone = false
					if p.state == statusFailed || p.state == statusAncestorFailed {
						child.state = statusAncestorFailed
						break
					}
				}
			}
			if allDone {
				child.state = statusReady
			}
		}
	}
}

func (q *dagQueue) empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.entries {
		if e.state == statusPending || e.state == statusBlocked || e.state == statusReady || e.state == statusRunning {
			return false
		}
	}
	return true
}

func (q *dagQueue) results() []Result {
	out := make([]Result, 0, len(q.entries))
	for _, e := range q.entries {
		out = append(out, e.result)
	}
	return out
}
