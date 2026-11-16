package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"golang.org/x/sync/errgroup"
)

// fromToTasks is the number of tasks a worktree comparison splits into: one for the from side, one for the to side.
const fromToTasks = 2

// worktreeWorkers returns how many of the discoveries in worktrees run at once, and how many workers each has.
func (d *Discovery) worktreeWorkers() int {
	if d.numWorkers <= 0 {
		return runtime.GOMAXPROCS(0)
	}

	return d.numWorkers
}

// discoverInWorktrees discovers the components the Git filters of d select in their worktrees, each classified
// without a parse. Callers check [Discovery.findsInWorktrees] first.
func (d *Discovery) discoverInWorktrees(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
) (*foundResults, error) {
	results := new(foundResults)

	w := d.worktrees
	if len(w.WorktreePairs) == 0 {
		l.Debug("No worktree pairs available, skipping worktree discovery")
		return results, nil
	}

	discoveredComponents := component.NewThreadSafeComponents(d.paths, component.Components{})

	boundary := d.worktreeBoundary(ctx, l, v)

	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(d.worktreeWorkers())

	for _, pair := range w.WorktreePairs {
		discoveryGroup.Go(func() error {
			// Expand routes reading filters for deleted files onto the from side, since a deleted file
			// only exists in the from worktree where its read relationship can be evaluated. These need
			// different handling from the path filters for genuinely removed components, so split them.
			deletedReadFilters, removalFilters := pair.FromFilters.PartitionReadingFilters()

			fromToG, fromToCtx := errgroup.WithContext(discoveryCtx)

			if len(removalFilters) > 0 {
				fromToG.Go(func() error {
					components, err := d.discoverInWorktree(
						fromToCtx, l, v, s, opts, pair.FromWorktree, removalFilters, FromWorktreeKind, boundary,
					)
					if err != nil {
						return err
					}

					for _, c := range components {
						discoveredComponents.EnsureComponent(c)
					}

					return nil
				})
			}

			if len(pair.ToFilters) > 0 || len(deletedReadFilters) > 0 {
				fromToG.Go(func() error {
					finalToFilters := pair.ToFilters

					if len(deletedReadFilters) > 0 {
						translated, err := d.deletedReadingComponentsToFilters(
							fromToCtx, l, v, s, opts, pair.FromWorktree, deletedReadFilters, boundary,
						)
						if err != nil {
							return err
						}

						finalToFilters = slices.Concat(pair.ToFilters, translated)
					}

					if len(finalToFilters) == 0 {
						return nil
					}

					components, err := d.discoverInWorktree(
						fromToCtx,
						l,
						v,
						s,
						opts,
						pair.ToWorktree,
						finalToFilters,
						ToWorktreeKind,
						boundary,
					)
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
		components, err := d.discoverChangesInWorktreeStacks(discoveryCtx, l, v, s, opts, w, boundary)
		if err != nil {
			return err
		}

		for _, c := range components {
			discoveredComponents.EnsureComponent(c)
		}

		return nil
	})

	if err := discoveryGroup.Wait(); err != nil {
		return nil, err
	}

	for _, c := range discoveredComponents.ToComponents() {
		status, reason, graphIdx := d.classifier.Classify(c, filter.ClassificationContext{})
		if status == filter.StatusExcluded {
			continue
		}

		results.add(DiscoveryResult{
			Component:            c,
			Status:               status,
			Reason:               reason,
			GraphExpressionIndex: graphIdx,
		})
	}

	return results, nil
}

// discoverInWorktree discovers components in a single worktree, narrowed to boundary when set.
func (d *Discovery) discoverInWorktree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	wt worktrees.Worktree,
	filters filter.Filters,
	kind WorktreeKind,
	boundary string,
) (component.Components, error) {
	discoveryContext := d.discoveryContext.Copy()
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

	// Propagate non-git filters from the parent discovery to the worktree sub-discovery.
	// Git expressions are excluded to avoid infinite recursion (the worktree discovery is
	// already handling them). All other filters (path, attribute, negation) are included
	// so that exclusions and type constraints apply within sub-discoveries.
	allFilters := slices.Concat(filters, d.filters.ExcludingGitFilters())

	subDiscovery := NewDiscovery(wt.Path).
		WithFilters(allFilters).
		WithDiscoveryContext(discoveryContext).
		WithNumWorkers(d.worktreeWorkers()).
		withParseSettingsFrom(d)

	if boundary != "" {
		walkRoot, ok, err := WorktreeWalkRoot(v.FS, wt.Path, boundary)
		if err != nil {
			return nil, err
		}

		if !ok {
			return component.Components{}, nil
		}

		subDiscovery = subDiscovery.WithWalkRoot(walkRoot)
	}

	return subDiscovery.Discover(ctx, l, v, s, opts)
}

// withParseSettingsFrom carries a parent discovery's parse settings onto a worktree
// sub-discovery. A sub-discovery that parses more strictly than the parent turns a parse
// error the parent would tolerate into an aborted worktree discovery.
func (d *Discovery) withParseSettingsFrom(parent *Discovery) *Discovery {
	if parent.suppressParseErrors {
		d = d.WithSuppressParseErrors()
	}

	if parent.trackReads {
		d = d.WithTrackReads()
	}

	return d
}

// deletedReadingComponentsToFilters discovers, in the from worktree, the units that read files
// deleted in the diff (those files only exist on the from side), then returns a path filter per
// discovered unit aimed at its equivalent path in the to worktree. A unit that no longer exists in the
// to worktree simply matches nothing there, which is the expected outcome for a genuine removal that
// another filter already covers. Stacks are intentionally skipped: a stack reading a deleted file is
// routed through ReadingAffectedStacks during generation so its generated units get walked, which a
// plain path filter (matching only the stack file) would not achieve.
func (d *Discovery) deletedReadingComponentsToFilters(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	fromWorktree worktrees.Worktree,
	readingFilters filter.Filters,
	boundary string,
) (filter.Filters, error) {
	affected, err := d.discoverInWorktree(
		ctx,
		l,
		v,
		s,
		opts,
		fromWorktree,
		readingFilters,
		FromWorktreeKind,
		boundary,
	)
	if err != nil {
		return nil, err
	}

	toFilters := make(filter.Filters, 0, len(affected))

	for _, c := range affected {
		if _, ok := c.(*component.Stack); ok {
			continue
		}

		relPath, err := filepath.Rel(fromWorktree.Path, c.Path())
		if err != nil {
			return nil, fmt.Errorf(
				"failed to resolve relative path for reading-affected component %s: %w",
				c.Path(),
				err,
			)
		}

		expr, err := filter.NewPathFilter(relPath)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create path filter for reading-affected component %s: %w",
				relPath,
				err,
			)
		}

		toFilters = append(toFilters, filter.NewFilter(expr, expr.String()))
	}

	return toFilters, nil
}

// discoverChangesInWorktreeStacks discovers changes in worktree stacks.
func (d *Discovery) discoverChangesInWorktreeStacks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	w *worktrees.Worktrees,
	boundary string,
) (component.Components, error) {
	discoveredComponents := component.NewThreadSafeComponents(d.paths, component.Components{})

	stackDiff := w.Stacks()

	allChanged := make(
		[]worktrees.StackDiffChangedPair,
		0,
		len(stackDiff.Changed)+len(stackDiff.ReadingAffected),
	)

	for _, pairs := range [][]worktrees.StackDiffChangedPair{stackDiff.Changed, stackDiff.ReadingAffected} {
		for _, changed := range pairs {
			if WithinWorktreeBoundary(v.FS, changed.ToStack, boundary) {
				allChanged = append(allChanged, changed)
			}
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	// Cap workers to the total number of diff operations, but no more than available CPUs (at least 1).
	g.SetLimit(
		max(
			1,
			min(
				runtime.GOMAXPROCS(0),
				len(stackDiff.Added)+len(stackDiff.Removed)+len(allChanged)*2,
			),
		),
	)

	var (
		mu   sync.Mutex
		errs = make([]error, 0, len(allChanged))
	)

	for _, changed := range allChanged {
		g.Go(func() error {
			components, err := d.walkChangedStack(
				ctx,
				l,
				v,
				s,
				opts,
				changed.FromStack,
				changed.ToStack,
			)
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
func (d *Discovery) walkChangedStack(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	fromStack *component.Stack,
	toStack *component.Stack,
) (component.Components, error) {
	fromDiscoveryContext := d.discoveryContext.Copy()
	fromDiscoveryContext.WorkingDir = fromStack.Path()
	fromDiscoveryContext.Ref = fromStack.DiscoveryContext().Ref

	fromDiscoveryContext, err := TranslateDiscoveryContextArgsForWorktree(
		fromDiscoveryContext,
		FromWorktreeKind,
	)
	if err != nil {
		return nil, err
	}

	toDiscoveryContext := d.discoveryContext.Copy()
	toDiscoveryContext.WorkingDir = toStack.Path()
	toDiscoveryContext.Ref = toStack.DiscoveryContext().Ref

	toDiscoveryContext, err = TranslateDiscoveryContextArgsForWorktree(
		toDiscoveryContext,
		ToWorktreeKind,
	)
	if err != nil {
		return nil, err
	}

	var fromComponents, toComponents component.Components

	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(min(runtime.GOMAXPROCS(0), fromToTasks))

	var (
		mu   sync.Mutex
		errs = make([]error, 0, fromToTasks)
	)

	parentFilters := d.filters.ExcludingGitFilters()

	discoveryGroup.Go(func() error {
		fromDiscovery := NewDiscovery(fromStack.Path()).
			WithDiscoveryContext(fromDiscoveryContext).
			WithFilters(parentFilters).
			WithNumWorkers(d.worktreeWorkers()).
			withParseSettingsFrom(d)

		var fromDiscoveryErr error

		fromComponents, fromDiscoveryErr = fromDiscovery.Discover(discoveryCtx, l, v, s, opts)
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
			WithFilters(parentFilters).
			WithNumWorkers(d.worktreeWorkers()).
			withParseSettingsFrom(d)

		var toDiscoveryErr error

		toComponents, toDiscoveryErr = toDiscovery.Discover(discoveryCtx, l, v, s, opts)
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

	componentPairs, err := MatchComponentPairs(fromComponents, toComponents)
	if err != nil {
		return nil, err
	}

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
		shaGroup.SetLimit(min(runtime.GOMAXPROCS(0), fromToTasks))

		shaGroup.Go(func() error {
			var localErr error

			fromSHA, localErr = GenerateDirSHA256(v.FS, pair.FromComponent.Path())

			return localErr
		})

		shaGroup.Go(func() error {
			var localErr error

			toSHA, localErr = GenerateDirSHA256(v.FS, pair.ToComponent.Path())

			return localErr
		})

		if err := shaGroup.Wait(); err != nil {
			return nil, err
		}

		if fromSHA != toSHA {
			dc := pair.ToComponent.DiscoveryContext().
				CopyWithNewOrigin(component.OriginWorktreeDiscovery)
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
) ([]ComponentPair, error) {
	componentPairs := make([]ComponentPair, 0, max(len(fromComponents), len(toComponents)))

	for _, fromComponent := range fromComponents {
		if fromComponent.DiscoveryContext() == nil {
			return nil, NewMissingDiscoveryContextError(fromComponent.Path())
		}

		fromComponentSuffix := strings.TrimPrefix(
			fromComponent.Path(),
			fromComponent.DiscoveryContext().WorkingDir,
		)

		for _, toComponent := range toComponents {
			if toComponent.DiscoveryContext() == nil {
				return nil, NewMissingDiscoveryContextError(toComponent.Path())
			}

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

	return componentPairs, nil
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
			!iacargs.HasDestroyFlag(discoveryContext.Args):
			discoveryContext.Args = append(discoveryContext.Args, "-destroy")
		case discoveryContext.Cmd == "" && len(discoveryContext.Args) == 0:
			// Discovery commands like find or list - no args needed
		default:
			return discoveryContext, NewGitFilterCommandError(
				discoveryContext.Cmd,
				discoveryContext.Args,
			)
		}

		return discoveryContext, nil

	case ToWorktreeKind:
		switch {
		case (discoveryContext.Cmd == "plan" || discoveryContext.Cmd == "apply") &&
			!iacargs.HasDestroyFlag(discoveryContext.Args):
			// No -destroy flag needed for to worktrees
		case discoveryContext.Cmd == "" && len(discoveryContext.Args) == 0:
			// Discovery commands like find or list - no args needed
		default:
			return discoveryContext, NewGitFilterCommandError(
				discoveryContext.Cmd,
				discoveryContext.Args,
			)
		}

		return discoveryContext, nil

	default:
		return discoveryContext, NewGitFilterCommandError(
			discoveryContext.Cmd,
			discoveryContext.Args,
		)
	}
}

// GenerateDirSHA256 calculates a single SHA256 checksum for all files in a directory.
func GenerateDirSHA256(fsys vfs.FS, rootDir string) (string, error) {
	var filePaths []string

	err := vfs.WalkDir(fsys, rootDir, func(path string, d fs.DirEntry, err error) error {
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

		f, err := fsys.Open(path)
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
