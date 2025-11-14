// Package worktrees provides functionality for creating and managing Git worktrees for operating across multiple
// Git references.
package worktrees

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

// Worktrees is a collection of Git references to associated worktree paths and Git filter expressions to diffs.
//
// It needs to be passed into any functions that need to interact with Git worktrees, like the generate and discovery
// packages.
type Worktrees struct {
	RefsToPaths           map[string]string
	GitExpressionsToDiffs map[*filter.GitExpression]*git.Diffs

	gitRunner *git.GitRunner
}

// Cleanup removes all created Git worktrees and their temporary directories.
func (w *Worktrees) Cleanup(ctx context.Context, l log.Logger) error {
	for _, path := range w.RefsToPaths {
		if err := w.gitRunner.RemoveWorktree(ctx, path); err != nil {
			return fmt.Errorf("failed to remove Git worktree for reference %s: %w", path, err)
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

	for expression, diffs := range w.GitExpressionsToDiffs {
		fromWorktree := w.RefsToPaths[expression.FromRef]
		toWorktree := w.RefsToPaths[expression.ToRef]

		for _, added := range diffs.Added {
			if filepath.Base(added) != config.DefaultStackFile {
				continue
			}

			stackDiff.Added = append(
				stackDiff.Added,
				component.NewStack(filepath.Join(toWorktree, filepath.Dir(added))).WithDiscoveryContext(
					&component.DiscoveryContext{
						WorkingDir: toWorktree,
						Ref:        expression.ToRef,
					},
				),
			)
		}

		for _, removed := range diffs.Removed {
			if filepath.Base(removed) != config.DefaultStackFile {
				continue
			}

			stackDiff.Removed = append(
				stackDiff.Removed,
				component.NewStack(filepath.Join(fromWorktree, filepath.Dir(removed))).WithDiscoveryContext(
					&component.DiscoveryContext{
						WorkingDir: fromWorktree,
						Ref:        expression.FromRef,
					},
				),
			)
		}

		for _, changed := range diffs.Changed {
			if filepath.Base(changed) != config.DefaultStackFile {
				continue
			}

			stackDiff.Changed = append(
				stackDiff.Changed,
				StackDiffChangedPair{
					FromStack: component.NewStack(filepath.Join(fromWorktree, filepath.Dir(changed))).WithDiscoveryContext(
						&component.DiscoveryContext{
							WorkingDir: fromWorktree,
							Ref:        expression.FromRef,
						},
					),
					ToStack: component.NewStack(filepath.Join(toWorktree, filepath.Dir(changed))).WithDiscoveryContext(
						&component.DiscoveryContext{
							WorkingDir: toWorktree,
							Ref:        expression.ToRef,
						},
					),
				},
			)
		}
	}

	return stackDiff
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
			RefsToPaths:           make(map[string]string),
			GitExpressionsToDiffs: make(map[*filter.GitExpression]*git.Diffs),
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

	var refsToPaths map[string]string

	gitRunner, err := git.NewGitRunner()
	if err != nil {
		return nil, fmt.Errorf("failed to create Git runner for worktree creation: %w", err)
	}

	gitRunner = gitRunner.WithWorkDir(workingDir)

	if len(gitRefs) > 0 {
		gitCmdGroup.Go(func() error {
			var err error
			if refsToPaths, err = createGitWorktrees(gitCmdCtx, l, gitRunner, gitRefs); err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()

				return err
			}

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
			RefsToPaths:           refsToPaths,
			GitExpressionsToDiffs: expressionsToDiffs,
			gitRunner:             gitRunner,
		}, err
	}

	if len(errs) > 0 {
		return &Worktrees{
			RefsToPaths:           refsToPaths,
			GitExpressionsToDiffs: expressionsToDiffs,
			gitRunner:             gitRunner,
		}, errors.Join(errs...)
	}

	return &Worktrees{
		RefsToPaths:           refsToPaths,
		GitExpressionsToDiffs: expressionsToDiffs,
		gitRunner:             gitRunner,
	}, nil
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

				errs = append(errs, fmt.Errorf("failed to create temporary directory for worktree: %w", err))

				mu.Unlock()

				return nil
			}

			// macOS will create the temporary directory with symlinks, so we need to evaluate them.
			tmpDir, err = filepath.EvalSymlinks(tmpDir)
			if err != nil {
				mu.Lock()

				errs = append(errs, fmt.Errorf("failed to evaluate symlinks for temporary directory: %w", err))

				mu.Unlock()

				return nil
			}

			err = gitRunner.CreateDetachedWorktree(ctx, tmpDir, ref)
			if err != nil {
				mu.Lock()

				errs = append(errs, fmt.Errorf("failed to create Git worktree for reference %s: %w", ref, err))

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
		return refsToPaths, fmt.Errorf("failed to create Git worktrees: %w", err)
	}

	if len(errs) > 0 {
		return refsToPaths, errors.Join(errs...)
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
