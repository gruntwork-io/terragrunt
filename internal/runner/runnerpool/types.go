package runnerpool

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// Task wraps a runbase.Unit so we can attach execution‑time helpers while
// leaving the underlying model untouched./gruntwork-io/terragrunt/internal/runbase"

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

type Result struct {
	TaskID   string
	ExitCode int
	Err      error
}

type TaskRunner func(ctx context.Context, t *Task) Result
