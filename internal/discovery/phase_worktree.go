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
	"golang.org/x/sync/errgroup"
)

// WorktreePhase discovers components in Git worktrees for Git-based filters.
type WorktreePhase struct {
	// gitExpressions contains Git filter expressions that require worktree discovery.
	gitExpressions filter.GitExpressions
	// numWorkers is the number of concurrent workers.
	numWorkers int
}

// NewWorktreePhase creates a new WorktreePhase.
func NewWorktreePhase(gitExpressions filter.GitExpressions, numWorkers int) *WorktreePhase {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}

	return &WorktreePhase{
		gitExpressions: gitExpressions,
		numWorkers:     numWorkers,
	}
}

// Name returns the human-readable name of the phase.
func (p *WorktreePhase) Name() string {
	return "worktree"
}

// Kind returns the PhaseKind identifier.
func (p *WorktreePhase) Kind() PhaseKind {
	return PhaseWorktree
}

// NumWorkers returns the number of concurrent workers.
func (p *WorktreePhase) NumWorkers() int {
	return p.numWorkers
}

// Run executes the worktree discovery phase.
func (p *WorktreePhase) Run(ctx context.Context, input *PhaseInput) PhaseOutput {
	discovered := make(chan DiscoveryResult, p.numWorkers*channelBufferMultiplier)
	candidates := make(chan DiscoveryResult, p.numWorkers*channelBufferMultiplier)
	errors := make(chan error, p.numWorkers)
	done := make(chan struct{})

	go func() {
		defer close(discovered)
		defer close(candidates)
		defer close(errors)
		defer close(done)

		p.runDiscovery(ctx, input, discovered, candidates, errors)
	}()

	return PhaseOutput{
		Discovered: discovered,
		Candidates: candidates,
		Done:       done,
		Errors:     errors,
	}
}

// runDiscovery performs the actual worktree discovery.
func (p *WorktreePhase) runDiscovery(
	ctx context.Context,
	input *PhaseInput,
	discovered chan<- DiscoveryResult,
	candidates chan<- DiscoveryResult,
	errors chan<- error,
) {
	discovery := input.Discovery
	if discovery == nil || discovery.worktrees == nil {
		input.Logger.Debug("No worktrees provided, skipping worktree discovery")
		return
	}

	w := discovery.worktrees
	if len(w.WorktreePairs) == 0 {
		input.Logger.Debug("No worktree pairs available, skipping worktree discovery")
		return
	}

	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(p.numWorkers)

	for _, pair := range w.WorktreePairs {
		discoveryGroup.Go(func() error {
			fromFilters, toFilters := pair.Expand()

			fromToG, fromToCtx := errgroup.WithContext(discoveryCtx)

			if len(fromFilters) > 0 {
				fromToG.Go(func() error {
					components, err := p.discoverInWorktree(fromToCtx, input, pair.FromWorktree, fromFilters, FromWorktreeKind)
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
					components, err := p.discoverInWorktree(fromToCtx, input, pair.ToWorktree, toFilters, ToWorktreeKind)
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
		components, err := p.discoverChangesInWorktreeStacks(discoveryCtx, input, w)
		if err != nil {
			return err
		}

		for _, c := range components {
			discoveredComponents.EnsureComponent(c)
		}

		return nil
	})

	if err := discoveryGroup.Wait(); err != nil {
		select {
		case errors <- err:
		default:
		}

		return
	}

	for _, c := range discoveredComponents.ToComponents() {
		status, reason, graphIdx := StatusDiscovered, CandidacyReasonNone, -1

		if input.Classifier != nil {
			ctx := filter.ClassificationContext{}
			status, reason, graphIdx = input.Classifier.Classify(c, ctx)
		}

		result := DiscoveryResult{
			Component:            c,
			Status:               status,
			Reason:               reason,
			Phase:                PhaseWorktree,
			GraphExpressionIndex: graphIdx,
		}

		switch result.Status {
		case StatusDiscovered:
			select {
			case discovered <- result:
			case <-ctx.Done():
				return
			}
		case StatusCandidate:
			select {
			case candidates <- result:
			case <-ctx.Done():
				return
			}
		case StatusExcluded:
			// Excluded components are not sent to any channel
		}
	}
}

// discoverInWorktree discovers components in a single worktree.
func (p *WorktreePhase) discoverInWorktree(
	ctx context.Context,
	input *PhaseInput,
	wt worktrees.Worktree,
	filters filter.Filters,
	kind WorktreeKind,
) (component.Components, error) {
	discovery := input.Discovery

	discoveryContext := discovery.discoveryContext.Copy()
	discoveryContext.Ref = wt.Ref
	discoveryContext.WorkingDir = wt.Path
	discoveryContext.SuggestOrigin(component.OriginWorktreeDiscovery)

	if discoveryContext.Args != nil {
		argsCopy := make([]string, len(discoveryContext.Args))
		copy(argsCopy, discoveryContext.Args)
		discoveryContext.Args = argsCopy
	}

	discoveryContext, err := TranslateDiscoveryContextArgsForWorktree(discoveryContext, kind)
	if err != nil {
		return nil, err
	}

	subDiscovery := NewDiscovery(wt.Path).
		WithFilters(filters).
		WithDiscoveryContext(discoveryContext).
		WithNumWorkers(p.numWorkers)

	if discovery.suppressParseErrors {
		subDiscovery = subDiscovery.WithSuppressParseErrors()
	}

	if len(discovery.parserOptions) > 0 {
		subDiscovery = subDiscovery.WithParserOptions(discovery.parserOptions)
	}

	components, err := subDiscovery.Discover(ctx, input.Logger, input.Opts)
	if err != nil {
		return components, err
	}

	return components, nil
}

// discoverChangesInWorktreeStacks discovers changes in worktree stacks.
func (p *WorktreePhase) discoverChangesInWorktreeStacks(
	ctx context.Context,
	input *PhaseInput,
	w *worktrees.Worktrees,
) (component.Components, error) {
	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	stackDiff := w.Stacks()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), len(stackDiff.Added)+len(stackDiff.Removed)+len(stackDiff.Changed)*2))

	var (
		mu   sync.Mutex
		errs = make([]error, 0, len(stackDiff.Changed))
	)

	for _, changed := range stackDiff.Changed {
		g.Go(func() error {
			components, err := p.walkChangedStack(ctx, input, changed.FromStack, changed.ToStack)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()

				return err
			}

			for _, c := range components {
				discoveredComponents.EnsureComponent(c)
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return discoveredComponents.ToComponents(), nil
}

// walkChangedStack walks a changed stack and discovers components within it.
func (p *WorktreePhase) walkChangedStack(
	ctx context.Context,
	input *PhaseInput,
	fromStack *component.Stack,
	toStack *component.Stack,
) (component.Components, error) {
	discovery := input.Discovery

	fromDiscoveryContext := discovery.discoveryContext.Copy()
	fromDiscoveryContext.WorkingDir = fromStack.Path()
	fromDiscoveryContext.Ref = fromStack.DiscoveryContext().Ref

	fromDiscoveryContext, err := TranslateDiscoveryContextArgsForWorktree(fromDiscoveryContext, FromWorktreeKind)
	if err != nil {
		return nil, err
	}

	toDiscoveryContext := discovery.discoveryContext.Copy()
	toDiscoveryContext.WorkingDir = toStack.Path()
	toDiscoveryContext.Ref = toStack.DiscoveryContext().Ref

	toDiscoveryContext, err = TranslateDiscoveryContextArgsForWorktree(toDiscoveryContext, ToWorktreeKind)
	if err != nil {
		return nil, err
	}

	var fromComponents, toComponents component.Components

	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(min(runtime.NumCPU(), 2)) //nolint:mnd

	var (
		mu   sync.Mutex
		errs = make([]error, 0, 2) //nolint:mnd
	)

	discoveryGroup.Go(func() error {
		fromDiscovery := NewDiscovery(fromStack.Path()).
			WithDiscoveryContext(fromDiscoveryContext).
			WithFilters(filter.Filters{}).
			WithNumWorkers(p.numWorkers)

		var fromDiscoveryErr error

		fromComponents, fromDiscoveryErr = fromDiscovery.Discover(discoveryCtx, input.Logger, input.Opts)
		if fromDiscoveryErr != nil {
			mu.Lock()

			errs = append(errs, fromDiscoveryErr)

			mu.Unlock()

			return nil
		}

		for _, c := range fromComponents {
			dc := c.DiscoveryContext().CopyWithNewOrigin(component.OriginWorktreeDiscovery)
			dc.WorkingDir = fromStack.DiscoveryContext().WorkingDir
			c.SetDiscoveryContext(dc)
		}

		return nil
	})

	discoveryGroup.Go(func() error {
		toDiscovery := NewDiscovery(toStack.Path()).
			WithDiscoveryContext(toDiscoveryContext).
			WithFilters(filter.Filters{}).
			WithNumWorkers(p.numWorkers)

		var toDiscoveryErr error

		toComponents, toDiscoveryErr = toDiscovery.Discover(discoveryCtx, input.Logger, input.Opts)
		if toDiscoveryErr != nil {
			mu.Lock()

			errs = append(errs, toDiscoveryErr)

			mu.Unlock()

			return nil
		}

		for _, c := range toComponents {
			dc := c.DiscoveryContext().CopyWithNewOrigin(component.OriginWorktreeDiscovery)
			dc.WorkingDir = toStack.DiscoveryContext().WorkingDir
			c.SetDiscoveryContext(dc)
		}

		return nil
	})

	if err = discoveryGroup.Wait(); err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	componentPairs := MatchComponentPairs(fromComponents, toComponents)

	finalComponents := make(component.Components, 0, max(len(fromComponents), len(toComponents)))

	for _, fromComponent := range fromComponents {
		if !slices.ContainsFunc(componentPairs, func(cp ComponentPair) bool {
			return cp.FromComponent == fromComponent
		}) {
			finalComponents = append(finalComponents, fromComponent)
		}
	}

	for _, toComponent := range toComponents {
		if !slices.ContainsFunc(componentPairs, func(cp ComponentPair) bool {
			return cp.ToComponent == toComponent
		}) {
			finalComponents = append(finalComponents, toComponent)
		}
	}

	for _, pair := range componentPairs {
		var fromSHA, toSHA string

		shaGroup, _ := errgroup.WithContext(ctx)
		shaGroup.SetLimit(min(runtime.NumCPU(), 2)) //nolint:mnd

		shaGroup.Go(func() error {
			var localErr error

			fromSHA, localErr = GenerateDirSHA256(pair.FromComponent.Path())

			return localErr
		})

		shaGroup.Go(func() error {
			var localErr error

			toSHA, localErr = GenerateDirSHA256(pair.ToComponent.Path())

			return localErr
		})

		if err := shaGroup.Wait(); err != nil {
			return nil, err
		}

		if fromSHA != toSHA {
			dc := pair.ToComponent.DiscoveryContext().CopyWithNewOrigin(component.OriginWorktreeDiscovery)
			pair.ToComponent.SetDiscoveryContext(dc)
			finalComponents = append(finalComponents, pair.ToComponent)
		}
	}

	return finalComponents, nil
}

// ComponentPair represents a pair of matched components from different worktrees.
type ComponentPair struct {
	FromComponent component.Component
	ToComponent   component.Component
}

// MatchComponentPairs matches components between from and to stacks by their relative paths.
func MatchComponentPairs(
	fromComponents component.Components,
	toComponents component.Components,
) []ComponentPair {
	componentPairs := make([]ComponentPair, 0, max(len(fromComponents), len(toComponents)))

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
				componentPairs = append(componentPairs, ComponentPair{
					FromComponent: fromComponent,
					ToComponent:   toComponent,
				})
			}
		}
	}

	return componentPairs
}

// WorktreeKind represents the type of worktree (from or to).
type WorktreeKind int

const (
	// FromWorktreeKind represents a "from" worktree (the older reference).
	FromWorktreeKind WorktreeKind = iota
	// ToWorktreeKind represents a "to" worktree (the newer reference).
	ToWorktreeKind
)

// TranslateDiscoveryContextArgsForWorktree translates discovery context arguments for a worktree.
func TranslateDiscoveryContextArgsForWorktree(
	discoveryContext *component.DiscoveryContext,
	wKind WorktreeKind,
) (*component.DiscoveryContext, error) {
	switch wKind {
	case FromWorktreeKind:
		switch {
		case (discoveryContext.Cmd == "plan" || discoveryContext.Cmd == "apply") &&
			!slices.Contains(discoveryContext.Args, "-destroy"):
			discoveryContext.Args = append(discoveryContext.Args, "-destroy")
		case discoveryContext.Cmd == "" && len(discoveryContext.Args) == 0:
			// Discovery commands like find or list - no args needed
		default:
			return discoveryContext, NewGitFilterCommandError(discoveryContext.Cmd, discoveryContext.Args)
		}

		return discoveryContext, nil

	case ToWorktreeKind:
		switch {
		case (discoveryContext.Cmd == "plan" || discoveryContext.Cmd == "apply") &&
			!slices.Contains(discoveryContext.Args, "-destroy"):
			// No -destroy flag needed for to worktrees
		case discoveryContext.Cmd == "" && len(discoveryContext.Args) == 0:
			// Discovery commands like find or list - no args needed
		default:
			return discoveryContext, NewGitFilterCommandError(discoveryContext.Cmd, discoveryContext.Args)
		}

		return discoveryContext, nil

	default:
		return discoveryContext, NewGitFilterCommandError(discoveryContext.Cmd, discoveryContext.Args)
	}
}

// GenerateDirSHA256 calculates a single SHA256 checksum for all files in a directory.
func GenerateDirSHA256(rootDir string) (string, error) {
	var filePaths []string

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Ignore .terragrunt-stack-manifest as it contains absolute paths
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
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return "", fmt.Errorf("could not compute relative path for %s: %w", path, err)
		}

		normalizedPath := filepath.ToSlash(relPath)

		// These writes are guaranteed to succeed. They just return errors because of the
		// Writer interface, but we don't care about those errors.
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

	return hex.EncodeToString(hash.Sum(nil)), nil
}
