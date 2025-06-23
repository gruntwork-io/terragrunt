package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// Task wraps a runbase.Unit so we can attach execution‑time helpers while
// leaving the underlying model untouched.
type Task struct {
	Unit *runbase.Unit
}

// ID returns a stable identifier (we use the absolute path to the module).
func (t *Task) ID() string { return t.Unit.Path }

// Parents returns the IDs (absolute paths) of upstream units that this task
// depends on. It translates each dependency struct into its Path string.
func (t *Task) Parents() []string {
	parents := make([]string, 0, len(t.Unit.Dependencies))
	for _, dep := range t.Unit.Dependencies {
		parents = append(parents, dep.Path)
	}
	return parents
}

// Result captures the outcome of running one Task.
type Result struct {
	TaskID   string
	ExitCode int
	Err      error
}

// TaskRunner executes a single Task.
// Implementations should honour ctx cancellation.
type TaskRunner func(ctx context.Context, t *Task) Result
