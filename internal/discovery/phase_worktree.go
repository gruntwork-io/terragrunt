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
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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
	// Default to available CPUs when no explicit worker count is given.
	if numWorkers <= 0 {
		numWorkers = runtime.GOMAXPROCS(0)
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
func (p *WorktreePhase) Run(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	input *PhaseInput,
) (*PhaseResults, error) {
	results := NewPhaseResults()

	discovery := input.Discovery
	if discovery == nil || discovery.worktrees == nil {
		l.Debug("No worktrees provided, skipping worktree discovery")
		return results, nil
	}

	w := discovery.worktrees
	if len(w.WorktreePairs) == 0 {
		l.Debug("No worktree pairs available, skipping worktree discovery")
		return results, nil
	}

	discoveredComponents := component.NewThreadSafeComponents(v.FS, component.Components{})

	// Behind the canonical-worktree-paths experiment, worktree discoveries are
	// rebased onto the user's working directory before publication (issue #6778).
	canonicalPaths := input.Opts != nil &&
		input.Opts.Experiments.Evaluate(experiment.CanonicalWorktreePaths)

	// Canonical paths join worktree-root-relative paths onto the working
	// directory, which only corresponds when that directory is the repository
	// root; from a subdirectory the mapping would rebase onto unrelated paths,
	// so canonicalization is declined there.
	if canonicalPaths && !w.IsWorkingDirRepoRoot(ctx, v.FS) {
		l.Debugf(
			"canonical-worktree-paths: working directory is not the repository root " +
				"or the root could not be determined, keeping worktree paths",
		)

		canonicalPaths = false
	}

	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(p.numWorkers)

	for _, pair := range w.WorktreePairs {
		discoveryGroup.Go(func() error {
			fromFilters, toFilters, err := pair.Expand(v.FS, discovery.configFilenames...)
			if err != nil {
				return err
			}

			// Expand routes reading filters for deleted files onto the from side, since a deleted file
			// only exists in the from worktree where its read relationship can be evaluated. These need
			// different handling from the path filters for genuinely removed components, so split them.
			deletedReadFilters, removalFilters := fromFilters.PartitionReadingFilters()

			fromToG, fromToCtx := errgroup.WithContext(discoveryCtx)

			if len(removalFilters) > 0 {
				fromToG.Go(func() error {
					components, err := p.discoverInWorktree(
						fromToCtx, l, v, input, pair.FromWorktree, removalFilters, FromWorktreeKind,
					)
					if err != nil {
						return err
					}

					components = canonicalizeWorktreeComponents(v.FS, discovery, &pair, components, canonicalPaths)

					for _, c := range components {
						discoveredComponents.EnsureComponent(v.FS, c)
					}

					return nil
				})
			}

			if len(toFilters) > 0 || len(deletedReadFilters) > 0 {
				fromToG.Go(func() error {
					finalToFilters := toFilters

					if len(deletedReadFilters) > 0 {
						translated, err := p.deletedReadingComponentsToFilters(
							fromToCtx, l, v, input, pair.FromWorktree, deletedReadFilters,
						)
						if err != nil {
							return err
						}

						finalToFilters = slices.Concat(toFilters, translated)
					}

					if len(finalToFilters) == 0 {
						return nil
					}

					components, err := p.discoverInWorktree(
						fromToCtx,
						l,
						v,
						input,
						pair.ToWorktree,
						finalToFilters,
						ToWorktreeKind,
					)
					if err != nil {
						return err
					}

					components = canonicalizeWorktreeComponents(v.FS, discovery, &pair, components, canonicalPaths)

					for _, c := range components {
						discoveredComponents.EnsureComponent(v.FS, c)
					}

					return nil
				})
			}

			return fromToG.Wait()
		})
	}

	discoveryGroup.Go(func() error {
		components, err := p.discoverChangesInWorktreeStacks(discoveryCtx, l, v, input, w, canonicalPaths)
		if err != nil {
			return err
		}

		for _, c := range components {
			discoveredComponents.EnsureComponent(v.FS, c)
		}

		return nil
	})

	if err := discoveryGroup.Wait(); err != nil {
		return nil, err
	}

	if canonicalPaths {
		if err := detectConflictingSelections(discovery, w, discoveredComponents.ToComponents()); err != nil {
			return nil, err
		}
	}

	for _, c := range discoveredComponents.ToComponents() {
		status, reason, graphIdx := filter.StatusReadyForFilter, filter.CandidacyReasonNone, -1

		if input.Classifier != nil {
			classCtx := filter.ClassificationContext{}
			status, reason, graphIdx = input.Classifier.Classify(c, classCtx)
		}

		result := DiscoveryResult{
			Component:            c,
			Status:               status,
			Reason:               reason,
			Phase:                PhaseWorktree,
			GraphExpressionIndex: graphIdx,
		}

		switch result.Status {
		case filter.StatusReadyForFilter:
			results.AddDiscovered(result)
		case filter.StatusCandidate:
			results.AddCandidate(result)
		case filter.StatusExcluded:
			// Excluded components are not added
		}
	}

	return results, nil
}

// discoverInWorktree discovers components in a single worktree.
func (p *WorktreePhase) discoverInWorktree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
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

	// Propagate non-git filters from the parent discovery to the worktree sub-discovery.
	// Git expressions are excluded to avoid infinite recursion (the worktree phase is
	// already handling them). All other filters (path, attribute, negation) are included
	// so that exclusions and type constraints apply within sub-discoveries.
	allFilters := slices.Concat(filters, discovery.filters.ExcludingGitFilters())

	subDiscovery := NewDiscovery(wt.Path).
		WithFilters(allFilters).
		WithDiscoveryContext(discoveryContext).
		WithNumWorkers(p.numWorkers)

	if discovery.suppressParseErrors {
		subDiscovery = subDiscovery.WithSuppressParseErrors()
	}

	if len(discovery.parserOptions) > 0 {
		subDiscovery = subDiscovery.WithParserOptions(discovery.parserOptions)
	}

	// Custom config filenames must carry into the worktree walk, or a repo
	// using them discovers nothing through git expressions.
	if len(discovery.configFilenames) > 0 {
		subDiscovery = subDiscovery.WithConfigFilenames(discovery.configFilenames)
	}

	components, err := subDiscovery.Discover(ctx, l, v, input.Opts)
	if err != nil {
		return components, err
	}

	return components, nil
}

// deletedReadingComponentsToFilters discovers, in the from worktree, the units that read files
// deleted in the diff (those files only exist on the from side), then returns a path filter per
// discovered unit aimed at its equivalent path in the to worktree. A unit that no longer exists in the
// to worktree simply matches nothing there, which is the expected outcome for a genuine removal that
// another filter already covers. Stacks are intentionally skipped: a stack reading a deleted file is
// routed through ReadingAffectedStacks during generation so its generated units get walked, which a
// plain path filter (matching only the stack file) would not achieve.
func (p *WorktreePhase) deletedReadingComponentsToFilters(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	input *PhaseInput,
	fromWorktree worktrees.Worktree,
	readingFilters filter.Filters,
) (filter.Filters, error) {
	affected, err := p.discoverInWorktree(
		ctx,
		l,
		v,
		input,
		fromWorktree,
		readingFilters,
		FromWorktreeKind,
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
func (p *WorktreePhase) discoverChangesInWorktreeStacks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	input *PhaseInput,
	w *worktrees.Worktrees,
	canonicalPaths bool,
) (component.Components, error) {
	discoveredComponents := component.NewThreadSafeComponents(v.FS, component.Components{})

	stackDiff := w.Stacks()

	allChanged := make(
		[]worktrees.StackDiffChangedPair,
		0,
		len(stackDiff.Changed)+len(stackDiff.ReadingAffected),
	)
	allChanged = append(allChanged, stackDiff.Changed...)
	allChanged = append(allChanged, stackDiff.ReadingAffected...)

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
			components, err := p.walkChangedStack(
				ctx,
				l,
				v,
				input,
				changed.FromStack,
				changed.ToStack,
			)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()

				return err
			}

			components = canonicalizeWorktreeComponents(
				v.FS, input.Discovery, &changed.Pair, components, canonicalPaths,
			)

			for _, c := range components {
				discoveredComponents.EnsureComponent(v.FS, c)
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
	l log.Logger,
	v *venv.Venv,
	input *PhaseInput,
	fromStack *component.Stack,
	toStack *component.Stack,
) (component.Components, error) {
	discovery := input.Discovery

	fromDiscoveryContext := discovery.discoveryContext.Copy()
	fromDiscoveryContext.WorkingDir = fromStack.Path()
	fromDiscoveryContext.Ref = fromStack.DiscoveryContext().Ref

	fromDiscoveryContext, err := TranslateDiscoveryContextArgsForWorktree(
		fromDiscoveryContext,
		FromWorktreeKind,
	)
	if err != nil {
		return nil, err
	}

	toDiscoveryContext := discovery.discoveryContext.Copy()
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
	// Run at most 2 discovery tasks (from/to) in parallel, capped by available CPUs.
	discoveryGroup.SetLimit(min(runtime.GOMAXPROCS(0), 2)) //nolint:mnd

	var (
		mu   sync.Mutex
		errs = make([]error, 0, 2) //nolint:mnd
	)

	parentFilters := discovery.filters.ExcludingGitFilters()

	// walkStackSide discovers one side of the stack pair and stamps the
	// results with the worktree-discovery origin under the stack's own dir.
	walkStackSide := func(stack *component.Stack, dCtx *component.DiscoveryContext) (component.Components, error) {
		sideDiscovery := NewDiscovery(stack.Path()).
			WithDiscoveryContext(dCtx).
			WithFilters(parentFilters).
			WithNumWorkers(p.numWorkers)

		// Custom config filenames must carry into the stack walks too.
		if len(discovery.configFilenames) > 0 {
			sideDiscovery = sideDiscovery.WithConfigFilenames(discovery.configFilenames)
		}

		components, discoverErr := sideDiscovery.Discover(discoveryCtx, l, v, input.Opts)
		if discoverErr != nil {
			return nil, discoverErr
		}

		for _, c := range components {
			dc := c.DiscoveryContext().CopyWithNewOrigin(component.OriginWorktreeDiscovery)
			dc.WorkingDir = stack.DiscoveryContext().WorkingDir
			c.SetDiscoveryContext(dc)
		}

		return components, nil
	}

	discoveryGroup.Go(func() error {
		var fromDiscoveryErr error

		fromComponents, fromDiscoveryErr = walkStackSide(fromStack, fromDiscoveryContext)
		if fromDiscoveryErr != nil {
			mu.Lock()

			errs = append(errs, fromDiscoveryErr)

			mu.Unlock()
		}

		return nil
	})

	discoveryGroup.Go(func() error {
		var toDiscoveryErr error

		toComponents, toDiscoveryErr = walkStackSide(toStack, toDiscoveryContext)
		if toDiscoveryErr != nil {
			mu.Lock()

			errs = append(errs, toDiscoveryErr)

			mu.Unlock()
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
		// Hash from/to directories in parallel (at most 2), capped by available CPUs.
		shaGroup.SetLimit(min(runtime.GOMAXPROCS(0), 2)) //nolint:mnd

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
			!slices.Contains(discoveryContext.Args, "-destroy"):
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
			!slices.Contains(discoveryContext.Args, "-destroy"):
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

// canonicalizeWorktreeComponents rebases each component onto the user's
// working directory when the canonical-worktree-paths experiment is enabled.
// The pair that discovered the components anchors removal provenance: refs
// share physical worktrees across expressions, so the owning pair must be
// named by the caller rather than guessed from the path.
func canonicalizeWorktreeComponents(
	fsys vfs.FS,
	d *Discovery,
	pair *worktrees.WorktreePair,
	components component.Components,
	enabled bool,
) component.Components {
	if !enabled {
		return components
	}

	canonical := make(component.Components, 0, len(components))

	for _, c := range components {
		canonical = append(canonical, canonicalizeWorktreeComponent(fsys, d, pair, c))
	}

	return canonical
}

// canonicalizeWorktreeComponent maps a component discovered in a temporary git
// worktree onto its equivalent path under the user's working directory, so the
// repo unit and its worktree twin become one component (issue #6778). Removal
// provenance is decided against the committed to-ref state: a component whose
// config is absent from the pair's to-worktree was removed there and keeps its
// worktree path, so an untracked or restored file in the dirty working tree
// can never turn its destroy plan into an ordinary run.
func canonicalizeWorktreeComponent(
	fsys vfs.FS,
	d *Discovery,
	pair *worktrees.WorktreePair,
	c component.Component,
) component.Component {
	rel, ok := pairRelPath(pair, c.Path())
	if !ok {
		return c
	}

	repoPath := filepath.Join(d.workingDir, rel)
	toPath := filepath.Join(pair.ToWorktree.Path, rel)

	dCtx := canonicalDiscoveryContext(c.DiscoveryContext(), d.workingDir)

	switch cc := c.(type) {
	case *component.Stack:
		if !vfs.IsFile(fsys, filepath.Join(toPath, config.DefaultStackFile)) ||
			!vfs.IsFile(fsys, filepath.Join(repoPath, config.DefaultStackFile)) {
			return c
		}

		canonical := component.NewStack(repoPath)
		canonical.SetDiscoveryContext(dCtx)

		return canonical
	case *component.Unit:
		fname := canonicalUnitConfigFilename(fsys, toPath, repoPath, cc.ConfigFile(), d.configFilenames)
		if fname == "" {
			return c
		}

		canonical := component.NewUnit(repoPath)
		canonical.SetConfigFile(fname)
		canonical.SetDiscoveryContext(dCtx)

		return canonical
	}

	return c
}

// pairRelPath returns path's location relative to whichever of the pair's
// worktree roots holds it.
func pairRelPath(pair *worktrees.WorktreePair, path string) (string, bool) {
	for _, wt := range []worktrees.Worktree{pair.FromWorktree, pair.ToWorktree} {
		if wt.Path == "" {
			continue
		}

		if path != wt.Path && !strings.HasPrefix(path, wt.Path+string(filepath.Separator)) {
			continue
		}

		rel, err := filepath.Rel(wt.Path, path)
		if err != nil {
			return "", false
		}

		return rel, true
	}

	return "", false
}

// canonicalUnitConfigFilename returns a unit config filename present at both
// the pair's to-worktree location and the working-directory location, or ""
// when none is, meaning the unit does not live at the to ref or is not
// materialized under the working directory. Any allowed filename counts, so a
// unit whose config file was renamed between the refs is one unit rather than
// a destroy of the old name racing a run of the new one; the discovered
// filename is only tried first.
func canonicalUnitConfigFilename(
	fsys vfs.FS,
	toPath, repoPath, discovered string,
	configFilenames []string,
) string {
	exists := func(fname string) bool {
		return vfs.IsFile(fsys, filepath.Join(toPath, fname)) &&
			vfs.IsFile(fsys, filepath.Join(repoPath, fname))
	}

	if discovered != "" && exists(discovered) {
		return discovered
	}

	if len(configFilenames) == 0 {
		configFilenames = DefaultConfigFilenames
	}

	for _, fname := range configFilenames {
		if fname == config.DefaultStackFile || fname == discovered {
			continue
		}

		if exists(fname) {
			return fname
		}
	}

	return ""
}

// canonicalDiscoveryContext rebases the worktree discovery context onto the
// working directory, keeping Ref (git-expression matching) intact. The
// from-side -destroy argument is stripped: a component whose config exists at
// the canonical path is by definition not removed, and a from-side twin must
// never race a to-side twin into planning a destroy of a live unit.
func canonicalDiscoveryContext(
	dCtx *component.DiscoveryContext,
	workingDir string,
) *component.DiscoveryContext {
	if dCtx == nil {
		return &component.DiscoveryContext{WorkingDir: workingDir}
	}

	copied := dCtx.Copy()
	copied.WorkingDir = workingDir
	copied.Args = slices.DeleteFunc(copied.Args, func(arg string) bool {
		return arg == "-destroy"
	})

	return copied
}

// detectConflictingSelections fails when one Git expression kept a unit's
// worktree copy for a destroy plan while another canonicalized the same unit
// for a normal run; executing both against one state is never safe.
func detectConflictingSelections(
	d *Discovery,
	w *worktrees.Worktrees,
	components component.Components,
) error {
	const (
		normalRun   = 1
		destroyPlan = 2
	)

	selections := make(map[string]int, len(components))

	for _, c := range components {
		if _, ok := c.(*component.Unit); !ok {
			continue
		}

		dCtx := c.DiscoveryContext()

		switch {
		case c.Path() == d.workingDir ||
			strings.HasPrefix(c.Path(), d.workingDir+string(filepath.Separator)):
			rel, err := filepath.Rel(d.workingDir, c.Path())
			if err != nil {
				continue
			}

			selections[rel] |= normalRun
		case dCtx != nil && slices.Contains(dCtx.Args, "-destroy"):
			rel, ok := worktreeRel(w, c.Path())
			if !ok {
				continue
			}

			selections[rel] |= destroyPlan
		}
	}

	conflicted := make([]string, 0, len(selections))

	for rel, flags := range selections {
		if flags == normalRun|destroyPlan {
			conflicted = append(conflicted, rel)
		}
	}

	if len(conflicted) == 0 {
		return nil
	}

	sort.Strings(conflicted)

	return NewConflictingGitSelectionsError(conflicted)
}

// worktreeRel returns path's location relative to whichever worktree root
// holds it; refs share physical worktrees, so the answer is pair-independent.
func worktreeRel(w *worktrees.Worktrees, path string) (string, bool) {
	for _, pair := range w.WorktreePairs {
		if rel, ok := pairRelPath(&pair, path); ok {
			return rel, true
		}
	}

	return "", false
}
