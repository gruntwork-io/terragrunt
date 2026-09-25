package runner

import "github.com/gruntwork-io/terragrunt/internal/worktrees"

// Option carries a setting from a command into discovery and execution. Each
// consumer type-asserts for the accessor it reads, so this interface exists only
// to keep unrelated values out.
type Option interface {
	runnerOption()
}

// WorktreeOption carries worktrees through the runner pipeline for git filter expressions.
type WorktreeOption struct {
	Worktrees *worktrees.Worktrees
}

func (o WorktreeOption) runnerOption() {}

// WithWorktrees provides git worktrees to discovery for git filter expressions.
func WithWorktrees(w *worktrees.Worktrees) Option {
	return WorktreeOption{Worktrees: w}
}

// graphTargetOption marks a graph target that discovery uses to prune the run to
// the target path and its dependents.
type graphTargetOption struct {
	target string
}

func (o graphTargetOption) runnerOption() {}

// GraphTarget exposes the requested graph target for discovery to consume.
func (o graphTargetOption) GraphTarget() string {
	return o.target
}

// WithGraphTarget limits the run to the target path and its dependents.
func WithGraphTarget(targetDir string) Option {
	return graphTargetOption{target: targetDir}
}
