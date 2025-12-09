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
					components, err := wd.discoverInWorktree(fromToCtx, l, opts, w, pair.FromWorktree, fromFilters, fromWorktreeKind)
					if err != nil {
						return err
					}

					for _, c := range components {
						discoveredComponents.EnsureComponent(c)
					}

					return nil
				})
			}

			if len(toFilters) > 0 {
				fromToG.Go(func() error {
					components, err := wd.discoverInWorktree(fromToCtx, l, opts, w, pair.ToWorktree, toFilters, toWorktreeKind)
					if err != nil {
						return err
					}

					for _, c := range components {
						discoveredComponents.EnsureComponent(c)
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

		// Run directly in worktree paths - no translation needed
		for _, c := range components {
			discoveredComponents.EnsureComponent(c)
		}

		return nil
	})

	if err := discoveryGroup.Wait(); err != nil {
		return nil, err
	}

	return discoveredComponents.ToComponents(), nil
}

// discoverInWorktree discovers components in a single worktree.
func (wd *WorktreeDiscovery) discoverInWorktree(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	w *worktrees.Worktrees,
	wt worktrees.Worktree,
	filters filter.Filters,
	kind worktreeKind,
) (component.Components, error) {
	discovery := *wd.originalDiscovery
	discoveryContext := *discovery.discoveryContext
	discoveryContext.Ref = wt.Ref
	discoveryContext.WorkingDir = wt.Path

	// Deep copy Args slice to avoid race conditions across goroutines
	if discoveryContext.Args != nil {
		argsCopy := make([]string, len(discoveryContext.Args))
		copy(argsCopy, discoveryContext.Args)
		discoveryContext.Args = argsCopy
	}

	discoveryContext, err := translateDiscoveryContextArgsForWorktree(discoveryContext, kind)
	if err != nil {
		return nil, err
	}

	components, err := discovery.
		WithFilters(filters).
		WithDiscoveryContext(&discoveryContext).
		Discover(ctx, l, opts)
	if err != nil {
		return nil, err
	}

	// Adjust WorkingDir to user's subdirectory for display purposes
	adjustedWorkingDir := w.WorkingDir(ctx, wt.Path)

	for _, c := range components {
		if dctx := c.DiscoveryContext(); dctx != nil {
			dctx.WorkingDir = adjustedWorkingDir
		}
	}

	return components, nil
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
