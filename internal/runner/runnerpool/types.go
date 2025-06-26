package runnerpool

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// Task wraps a runbase.Unit so we can attach execution‑time helpers while
// leaving the underlying model untouched.
type Task struct {
	Unit *runbase.Unit
}

// ID canonical ID of the module: clean( Unit.Path ).
func (t *Task) ID() string { return filepath.Clean(t.Unit.Path) }

// Parents returns canonical IDs for all upstream dependencies.
func (t *Task) Parents() []string {
	ids := make([]string, 0, len(t.Unit.Dependencies))
	for _, dep := range t.Unit.Dependencies {
		ids = append(ids, filepath.Clean(dep.Path))
	}

	return ids
}

// helper to construct a Task from a Unit.
func taskFromUnit(u *runbase.Unit) *Task { return &Task{Unit: u} }

// Result represents the outcome of executing a Task, including any error, the Task's ID, and the exit code.
type Result struct {
	Err      error
	TaskID   string
	ExitCode int
}

// TaskRunner defines a function type that executes a Task within a given context and returns a Result.
type TaskRunner func(ctx context.Context, t *Task) Result

// Status represents the lifecycle State of a Task
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

// Entry is an internal struct used to track the execution State of a Task, its dependencies, and Dependents within the runner pool.
type Entry struct {
	Result        Result
	Task          *Task
	Dependents    []*Entry
	State         Status
	RemainingDeps int
}
