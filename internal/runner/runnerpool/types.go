package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// TaskRunner defines a function type that executes a Task within a given context and returns a Result.
type TaskRunner func(ctx context.Context, u *runbase.Unit) (int, error)
