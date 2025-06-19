package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// Task is a lightweight wrapper around runbase.Unit that the runner‑pool
// schedules. The underlying Path field is treated as the stable ID; we expose
// helpers so the rest of this package never needs to know runbase details.
type Task struct {
	Unit *runbase.Unit
}

func (t *Task) ID() string { return t.Unit.Path }
func (t *Task) Parents() []string {
	parents := make([]string, 0, len(t.Unit.Dependencies))
	for _, dep := range t.Unit.Dependencies {
		parents = append(parents, dep.Path)
	}
	return parents
}

// Result captures the outcome of running a Task.
type Result struct {
	TaskID   string
	ExitCode int
	Err      error
}

// TaskRunner is a function that executes a single Task.
type TaskRunner func(ctx context.Context, t *Task) Result

type Status int

const (
	StatusPending Status = iota
	StatusBlocked
	StatusReady
	StatusRunning
	StatusSucceeded
	StatusFailed
	StatusAncestorFailed
	StatusFailFast
)

// entry ties one immutable Task to its mutable runtime state.
type entry struct {
	task      *Task
	status    Status
	blockedBy []*entry
	result    Result
}
