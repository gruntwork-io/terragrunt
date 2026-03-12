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
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
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
func (p *WorktreePhase) Run(ctx context.Context, l log.Logger, input *PhaseInput) (*PhaseResults, error) {
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

	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	discoveryGroup, discoveryCtx := errgroup.WithContext(ctx)
	discoveryGroup.SetLimit(p.numWorkers)

	for _, pair := range w.WorktreePairs {
		discoveryGroup.Go(func() error {
			fromFilters, toFilters, err := pair.Expand()
			if err != nil {
				return err
			}

			fromToG, fromToCtx := errgroup.WithContext(discoveryCtx)

			if len(fromFilters) > 0 {
				fromToG.Go(func() error {
					components, err := p.discoverInWorktree(fromToCtx, l, input, pair.FromWorktree, fromFilters, FromWorktreeKind)
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
					components, err := p.discoverInWorktree(fromToCtx, l, input, pair.ToWorktree, toFilters, ToWorktreeKind)
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
		components, err := p.discoverChangesInWorktreeStacks(discoveryCtx, l, input, w)
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
		status, reason, graphIdx := StatusDiscovered, CandidacyReasonNone, -1

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
		case StatusDiscovered:
			results.AddDiscovered(result)
		case StatusCandidate:
			results.AddCandidate(result)
		case StatusExcluded:
			// Excluded components are not added
		}
	}

	return results, nil
}

// discoverInWorktree discovers components in a single worktree.
func (p *WorktreePhase) discoverInWorktree(
	ctx context.Context,
	l log.Logger,
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

	components, err := subDiscovery.Discover(ctx, l, input.Opts)
	if err != nil {
		return components, err
	}

	return components, nil
}

// discoverChangesInWorktreeStacks discovers changes in worktree stacks.
func (p *WorktreePhase) discoverChangesInWorktreeStacks(
	ctx context.Context,
	l log.Logger,
	input *PhaseInput,
	w *worktrees.Worktrees,
) (component.Components, error) {
	discoveredComponents := component.NewThreadSafeComponents(component.Components{})

	stackDiff := w.Stacks()

	// Detect stacks affected by changes to files they reference via read_terragrunt_config().
	// This requires parsing infrastructure (ctx, l, opts), which is why it lives here
	// rather than in Stacks().
	readingAffected := findStacksAffectedByReading(ctx, l, input, w, &stackDiff)
	stackDiff.Changed = append(stackDiff.Changed, readingAffected...)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(max(1, min(runtime.NumCPU(), len(stackDiff.Added)+len(stackDiff.Removed)+len(stackDiff.Changed)*2)))

	var (
		mu   sync.Mutex
		errs = make([]error, 0, len(stackDiff.Changed))
	)

	for _, changed := range stackDiff.Changed {
		g.Go(func() error {
			components, err := p.walkChangedStack(ctx, l, input, changed.FromStack, changed.ToStack)
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

// findStacksAffectedByReading detects stacks affected by changes to files they reference
// via read_terragrunt_config(). For each worktree pair, it collects non-stack diff paths,
// identifies which directories contain stack files, parses those stack files to determine
// which files are actually read, and returns changed pairs for stacks whose readings overlap
// with the diff.
func findStacksAffectedByReading(
	ctx context.Context,
	l log.Logger,
	input *PhaseInput,
	w *worktrees.Worktrees,
	existingDiff *worktrees.StackDiff,
) []worktrees.StackDiffChangedPair {
	var result []worktrees.StackDiffChangedPair

	for _, pair := range w.WorktreePairs {
		handledDirs := buildHandledStackDirs(existingDiff, &pair)
		diffPaths := collectNonStackDiffPaths(&pair)

		if len(diffPaths) == 0 {
			continue
		}

		stackDirsToCheck := findStackDirsWithChanges(diffPaths, pair.FromWorktree.Path, pair.ToWorktree.Path, handledDirs)

		for stackDir, paths := range stackDirsToCheck {
			if stackReferencesAnyDiffPath(ctx, l, input, &pair, stackDir, paths) {
				result = append(result, worktrees.StackDiffChangedPair{
					FromStack: component.NewStack(filepath.Join(pair.FromWorktree.Path, stackDir)).WithDiscoveryContext(
						&component.DiscoveryContext{
							WorkingDir: pair.FromWorktree.Path,
							Ref:        pair.FromWorktree.Ref,
						},
					),
					ToStack: component.NewStack(filepath.Join(pair.ToWorktree.Path, stackDir)).WithDiscoveryContext(
						&component.DiscoveryContext{
							WorkingDir: pair.ToWorktree.Path,
							Ref:        pair.ToWorktree.Ref,
						},
					),
				})
			}
		}
	}

	return result
}

// buildHandledStackDirs builds a set of relative directory paths already represented
// in the existing StackDiff results to prevent duplicate detection.
func buildHandledStackDirs(stackDiff *worktrees.StackDiff, pair *worktrees.WorktreePair) map[string]struct{} {
	handled := make(map[string]struct{})

	for _, s := range stackDiff.Added {
		if rel, err := filepath.Rel(pair.ToWorktree.Path, s.Path()); err == nil {
			handled[rel] = struct{}{}
		}
	}

	for _, s := range stackDiff.Removed {
		if rel, err := filepath.Rel(pair.FromWorktree.Path, s.Path()); err == nil {
			handled[rel] = struct{}{}
		}
	}

	for _, ch := range stackDiff.Changed {
		if rel, err := filepath.Rel(pair.ToWorktree.Path, ch.ToStack.Path()); err == nil {
			handled[rel] = struct{}{}
		}
	}

	return handled
}

// collectNonStackDiffPaths returns all diff paths (added, removed, changed) that are not
// stack files themselves.
func collectNonStackDiffPaths(pair *worktrees.WorktreePair) []string {
	if pair.Diffs == nil {
		return nil
	}

	var result []string

	for _, paths := range [][]string{pair.Diffs.Added, pair.Diffs.Removed, pair.Diffs.Changed} {
		for _, p := range paths {
			if filepath.Base(p) != config.DefaultStackFile {
				result = append(result, p)
			}
		}
	}

	return result
}

// findStackDirsWithChanges groups non-stack diff paths by directory, returning only directories
// that contain a terragrunt.stack.hcl in both worktrees and are not already handled.
// Note: only files that are direct siblings of the stack file are detected. References to files
// in other directories (e.g., read_terragrunt_config("../../shared/config.hcl")) are not detected.
func findStackDirsWithChanges(
	diffPaths []string,
	fromWorktree, toWorktree string,
	handledDirs map[string]struct{},
) map[string][]string {
	result := make(map[string][]string)
	seen := make(map[string]bool)

	for _, diffPath := range diffPaths {
		dir := filepath.Dir(diffPath)

		if _, handled := handledDirs[dir]; handled {
			continue
		}

		hasStack, checked := seen[dir]
		if !checked {
			fromExists := util.FileExists(filepath.Join(fromWorktree, dir, config.DefaultStackFile))
			toExists := util.FileExists(filepath.Join(toWorktree, dir, config.DefaultStackFile))
			hasStack = fromExists && toExists
			seen[dir] = hasStack
		}

		if hasStack {
			result[dir] = append(result[dir], diffPath)
		}
	}

	return result
}

// stackReferencesAnyDiffPath parses a stack file and checks whether any of the provided diff
// paths are in the stack's FilesRead list — i.e., actually referenced via read_terragrunt_config().
func stackReferencesAnyDiffPath(
	ctx context.Context,
	l log.Logger,
	input *PhaseInput,
	pair *worktrees.WorktreePair,
	stackDir string,
	diffPaths []string,
) bool {
	// Parse the "to" worktree (current state). We intentionally skip the "from" worktree:
	// if the new stack version no longer references a file, it should not be considered affected.
	stackFilePath := filepath.Join(pair.ToWorktree.Path, stackDir, config.DefaultStackFile)
	if !util.FileExists(stackFilePath) {
		return false
	}

	filesRead := parseStackReadingList(ctx, l, input, pair.ToWorktree.Path, stackFilePath)

	diffSet := make(map[string]struct{}, len(diffPaths))
	for _, dp := range diffPaths {
		diffSet[filepath.ToSlash(dp)] = struct{}{}
	}

	for _, readFile := range filesRead {
		rel, err := filepath.Rel(pair.ToWorktree.Path, readFile)
		if err != nil {
			continue
		}

		if _, found := diffSet[filepath.ToSlash(rel)]; found {
			return true
		}
	}

	return false
}

// parseStackReadingList parses a stack file and returns the list of files read during parsing.
// Errors are logged at debug level; partial results from FilesRead are returned even on error
// since trackFileRead() runs before actual file parsing.
func parseStackReadingList(
	ctx context.Context,
	l log.Logger,
	input *PhaseInput,
	worktreeRoot string,
	stackFilePath string,
) []string {
	stackDir := filepath.Dir(stackFilePath)

	parseOpts := input.Opts.Clone()
	parseOpts.WorkingDir = stackDir
	parseOpts.RootWorkingDir = worktreeRoot
	parseOpts.TerragruntConfigPath = stackFilePath
	parseOpts.OriginalTerragruntConfigPath = stackFilePath
	parseOpts.SkipOutput = true
	parseOpts.Writers.Writer = io.Discard
	parseOpts.Writers.ErrWriter = io.Discard

	parsCtx, pctx := configbridge.NewParsingContext(ctx, l, parseOpts)

	_, err := config.ReadStackConfigFile(parsCtx, l, pctx, stackFilePath, nil)
	if err != nil {
		l.Debugf("Failed to parse stack file %s for reading detection: %v", stackFilePath, err)
	}

	if pctx.FilesRead == nil {
		return nil
	}

	return *pctx.FilesRead
}

// walkChangedStack walks a changed stack and discovers components within it.
func (p *WorktreePhase) walkChangedStack(
	ctx context.Context,
	l log.Logger,
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

		fromComponents, fromDiscoveryErr = fromDiscovery.Discover(discoveryCtx, l, input.Opts)
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

		toComponents, toDiscoveryErr = toDiscovery.Discover(discoveryCtx, l, input.Opts)
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
