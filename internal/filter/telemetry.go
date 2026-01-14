// Package filter provides telemetry support for git worktree operations and filter evaluation.
package filter

import (
	"context"

	"github.com/gruntwork-io/terragrunt/telemetry"
)

// Telemetry operation names for git worktree and filter operations.
const (
	// Git worktree operations
	TelemetryOpGitWorktreeCreate      = "git_worktree_create"
	TelemetryOpGitWorktreeRemove      = "git_worktree_remove"
	TelemetryOpGitWorktreesCreate     = "git_worktrees_create"
	TelemetryOpGitWorktreesCleanup    = "git_worktrees_cleanup"
	TelemetryOpGitDiff                = "git_diff"
	TelemetryOpGitWorktreeDiscovery   = "git_worktree_discovery"
	TelemetryOpGitWorktreeStackWalk   = "git_worktree_stack_walk"
	TelemetryOpGitWorktreeFilterApply = "git_worktree_filter_apply"

	// Filter evaluation operations
	TelemetryOpFilterEvaluate      = "filter_evaluate"
	TelemetryOpFilterParse         = "filter_parse"
	TelemetryOpGitFilterExpand     = "git_filter_expand"
	TelemetryOpGitFilterEvaluate   = "git_filter_evaluate"
	TelemetryOpGraphFilterTraverse = "graph_filter_traverse"
)

// Telemetry attribute keys for git worktree operations.
const (
	AttrGitRef         = "git.ref"
	AttrGitFromRef     = "git.from_ref"
	AttrGitToRef       = "git.to_ref"
	AttrGitWorktreeDir = "git.worktree_dir"
	AttrGitWorkingDir  = "git.working_dir"
	AttrGitRefCount    = "git.ref_count"
	AttrGitDiffAdded   = "git.diff.added_count"
	AttrGitDiffRemoved = "git.diff.removed_count"
	AttrGitDiffChanged = "git.diff.changed_count"

	// Repository identification attributes
	AttrGitRepoRemote = "git.repo.remote"
	AttrGitRepoBranch = "git.repo.branch"
	AttrGitRepoCommit = "git.repo.commit"

	AttrFilterQuery       = "filter.query"
	AttrFilterType        = "filter.type"
	AttrFilterCount       = "filter.count"
	AttrComponentCount    = "component.count"
	AttrResultCount       = "result.count"
	AttrWorktreePairCount = "worktree.pair_count"
)

// TraceGitWorktreeCreate wraps a git worktree create operation with telemetry.
// The underlying Telemeter.Collect handles nil/unconfigured telemetry gracefully.
func TraceGitWorktreeCreate(ctx context.Context, ref, worktreeDir, repoRemote, repoBranch, repoCommit string, fn func(ctx context.Context) error) error {
	attrs := map[string]any{
		AttrGitRef:         ref,
		AttrGitWorktreeDir: worktreeDir,
	}
	if repoRemote != "" {
		attrs[AttrGitRepoRemote] = repoRemote
	}

	if repoBranch != "" {
		attrs[AttrGitRepoBranch] = repoBranch
	}

	if repoCommit != "" {
		attrs[AttrGitRepoCommit] = repoCommit
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitWorktreeCreate, attrs, fn)
}

// TraceGitWorktreeRemove wraps a git worktree remove operation with telemetry.
func TraceGitWorktreeRemove(ctx context.Context, ref, worktreeDir string, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitWorktreeRemove, map[string]any{
		AttrGitRef:         ref,
		AttrGitWorktreeDir: worktreeDir,
	}, fn)
}

// TraceGitWorktreesCreate wraps multiple git worktree create operations with telemetry.
func TraceGitWorktreesCreate(ctx context.Context, workingDir string, refCount int, repoRemote, repoBranch, repoCommit string, fn func(ctx context.Context) error) error {
	attrs := map[string]any{
		AttrGitWorkingDir: workingDir,
		AttrGitRefCount:   refCount,
	}
	if repoRemote != "" {
		attrs[AttrGitRepoRemote] = repoRemote
	}

	if repoBranch != "" {
		attrs[AttrGitRepoBranch] = repoBranch
	}

	if repoCommit != "" {
		attrs[AttrGitRepoCommit] = repoCommit
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitWorktreesCreate, attrs, fn)
}

// TraceGitWorktreesCleanup wraps git worktrees cleanup with telemetry.
func TraceGitWorktreesCleanup(ctx context.Context, pairCount int, repoRemote string, fn func(ctx context.Context) error) error {
	attrs := map[string]any{
		AttrWorktreePairCount: pairCount,
	}
	if repoRemote != "" {
		attrs[AttrGitRepoRemote] = repoRemote
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitWorktreesCleanup, attrs, fn)
}

// TraceGitDiff wraps a git diff operation with telemetry.
func TraceGitDiff(ctx context.Context, fromRef, toRef, repoRemote string, fn func(ctx context.Context) error) error {
	attrs := map[string]any{
		AttrGitFromRef: fromRef,
		AttrGitToRef:   toRef,
	}
	if repoRemote != "" {
		attrs[AttrGitRepoRemote] = repoRemote
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitDiff, attrs, fn)
}

// TraceGitWorktreeDiscovery wraps git worktree discovery operations with telemetry.
func TraceGitWorktreeDiscovery(ctx context.Context, pairCount int, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitWorktreeDiscovery, map[string]any{
		AttrWorktreePairCount: pairCount,
	}, fn)
}

// TraceGitWorktreeStackWalk wraps git worktree stack walking operations with telemetry.
func TraceGitWorktreeStackWalk(ctx context.Context, fromRef, toRef string, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitWorktreeStackWalk, map[string]any{
		AttrGitFromRef: fromRef,
		AttrGitToRef:   toRef,
	}, fn)
}

// TraceGitWorktreeFilterApply wraps filter application to git worktrees with telemetry.
func TraceGitWorktreeFilterApply(ctx context.Context, filterCount, resultCount int, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitWorktreeFilterApply, map[string]any{
		AttrFilterCount: filterCount,
		AttrResultCount: resultCount,
	}, fn)
}

// TraceFilterEvaluate wraps filter evaluation with telemetry.
func TraceFilterEvaluate(ctx context.Context, filterCount, componentCount int, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpFilterEvaluate, map[string]any{
		AttrFilterCount:    filterCount,
		AttrComponentCount: componentCount,
	}, fn)
}

// TraceFilterParse wraps filter parsing with telemetry.
func TraceFilterParse(ctx context.Context, query string, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpFilterParse, map[string]any{
		AttrFilterQuery: query,
	}, fn)
}

// TraceGitFilterExpand wraps git filter expansion with telemetry.
func TraceGitFilterExpand(ctx context.Context, fromRef, toRef string, addedCount, removedCount, changedCount int, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitFilterExpand, map[string]any{
		AttrGitFromRef:     fromRef,
		AttrGitToRef:       toRef,
		AttrGitDiffAdded:   addedCount,
		AttrGitDiffRemoved: removedCount,
		AttrGitDiffChanged: changedCount,
	}, fn)
}

// TraceGitFilterEvaluate wraps git filter evaluation with telemetry.
func TraceGitFilterEvaluate(ctx context.Context, fromRef, toRef string, componentCount int, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGitFilterEvaluate, map[string]any{
		AttrGitFromRef:     fromRef,
		AttrGitToRef:       toRef,
		AttrComponentCount: componentCount,
	}, fn)
}

// TraceGraphFilterTraverse wraps graph filter traversal with telemetry.
func TraceGraphFilterTraverse(ctx context.Context, filterType string, componentCount int, fn func(ctx context.Context) error) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpGraphFilterTraverse, map[string]any{
		AttrFilterType:     filterType,
		AttrComponentCount: componentCount,
	}, fn)
}
