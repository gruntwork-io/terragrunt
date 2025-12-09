// Package worktrees provides functionality for creating and managing Git worktrees for operating across multiple
// Git references.
package worktrees

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

// Worktrees is a map of WorktreePairs, and the Git runner used to create and manage the worktrees.
// The key is the string representation of the GitExpression that generated the worktree pair.
type Worktrees struct {
	WorktreePairs      map[string]WorktreePair
	gitRunner          *git.GitRunner
	OriginalWorkingDir string
}

// WorktreePair is a pair of worktrees, one for the from and one for the to reference, along with
// the GitExpression that generated the diffs and the diff for that expression.
type WorktreePair struct {
	GitExpression *filter.GitExpression
	Diffs         *git.Diffs
	FromWorktree  Worktree
	ToWorktree    Worktree
}

// Worktree is collects a Git reference and the path to the associated worktree.
type Worktree struct {
	Ref  string
	Path string
}

// WorkingDir returns the path within a worktree that corresponds to the user's
// original working directory. This is used for display purposes after discovery completes.
func (w *Worktrees) WorkingDir(ctx context.Context, worktreePath string) string {
	if w.gitRunner == nil {
		return worktreePath
	}

	repoRoot, err := w.gitRunner.GetRepoRoot(ctx)
	if err != nil {
		return worktreePath
	}

	relPath, err := filepath.Rel(repoRoot, w.OriginalWorkingDir)
	if err != nil || relPath == "." {
		return worktreePath
	}

	return filepath.Join(worktreePath, relPath)
}

// DisplayPath translates a worktree path to the equivalent path in the original repository
// for user-facing output. This is useful for logging and reporting where users expect to see
// paths relative to their working directory, not temporary worktree paths.
// If the path is not within a worktree, it returns the path unchanged.
func (w *Worktrees) DisplayPath(worktreePath string) string {
	for _, pair := range w.WorktreePairs {
		for _, wt := range []Worktree{pair.FromWorktree, pair.ToWorktree} {
			// Use boundary-aware check to avoid false matches (e.g., "/tmp/work" vs "/tmp/work-other")
			if worktreePath == wt.Path || strings.HasPrefix(worktreePath, wt.Path+string(os.PathSeparator)) {
				// Get the relative path within the worktree
				relPath, err := filepath.Rel(wt.Path, worktreePath)
				if err != nil {
					return worktreePath
				}

				// Join with original working dir
				return filepath.Join(w.OriginalWorkingDir, relPath)
			}
		}
	}

	return worktreePath
}

// Cleanup removes all created Git worktrees and their temporary directories.
func (w *Worktrees) Cleanup(ctx context.Context, l log.Logger) error {
	seen := make(map[string]struct{})

	for _, pair := range w.WorktreePairs {
		for _, worktree := range []Worktree{pair.FromWorktree, pair.ToWorktree} {
			if _, ok := seen[worktree.Path]; ok {
				continue
			}

			seen[worktree.Path] = struct{}{}

			// Skip removal if the worktree path doesn't exist (may have been cleaned up already)
			if _, err := os.Stat(worktree.Path); os.IsNotExist(err) {
				l.Debugf("Worktree path %s already removed, skipping cleanup", worktree.Path)

				continue
			}

			if err := w.gitRunner.RemoveWorktree(ctx, worktree.Path); err != nil {
				// If the error is due to the worktree not existing, log and continue
				// This can happen during parallel test execution or if cleanup runs twice
				errStr := err.Error()
				if strings.Contains(errStr, "No such file or directory") ||
					strings.Contains(errStr, "does not exist") ||
					strings.Contains(errStr, "not a valid directory") {
					l.Debugf("Worktree for reference %s already cleaned up: %v", worktree.Ref, err)

					continue
				}

				return tgerrors.Errorf(
					"failed to remove Git worktree for reference %s (%s): %w",
					worktree.Ref,
					worktree.Path,
					err,
				)
			}
		}
	}

	return nil
}

type StackDiff struct {
	Added   []*component.Stack
	Removed []*component.Stack
	Changed []StackDiffChangedPair
}

type StackDiffChangedPair struct {
	FromStack *component.Stack
	ToStack   *component.Stack
}

// Stacks returns a slice of stacks that can be found in the diffs found in worktrees.
//
// This can be useful, as stacks need to be discovered in worktrees, generated, then diffed on-disk
// to find changed units.
//
// They are returned as added, removed, and changed stacks, respectively.
func (w *Worktrees) Stacks() StackDiff {
	stackDiff := StackDiff{
		Added:   []*component.Stack{},
		Removed: []*component.Stack{},
		Changed: []StackDiffChangedPair{},
	}

	for _, pair := range w.WorktreePairs {
		fromWorktree := pair.FromWorktree.Path
		toWorktree := pair.ToWorktree.Path

		for _, added := range pair.Diffs.Added {
			if filepath.Base(added) != config.DefaultStackFile {
				continue
			}

			stackDiff.Added = append(
				stackDiff.Added,
				component.NewStack(filepath.Join(toWorktree, filepath.Dir(added))).WithDiscoveryContext(
					&component.DiscoveryContext{
						WorkingDir: toWorktree,
						Ref:        pair.ToWorktree.Ref,
					},
				),
			)
		}

		for _, removed := range pair.Diffs.Removed {
			if filepath.Base(removed) != config.DefaultStackFile {
				continue
			}

			stackDiff.Removed = append(
				stackDiff.Removed,
				component.NewStack(filepath.Join(fromWorktree, filepath.Dir(removed))).WithDiscoveryContext(
					&component.DiscoveryContext{
						WorkingDir: fromWorktree,
						Ref:        pair.FromWorktree.Ref,
					},
				),
			)
		}

		for _, changed := range pair.Diffs.Changed {
			if filepath.Base(changed) != config.DefaultStackFile {
				continue
			}

			stackDiff.Changed = append(
				stackDiff.Changed,
				StackDiffChangedPair{
					FromStack: component.NewStack(filepath.Join(fromWorktree, filepath.Dir(changed))).WithDiscoveryContext(
						&component.DiscoveryContext{
							WorkingDir: fromWorktree,
							Ref:        pair.FromWorktree.Ref,
						},
					),
					ToStack: component.NewStack(filepath.Join(toWorktree, filepath.Dir(changed))).WithDiscoveryContext(
						&component.DiscoveryContext{
							WorkingDir: toWorktree,
							Ref:        pair.ToWorktree.Ref,
						},
					),
				},
			)
		}
	}

	return stackDiff
}

// Expand expands a worktree pair with an associated Git expression into the equivalent to and from filter
// expressions based on the provided diffs for the worktree pair.
func (wp *WorktreePair) Expand() (filter.Filters, filter.Filters) {
	diffs := wp.Diffs

	toPath := wp.ToWorktree.Path

	fromExpressions := make(filter.Expressions, 0, len(diffs.Removed))
	toExpressions := make(filter.Expressions, 0, len(diffs.Added)+len(diffs.Changed))

	// Build simple expressions that can be determined simply from the diffs.
	for _, path := range diffs.Removed {
		dir := filepath.Dir(path)

		switch filepath.Base(path) {
		case config.DefaultTerragruntConfigPath:
			fromExpressions = append(fromExpressions, filter.NewPathFilter(dir))
		case config.DefaultStackFile:
			fromExpressions = append(
				fromExpressions,
				filter.NewPathFilter(dir),
				filter.NewPathFilter(filepath.Join(dir, "**")),
			)
		default:
			// Check to see if the removed file is in the same directory as a unit in the to worktree.
			// If so, we'll consider the unit modified.
			if _, err := os.Stat(filepath.Join(toPath, dir, config.DefaultTerragruntConfigPath)); err == nil {
				toExpressions = append(toExpressions, filter.NewPathFilter(dir))
			}
		}
	}

	for _, path := range diffs.Added {
		dir := filepath.Dir(path)

		switch filepath.Base(path) {
		case config.DefaultTerragruntConfigPath:
			toExpressions = append(toExpressions, filter.NewPathFilter(dir))
		case config.DefaultStackFile:
			toExpressions = append(
				toExpressions,
				filter.NewPathFilter(dir),
				filter.NewPathFilter(filepath.Join(dir, "**")),
			)
		default:
			// Check to see if the added file is in the same directory as a unit in the to worktree.
			// If so, we'll consider the unit modified.
			if _, err := os.Stat(filepath.Join(toPath, dir, config.DefaultTerragruntConfigPath)); err == nil {
				toExpressions = append(toExpressions, filter.NewPathFilter(dir))
			}
		}
	}

	for _, path := range diffs.Changed {
		dir := filepath.Dir(path)

		switch filepath.Base(path) {
		case config.DefaultTerragruntConfigPath:
			toExpressions = append(toExpressions, filter.NewPathFilter(dir))
		case config.DefaultStackFile:
			// We handle changed stack files elsewhere, as we need to handle walking the filesystem to assess diffs.
		default:
			// Check to see if the changed file is in the same directory as a unit in the to worktree.
			// If so, we'll consider the unit modified.
			if _, err := os.Stat(filepath.Join(toPath, dir, config.DefaultTerragruntConfigPath)); err == nil {
				toExpressions = append(toExpressions, filter.NewPathFilter(dir))

				continue
			}

			// Otherwise, we'll consider it a file that could potentially be read by other units, and needs to be
			// tracked using a reading filter.
			toExpressions = append(toExpressions, filter.NewAttributeExpression(filter.AttributeReading, path))
		}
	}

	fromFilters := make(filter.Filters, 0, len(fromExpressions))
	for _, expression := range fromExpressions {
		fromFilters = append(
			fromFilters,
			filter.NewFilter(expression, expression.String()),
		)
	}

	toFilters := make(filter.Filters, 0, len(toExpressions))
	for _, expression := range toExpressions {
		toFilters = append(
			toFilters,
			filter.NewFilter(expression, expression.String()),
		)
	}

	return fromFilters, toFilters
}

// NewWorktrees creates a new Worktrees for a given set of Git filters.
//
// Note that it is the responsibility of the caller to call Cleanup on the Worktrees object when it is no longer needed.
func NewWorktrees(
	ctx context.Context,
	l log.Logger,
	workingDir string,
	gitExpressions filter.GitExpressions,
) (*Worktrees, error) {
	if len(gitExpressions) == 0 {
		return &Worktrees{
			WorktreePairs:      make(map[string]WorktreePair),
			OriginalWorkingDir: workingDir,
		}, nil
	}

	gitRefs := gitExpressions.UniqueGitRefs()

	var (
		errs []error
		mu   sync.Mutex
	)

	expressionsToDiffs := make(map[*filter.GitExpression]*git.Diffs, len(gitExpressions))

	gitCmdGroup, gitCmdCtx := errgroup.WithContext(ctx)
	gitCmdGroup.SetLimit(min(runtime.NumCPU(), len(gitRefs)))

	refsToPaths := make(map[string]string, len(gitRefs))

	gitRunner, err := git.NewGitRunner()
	if err != nil {
		return nil, tgerrors.Errorf("failed to create Git runner for worktree creation: %w", err)
	}

	gitRunner = gitRunner.WithWorkDir(workingDir)

	if len(gitRefs) > 0 {
		gitCmdGroup.Go(func() error {
			paths, err := createGitWorktrees(gitCmdCtx, l, gitRunner, gitRefs)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()

				return err
			}

			mu.Lock()

			maps.Copy(refsToPaths, paths)

			mu.Unlock()

			return nil
		})
	}

	for _, gitExpression := range gitExpressions {
		gitCmdGroup.Go(func() error {
			diffs, err := gitRunner.Diff(gitCmdCtx, gitExpression.FromRef, gitExpression.ToRef)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()

				return nil
			}

			mu.Lock()

			expressionsToDiffs[gitExpression] = diffs

			mu.Unlock()

			return nil
		})
	}

	if err := gitCmdGroup.Wait(); err != nil {
		return &Worktrees{
			WorktreePairs:      make(map[string]WorktreePair),
			OriginalWorkingDir: workingDir,
			gitRunner:          gitRunner,
		}, err
	}

	worktreePairs := make(map[string]WorktreePair, len(gitExpressions))
	for _, gitExpression := range gitExpressions {
		worktreePairs[gitExpression.String()] = WorktreePair{
			GitExpression: gitExpression,
			Diffs:         expressionsToDiffs[gitExpression],
			FromWorktree:  Worktree{Ref: gitExpression.FromRef, Path: refsToPaths[gitExpression.FromRef]},
			ToWorktree:    Worktree{Ref: gitExpression.ToRef, Path: refsToPaths[gitExpression.ToRef]},
		}
	}

	worktrees := &Worktrees{
		WorktreePairs:      worktreePairs,
		OriginalWorkingDir: workingDir,
		gitRunner:          gitRunner,
	}

	if len(errs) > 0 {
		return worktrees, tgerrors.Join(errs...)
	}

	return worktrees, nil
}

// createGitWorktrees creates detached worktrees for each unique Git reference needed by filters.
// The worktrees are created in temporary directories and tracked in refsToPaths.
func createGitWorktrees(
	ctx context.Context,
	l log.Logger,
	gitRunner *git.GitRunner,
	gitRefs []string,
) (map[string]string, error) {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), len(gitRefs)))

	var (
		errs []error
		mu   sync.Mutex
	)

	refsToPaths := make(map[string]string, len(gitRefs))

	for _, ref := range gitRefs {
		g.Go(func() error {
			tmpDir, err := os.MkdirTemp("", "terragrunt-worktree-"+sanitizeRef(ref)+"-*")
			if err != nil {
				mu.Lock()

				errs = append(errs, tgerrors.Errorf("failed to create temporary directory for worktree: %w", err))

				mu.Unlock()

				return nil
			}

			// macOS will create the temporary directory with symlinks, so we need to evaluate them.
			tmpDir, err = filepath.EvalSymlinks(tmpDir)
			if err != nil {
				mu.Lock()

				errs = append(errs, tgerrors.Errorf("failed to evaluate symlinks for temporary directory: %w", err))

				mu.Unlock()

				return nil
			}

			err = gitRunner.CreateDetachedWorktree(ctx, tmpDir, ref)
			if err != nil {
				mu.Lock()

				errs = append(errs, tgerrors.Errorf("failed to create Git worktree for reference %s: %w", ref, err))

				mu.Unlock()

				return nil
			}

			mu.Lock()

			refsToPaths[ref] = tmpDir

			mu.Unlock()

			l.Debugf("Created Git worktree for reference %s at %s", ref, tmpDir)

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return refsToPaths, tgerrors.Errorf("failed to create Git worktrees: %w", err)
	}

	if len(errs) > 0 {
		return refsToPaths, tgerrors.Join(errs...)
	}

	return refsToPaths, nil
}

// sanitizeRef sanitizes a Git reference string for use in file paths.
// It replaces invalid characters with underscores.
func sanitizeRef(ref string) string {
	result := strings.Builder{}

	for _, r := range ref {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)

			continue
		}

		result.WriteRune('_')
	}

	return result.String()
}
