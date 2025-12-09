package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

// WorktreeDiscovery is the configuration for discovery in Git worktrees.
type WorktreeDiscovery struct {
	// originalDiscovery is the original discovery object that triggered the worktree discovery.
	originalDiscovery *Discovery

	// gitExpressions contains Git filter expressions that require worktree discovery
	gitExpressions filter.GitExpressions

	// numWorkers is the number of workers to use to discover in worktrees.
	numWorkers int
}

// NewWorktreeDiscovery creates a new WorktreeDiscovery with the given configuration.
func NewWorktreeDiscovery(gitExpressions filter.GitExpressions) *WorktreeDiscovery {
	return &WorktreeDiscovery{
		gitExpressions: gitExpressions,
		numWorkers:     runtime.NumCPU(),
	}
}

// WithNumWorkers sets the number of workers for worktree discovery.
func (wd *WorktreeDiscovery) WithNumWorkers(numWorkers int) *WorktreeDiscovery {
	wd.numWorkers = numWorkers
	return wd
}

// WithOriginalDiscovery sets the original discovery object that triggered the worktree discovery.
func (wd *WorktreeDiscovery) WithOriginalDiscovery(originalDiscovery *Discovery) *WorktreeDiscovery {
	wd.originalDiscovery = originalDiscovery
	return wd
}

// translateComponentPath translates a component's path from a worktree temp path to the original working directory.
// This is necessary because components are discovered in worktree temp directories, but should be resolved
// in the original working directory for the runner to find terraform files.
//
// When running from a subdirectory, we need to account for the offset between the git root and the
// working directory. The worktree represents the git repo root, so we compute the equivalent path
// in the worktree for the original working directory and use that as the base for relative path computation.
func translateComponentPath(c component.Component, worktreePath, originalWorkingDir, gitRootOffset string) {
	if originalWorkingDir == "" {
		return
	}

	componentPath := c.Path()

	// Compute the equivalent worktree path for the original working directory.
	// This handles the case where the user is running from a subdirectory of the repo.
	// gitRootOffset is the relative path from git root to originalWorkingDir.
	equivalentWorktreePath := worktreePath
	if gitRootOffset != "" {
		equivalentWorktreePath = filepath.Join(worktreePath, gitRootOffset)
	}

	// Compute relative path from the equivalent worktree path to the component
	relativePath, err := filepath.Rel(equivalentWorktreePath, componentPath)
	if err != nil {
		// If we can't compute relative path, the component is not under the equivalent path
		return
	}

	// Check if the path is outside the equivalent worktree path (starts with "..")
	if strings.HasPrefix(relativePath, "..") {
		return
	}

	newPath := filepath.Join(originalWorkingDir, relativePath)
	c.SetPath(newPath)

	// Also update the discovery context's working directory
	discoveryCtx := c.DiscoveryContext()
	if discoveryCtx != nil {
		newDiscoveryCtx := *discoveryCtx
		newDiscoveryCtx.WorkingDir = originalWorkingDir
		c.SetDiscoveryContext(&newDiscoveryCtx)
	}
}

// Discover discovers components in all worktrees.
func (wd *WorktreeDiscovery) Discover(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	w *worktrees.Worktrees,
) (component.Components, error) {
	if w == nil {
		l.Debug("No worktrees provided, skipping worktree discovery")

		return component.Components{}, nil
	}

	originalWorkingDir := w.OriginalWorkingDir
	gitRootOffset := w.GitRootOffset(ctx)
	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	// Run from and to discovery concurrently for each expression
	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(wd.numWorkers)

	for _, pair := range w.WorktreePairs {
		discoveryGroup.Go(func() error {
			fromFilters, toFilters := pair.Expand()

			// Run from and to discovery concurrently
			fromToG, fromToCtx := errgroup.WithContext(discoveryCtx)

			// We only kick off from/to discovery if there any filters expanded from the git expression.
			// This ensures that we don't discover anything when using a Git filter that doesn't match anything.

			if len(fromFilters) > 0 {
				fromToG.Go(func() error {
					fromDiscovery := *wd.originalDiscovery

					fromDiscoveryContext := *fromDiscovery.discoveryContext
					fromDiscoveryContext.Ref = pair.FromWorktree.Ref
					fromDiscoveryContext.WorkingDir = pair.FromWorktree.Path

					fromDiscoveryContext, err := translateDiscoveryContextArgsForWorktree(
						fromDiscoveryContext,
						fromWorktreeKind,
					)
					if err != nil {
						return err
					}

					components, err := fromDiscovery.
						WithFilters(fromFilters).
						WithDiscoveryContext(&fromDiscoveryContext).
						Discover(fromToCtx, l, opts)
					if err != nil {
						return err
					}

					// Do NOT translate fromWorktree paths - removed units only exist in the worktree
					// They need to run terraform destroy from the worktree where they still exist
					// Filter to only include components under the equivalent subdirectory
					equivalentPath := pathInWorktree(pair.FromWorktree.Path, gitRootOffset)
					for _, c := range components {
						if strings.HasPrefix(c.Path(), equivalentPath) {
							discoveredComponents.EnsureComponent(c)
						}
					}

					return nil
				})
			}

			if len(toFilters) > 0 {
				fromToG.Go(func() error {
					toDiscovery := *wd.originalDiscovery

					toDiscoveryContext := *toDiscovery.discoveryContext
					toDiscoveryContext.Ref = pair.ToWorktree.Ref
					toDiscoveryContext.WorkingDir = pair.ToWorktree.Path

					toDiscoveryContext, err := translateDiscoveryContextArgsForWorktree(
						toDiscoveryContext,
						toWorktreeKind,
					)
					if err != nil {
						return err
					}

					toComponents, err := toDiscovery.
						WithFilters(toFilters).
						WithDiscoveryContext(&toDiscoveryContext).
						Discover(fromToCtx, l, opts)
					if err != nil {
						return err
					}

					// Translate component paths from worktree to original working dir
					// Filter to only include components under the equivalent subdirectory
					equivalentPath := pathInWorktree(pair.ToWorktree.Path, gitRootOffset)
					for _, c := range toComponents {
						if strings.HasPrefix(c.Path(), equivalentPath) {
							translateComponentPath(c, pair.ToWorktree.Path, originalWorkingDir, gitRootOffset)
							discoveredComponents.EnsureComponent(c)
						}
					}

					return nil
				})
			}

			return fromToG.Wait()
		})
	}

	discoveryGroup.Go(func() error {
		components, err := wd.discoverChangesInWorktreeStacks(ctx, l, opts, w)
		if err != nil {
			return err
		}

		// Only translate toWorktree paths - fromWorktree paths should stay as worktree paths
		// because removed units only exist in the worktree
		for _, c := range components {
			for _, pair := range w.WorktreePairs {
				translateComponentPath(c, pair.ToWorktree.Path, originalWorkingDir, gitRootOffset)
			}

			discoveredComponents.EnsureComponent(c)
		}

		return nil
	})

	if err := discoveryGroup.Wait(); err != nil {
		return nil, err
	}

	return discoveredComponents.ToComponents(), nil
}

// discoverChangesInWorktreeStacks discovers changes in worktree stacks.
//
// Stacks are only stored in Git as individual files, so we need to walk them on the filesystem to find any changes
// to the units they contain.
func (wd *WorktreeDiscovery) discoverChangesInWorktreeStacks(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	worktree *worktrees.Worktrees,
) (component.Components, error) {
	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	stackDiff := worktree.Stacks()

	w, ctx := errgroup.WithContext(ctx)
	w.SetLimit(min(runtime.NumCPU(), len(stackDiff.Added)+len(stackDiff.Removed)+len(stackDiff.Changed)*2))

	var (
		mu   sync.Mutex
		errs []error
	)

	// We append two changed stacks whenever we change either, one for the from stack and one for the to stack.
	for _, changed := range stackDiff.Changed {
		w.Go(func() error {
			components, err := wd.walkChangedStack(
				ctx, l, opts, wd.originalDiscovery,
				changed.FromStack,
				changed.ToStack,
			)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()

				return err
			}

			for _, component := range components {
				discoveredComponents.EnsureComponent(component)
			}

			return nil
		})
	}

	if err := w.Wait(); err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return discoveredComponents.ToComponents(), nil
}

type componentPair struct {
	FromComponent component.Component
	ToComponent   component.Component
}

// walkChangedStack walks a changed stack and discovers components within it.
//
// We need to do some diffing for situations where a stack is being changed, we can just include
// all the components within that stack, with the assumption that all the units within it are changed.
func (wd *WorktreeDiscovery) walkChangedStack(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	originalDiscovery *Discovery,
	fromStack *component.Stack,
	toStack *component.Stack,
) (component.Components, error) {
	fromDiscovery := *originalDiscovery

	fromDiscoveryContext := *fromDiscovery.discoveryContext

	fromDiscoveryContext.WorkingDir = fromStack.Path()
	fromDiscoveryContext.Ref = fromStack.DiscoveryContext().Ref

	fromDiscoveryContext, err := translateDiscoveryContextArgsForWorktree(
		fromDiscoveryContext,
		fromWorktreeKind,
	)
	if err != nil {
		return nil, err
	}

	toDiscovery := *originalDiscovery

	toDiscoveryContext := *toDiscovery.discoveryContext

	toDiscoveryContext.WorkingDir = toStack.Path()
	toDiscoveryContext.Ref = toStack.DiscoveryContext().Ref

	toDiscoveryContext, err = translateDiscoveryContextArgsForWorktree(
		toDiscoveryContext,
		toWorktreeKind,
	)
	if err != nil {
		return nil, err
	}

	var fromComponents, toComponents component.Components

	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(min(runtime.NumCPU(), 2)) //nolint:mnd

	var (
		mu   sync.Mutex
		errs []error
	)

	discoveryGroup.Go(func() error {
		var fromDiscoveryErr error

		fromComponents, fromDiscoveryErr = fromDiscovery.
			WithDiscoveryContext(&fromDiscoveryContext).
			WithFilters(
				filter.Filters{},
			).
			Discover(discoveryCtx, l, opts)
		if fromDiscoveryErr != nil {
			mu.Lock()

			errs = append(errs, fromDiscoveryErr)

			mu.Unlock()

			return nil
		}

		// Reset the discovery context working directory to the original directory.
		for _, component := range fromComponents {
			discoveryContext := *component.DiscoveryContext()
			discoveryContext.WorkingDir = fromStack.DiscoveryContext().WorkingDir
			component.SetDiscoveryContext(&discoveryContext)
		}

		return nil
	})

	discoveryGroup.Go(func() error {
		var toDiscoveryErr error

		toComponents, toDiscoveryErr = toDiscovery.
			WithDiscoveryContext(&toDiscoveryContext).
			WithFilters(
				filter.Filters{},
			).
			Discover(discoveryCtx, l, opts)
		if toDiscoveryErr != nil {
			mu.Lock()

			errs = append(errs, toDiscoveryErr)

			mu.Unlock()

			return nil
		}

		// Reset the discovery context working directory to the original directory.
		for _, component := range toComponents {
			discoveryContext := *component.DiscoveryContext()
			discoveryContext.WorkingDir = toStack.DiscoveryContext().WorkingDir
			component.SetDiscoveryContext(&discoveryContext)
		}

		return nil
	})

	if err = discoveryGroup.Wait(); err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	componentPairs := make([]componentPair, 0, max(len(fromComponents), len(toComponents)))

	for _, fromComponent := range fromComponents {
		fromComponentSuffix := strings.TrimPrefix(
			fromComponent.Path(),
			fromComponent.DiscoveryContext().WorkingDir,
		)

		for _, toComponent := range toComponents {
			toComponentSuffix := strings.TrimPrefix(
				toComponent.Path(),
				toComponent.DiscoveryContext().WorkingDir,
			)

			if filepath.Clean(fromComponentSuffix) == filepath.Clean(toComponentSuffix) {
				componentPairs = append(componentPairs, componentPair{
					FromComponent: fromComponent,
					ToComponent:   toComponent,
				})
			}
		}
	}

	finalComponents := make(component.Components, 0, max(len(fromComponents), len(toComponents)))

	for _, fromComponent := range fromComponents {
		if !slices.ContainsFunc(componentPairs, func(componentPair componentPair) bool {
			return componentPair.FromComponent == fromComponent
		}) {
			finalComponents = append(finalComponents, fromComponent)
		}
	}

	for _, toComponent := range toComponents {
		if !slices.ContainsFunc(componentPairs, func(componentPair componentPair) bool {
			return componentPair.ToComponent == toComponent
		}) {
			finalComponents = append(finalComponents, toComponent)
		}
	}

	for _, componentPair := range componentPairs {
		var fromSHA, toSHA string

		shaGroup, _ := errgroup.WithContext(ctx)
		shaGroup.SetLimit(min(runtime.NumCPU(), 2)) //nolint:mnd

		shaGroup.Go(func() error {
			fromSHA, err = generateDirSHA256(componentPair.FromComponent.Path())
			if err != nil {
				return err
			}

			return nil
		})

		shaGroup.Go(func() error {
			toSHA, err = generateDirSHA256(componentPair.ToComponent.Path())
			if err != nil {
				return err
			}

			return nil
		})

		if err := shaGroup.Wait(); err != nil {
			return nil, err
		}

		if fromSHA != toSHA {
			finalComponents = append(finalComponents, componentPair.ToComponent)
		}
	}

	return finalComponents, nil
}

type worktreeKind int

const (
	fromWorktreeKind worktreeKind = iota
	toWorktreeKind
)

// translateDiscoveryContextArgsForWorktree translates the discovery context arguments for a worktree.
func translateDiscoveryContextArgsForWorktree(
	discoveryContext component.DiscoveryContext,
	worktreeKind worktreeKind,
) (component.DiscoveryContext, error) {
	switch worktreeKind {
	case fromWorktreeKind:
		switch {
		case (discoveryContext.Cmd == "plan" || discoveryContext.Cmd == "apply") &&
			!slices.Contains(discoveryContext.Args, "-destroy"):
			discoveryContext.Args = append(discoveryContext.Args, "-destroy")
		case (discoveryContext.Cmd == "" && len(discoveryContext.Args) == 0):
			// This is the case when using a discovery command like find or list.
			// It's fine for these commands to not have any command or arguments.
		default:
			return discoveryContext, NewGitFilterCommandError(discoveryContext.Cmd, discoveryContext.Args)
		}

		return discoveryContext, nil
	case toWorktreeKind:
		// This branch is just for validation.
		switch {
		case (discoveryContext.Cmd == "plan" || discoveryContext.Cmd == "apply") &&
			!slices.Contains(discoveryContext.Args, "-destroy"):
			// We don't need to add the -destroy flag for to worktrees, as we're not destroying anything.
		case (discoveryContext.Cmd == "" && len(discoveryContext.Args) == 0):
			// This is the case when using a discovery command like find or list.
			// It's fine for these commands to not have any command or arguments.
		default:
			return discoveryContext, NewGitFilterCommandError(discoveryContext.Cmd, discoveryContext.Args)
		}

		return discoveryContext, nil
	default:
		return discoveryContext, NewGitFilterCommandError(discoveryContext.Cmd, discoveryContext.Args)
	}
}

// generateDirSHA256 calculates a single SHA256 checksum for all files in a directory
// and its subdirectories.
func generateDirSHA256(rootDir string) (string, error) {
	var filePaths []string

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// We ignore the `.terragrunt-stack-manifest` file here, as it encodes the manifest
		// using absolute paths of the contents of the stack, which will always result in a different SHA.
		//
		// We might want to change the way we generate the `.terragrunt-stack-manifest` file to use relative paths,
		// but that's a bigger change than needed for this to work. We might also want to just use this file
		// to evaluate whether there is a diff, but that's obviously not going to work here for the same reason.
		if filepath.Base(path) == ".terragrunt-stack-manifest" {
			return nil
		}

		filePaths = append(filePaths, path)

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error walking directory: %w", err)
	}

	sort.Strings(filePaths)

	hash := sha256.New()

	for _, path := range filePaths {
		// Include the relative path in the hash so renames/moves are detected
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return "", fmt.Errorf("could not compute relative path for %s: %w", path, err)
		}

		// Normalize path separators for cross-platform consistency
		normalizedPath := filepath.ToSlash(relPath)

		// Write path with null separator before content
		_, _ = hash.Write([]byte(normalizedPath))
		_, _ = hash.Write([]byte{0})

		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("could not open file %s: %w", path, err)
		}

		_, err = io.Copy(hash, f)

		closeErr := f.Close()

		if err != nil {
			return "", fmt.Errorf("could not copy file %s to hash: %w", path, err)
		}

		if closeErr != nil {
			return "", fmt.Errorf("could not close file %s: %w", path, closeErr)
		}
	}

	hashBytes := hash.Sum(nil)

	return hex.EncodeToString(hashBytes), nil
}

// pathInWorktree computes the equivalent path in the worktree for the original working directory.
// When running from a subdirectory of the git repo, this returns the equivalent subdirectory in the worktree.
// gitRootOffset is the relative path from git root to the original working directory.
func pathInWorktree(worktreePath, gitRootOffset string) string {
	if gitRootOffset == "" {
		return worktreePath
	}

	return filepath.Join(worktreePath, gitRootOffset)
}
