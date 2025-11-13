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

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

// Worktrees is a relatively expensive to construct object.
//
// It's a mapping of both Git references to worktree paths and Git filter expressions to diffs.
// It's only constructed once, and is then re-used thereafter.
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

// NewWorktrees creates a new Worktrees for a given set of Git filters.
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
