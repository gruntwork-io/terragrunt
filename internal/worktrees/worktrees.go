// Package worktrees provides functionality for creating and managing Git worktrees for operating across multiple
// Git references.
package worktrees

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/errgroup"
)

const (
	// worktreesPerPair is the number of worktrees in a comparison pair: the from worktree and the to worktree.
	worktreesPerPair = 2
)

// Worktrees is a map of WorktreePairs, and the Git runner used to create and manage the worktrees.
// The key is the string representation of the GitExpression that generated the worktree pair.
type Worktrees struct {
	// WorktreePairs maps Git expression strings to their corresponding worktree pairs.
	WorktreePairs map[string]*WorktreePair
	// OriginalWorkingDir is the user's working directory before worktrees were created.
	OriginalWorkingDir string
	// ReadingAffectedStacks holds stacks identified during generation as affected by
	// changes to files they read, even though the stack file itself did not change in
	// the git diff. These are included in Stacks().ReadingAffected so that the worktree
	// discovery phase walks them for unit-level changes.
	ReadingAffectedStacks []StackDiffChangedPair
}

// WorktreePair is a pair of worktrees, one for the from and one for the to reference, along with
// the GitExpression that generated the diffs, the diff for that expression, and what
// [WorktreePair.Expand] made of that diff.
type WorktreePair struct {
	GitExpression *filter.GitExpression
	Diffs         *git.Diffs
	FromWorktree  Worktree
	ToWorktree    Worktree
	FromFilters   filter.Filters
	ToFilters     filter.Filters
}

// Worktree is collects a Git reference and the path to the associated worktree.
type Worktree struct {
	Ref  string
	Path string
}

// WorktreeOpts contains parameters for NewWorktrees.
type WorktreeOpts struct {
	WorkingDir        string
	GitExpressions    filter.GitExpressions
	Experiments       experiment.Experiments
	FilteredPathsOnly bool
}

// DisplayPath translates a worktree path to the equivalent path in the original repository
// for user-facing output. This is useful for logging and reporting where users expect to see
// paths relative to their working directory, not temporary worktree paths.
// If the path is not within a worktree, it returns the path unchanged.
func (w *Worktrees) DisplayPath(worktreePath string) string {
	for _, pair := range w.WorktreePairs {
		for _, wt := range []Worktree{pair.FromWorktree, pair.ToWorktree} {
			if wt.Path == "" {
				continue
			}

			// Use boundary-aware check to avoid false matches (e.g., "/tmp/work" vs "/tmp/work-other")
			if worktreePath == wt.Path ||
				strings.HasPrefix(worktreePath, wt.Path+string(os.PathSeparator)) {
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
func (w *Worktrees) Cleanup(ctx context.Context, l log.Logger, v *venv.Venv) error {
	toRemove := w.worktreesToRemove()

	if len(toRemove) == 0 {
		return nil
	}

	gitRunner, err := git.NewGitRunner(v)
	if err != nil {
		return fmt.Errorf("failed to create Git runner for worktree cleanup: %w", err)
	}

	gitRunner = gitRunner.WithWorkDir(w.OriginalWorkingDir)

	return filter.TraceGitWorktreesCleanup(
		ctx,
		len(w.WorktreePairs),
		gitRunner.GetRemoteURL(ctx),
		func(ctx context.Context) error {
			g, groupCtx := errgroup.WithContext(ctx)
			// Every worktree was created under the same temporary directory, so
			// the first one's filesystem sizes removal for all of them.
			g.SetLimit(min(vfs.FSWorkersFor(v.FS, toRemove[0].Path), len(toRemove)))

			for _, worktree := range toRemove {
				g.Go(func() error {
					// Skip removal if the worktree path doesn't exist (may have been cleaned up already)
					if _, err := v.FS.Stat(worktree.Path); errors.Is(err, fs.ErrNotExist) {
						l.Debugf(
							"Worktree path %s already removed, skipping cleanup",
							worktree.Path,
						)

						return nil
					}

					err := filter.TraceGitWorktreeRemove(
						groupCtx,
						worktree.Ref,
						worktree.Path,
						func(ctx context.Context) error {
							return gitRunner.RemoveWorktree(ctx, worktree.Path)
						},
					)
					if err != nil {
						// If the error is due to the worktree not existing, log and continue
						// This can happen during parallel test execution or if cleanup runs twice
						errStr := err.Error()
						if strings.Contains(errStr, "No such file or directory") ||
							strings.Contains(errStr, "does not exist") ||
							strings.Contains(errStr, "not a valid directory") {
							l.Debugf(
								"Worktree for reference %s already cleaned up: %v",
								worktree.Ref,
								err,
							)

							return nil
						}

						return fmt.Errorf(
							"failed to remove Git worktree for reference %s (%s): %w",
							worktree.Ref,
							worktree.Path,
							err,
						)
					}

					return nil
				})
			}

			return g.Wait()
		},
	)
}

// worktreesToRemove returns the worktrees Cleanup has to delete, one entry per
// path. A reference that failed to materialize is left out: it carries no path,
// and the pair it belongs to was recorded all the same.
func (w *Worktrees) worktreesToRemove() []Worktree {
	toRemove := make([]Worktree, 0, len(w.WorktreePairs)*worktreesPerPair)

	for _, pair := range w.WorktreePairs {
		for _, worktree := range []Worktree{pair.FromWorktree, pair.ToWorktree} {
			if worktree.Path == "" {
				continue
			}

			toRemove = append(toRemove, worktree)
		}
	}

	slices.SortFunc(toRemove, func(a, b Worktree) int {
		return strings.Compare(a.Path, b.Path)
	})

	return slices.CompactFunc(toRemove, func(a, b Worktree) bool {
		return a.Path == b.Path
	})
}

type StackDiff struct {
	// Added contains stacks whose terragrunt.stack.hcl was added in the git diff.
	Added []*component.Stack
	// Removed contains stacks whose terragrunt.stack.hcl was removed in the git diff.
	Removed []*component.Stack
	// Changed contains stacks whose terragrunt.stack.hcl was modified in the git diff.
	Changed []StackDiffChangedPair
	// ReadingAffected contains stacks whose terragrunt.stack.hcl did not change directly,
	// but that read files which did. These are identified during stack generation and need
	// to be walked for unit-level changes.
	ReadingAffected []StackDiffChangedPair
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

			dir := filepath.Dir(added)

			stackDiff.Added = append(
				stackDiff.Added,
				component.NewStack(filepath.Join(toWorktree, dir)).WithDiscoveryContext(
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

			dir := filepath.Dir(removed)

			stackDiff.Removed = append(
				stackDiff.Removed,
				component.NewStack(filepath.Join(fromWorktree, dir)).WithDiscoveryContext(
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

			dir := filepath.Dir(changed)

			stackDiff.Changed = append(
				stackDiff.Changed,
				StackDiffChangedPair{
					FromStack: component.NewStack(filepath.Join(fromWorktree, dir)).
						WithDiscoveryContext(
							&component.DiscoveryContext{
								WorkingDir: fromWorktree,
								Ref:        pair.FromWorktree.Ref,
							},
						),
					ToStack: component.NewStack(filepath.Join(toWorktree, dir)).
						WithDiscoveryContext(
							&component.DiscoveryContext{
								WorkingDir: toWorktree,
								Ref:        pair.ToWorktree.Ref,
							},
						),
				},
			)
		}
	}

	stackDiff.ReadingAffected = w.ReadingAffectedStacks

	return stackDiff
}

// Expand expands a worktree pair with an associated Git expression into the
// equivalent to and from filter expressions based on the diffs for the pair.
//
// Whether a changed file sits beside a unit is read from the paths of the "to"
// reference, so a pair can be expanded before any worktree exists.
func (wp *WorktreePair) Expand(toTree git.TreePaths) (filter.Filters, filter.Filters, error) {
	diffs := wp.Diffs

	fromExpressions := make(filter.Expressions, 0, len(diffs.Removed))
	toExpressions := make(filter.Expressions, 0, len(diffs.Added)+len(diffs.Changed))

	// Build simple expressions that can be determined simply from the diffs.
	if err := expandDiffPaths(toTree, diffs.Removed, &fromExpressions, &toExpressions); err != nil {
		return nil, nil, err
	}

	if err := expandDiffPaths(toTree, diffs.Added, &toExpressions, &toExpressions); err != nil {
		return nil, nil, err
	}

	for _, path := range diffs.Changed {
		dir := filepath.Dir(path)

		switch filepath.Base(path) {
		case config.DefaultTerragruntConfigPath:
			expr, err := filter.NewPathFilter(dir)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create path filter for %s: %w", dir, err)
			}

			toExpressions = append(toExpressions, expr)
		default:
			// A changed file beside a unit means that unit was modified.
			if toTree.Has(unitConfigBeside(path)) {
				expr, err := filter.NewPathFilter(dir)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create path filter for %s: %w", dir, err)
				}

				toExpressions = append(toExpressions, expr)

				continue
			}

			// Otherwise, we'll consider it a file that could potentially be read by other units, and needs to be
			// tracked using a reading filter.
			expr, err := filter.NewAttributeExpression(filter.AttributeReading, path)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create reading filter for %s: %w", path, err)
			}

			toExpressions = append(toExpressions, expr)
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

	return fromFilters, toFilters, nil
}

// expansion is the diff between a Git expression's references and the filters
// that diff expands into.
type expansion struct {
	diffs       *git.Diffs
	fromFilters filter.Filters
	toFilters   filter.Filters
}

// gitSurvey records the diffs, tree listings and expansions read for a set of
// Git expressions, before any worktree exists. Expansions are keyed by Git
// expression string and trees by reference.
type gitSurvey struct {
	expansions map[string]expansion
	trees      map[string]git.TreePaths
}

// neededRefs returns the references that have to be materialized. An expression
// adding units reads only its "to" side, and one removing them only its "from"
// side, so the other is never checked out.
func (s *gitSurvey) neededRefs(gitExpressions filter.GitExpressions) map[string]struct{} {
	needed := make(map[string]struct{}, len(s.trees))

	for _, gitExpression := range gitExpressions {
		expansion := s.expansions[gitExpression.String()]

		fromReadFilters, _ := expansion.fromFilters.PartitionReadingFilters()
		toReadFilters, _ := expansion.toFilters.PartitionReadingFilters()

		// Both sides are read when a file crosses between them. Stack
		// generation pairs the stack at one reference with the stack at the
		// other, and a file deleted from one is read by the units of the other.
		both := len(fromReadFilters) > 0 ||
			len(toReadFilters) > 0 ||
			diffHasStackFile(expansion.diffs)

		if len(expansion.fromFilters) > 0 || both {
			needed[gitExpression.FromRef] = struct{}{}
		}

		if len(expansion.toFilters) > 0 || both {
			needed[gitExpression.ToRef] = struct{}{}
		}
	}

	return needed
}

// diffHasStackFile reports whether a stack file appears anywhere in diffs.
func diffHasStackFile(diffs *git.Diffs) bool {
	if diffs == nil {
		return false
	}

	for _, paths := range [][]string{diffs.Added, diffs.Removed, diffs.Changed} {
		if slices.ContainsFunc(paths, func(p string) bool {
			return path.Base(p) == config.DefaultStackFile
		}) {
			return true
		}
	}

	return false
}

// pathspecsPerRef returns the directories each reference has to be checked out
// with, and reports whether those directories are the whole of what a run
// reads.
//
// They are when every expanded filter names a path, because discovery then
// walks the worktree for exactly those components. A filter matching on what a
// unit reads calls for parsing instead, as does a stack file, and a
// configuration being parsed can read anything in the tree.
func (s *gitSurvey) pathspecsPerRef(l log.Logger, gitExpressions filter.GitExpressions) (map[string][]string, bool) {
	for ref, tree := range s.trees {
		for treePath := range tree {
			if path.Base(treePath) != config.DefaultStackFile {
				continue
			}

			l.Debugf(
				"Checking out whole worktrees: %s in %s is generated by parsing it",
				treePath,
				ref,
			)

			return nil, false
		}
	}

	pathspecs := make(map[string][]string, len(s.trees))

	for _, gitExpression := range gitExpressions {
		expansion := s.expansions[gitExpression.String()]

		for _, side := range []struct {
			ref     string
			filters filter.Filters
		}{
			{filters: expansion.fromFilters, ref: gitExpression.FromRef},
			{filters: expansion.toFilters, ref: gitExpression.ToRef},
		} {
			paths, ok := filterPaths(side.filters)
			if !ok {
				if expr, requiresParse := side.filters.RequiresParse(); requiresParse {
					l.Debugf(
						"Checking out whole worktrees: the filter %s needs configurations parsed inside them",
						expr,
					)
				}

				return nil, false
			}

			pathspecs[side.ref] = append(pathspecs[side.ref], paths...)
		}
	}

	return pathspecs, true
}

// filterPaths returns the directories the filters name, and reports whether
// every filter named one. A filter matching on what a component reads names no
// directory to check out.
func filterPaths(filters filter.Filters) ([]string, bool) {
	paths := make([]string, 0, len(filters))

	for _, f := range filters {
		expr, ok := f.Expression().(*filter.PathExpression)
		if !ok {
			return nil, false
		}

		// Pathspecs are relative to the repository root, so "." names the
		// whole tree and checks nothing out early.
		if expr.Value == "" || expr.Value == "." {
			return nil, false
		}

		// Git reads a wildcard or a leading colon in a pathspec as a pattern of
		// its own, which selects a different set of paths than the glob this
		// filter compiled.
		if strings.ContainsAny(expr.Value, "*?[]") || strings.HasPrefix(expr.Value, ":") {
			return nil, false
		}

		paths = append(paths, expr.Value)
	}

	return paths, true
}

// newEmptyWorktrees returns a Worktrees with no pairs, for a run with no Git
// expressions or one that failed before it had any pairs.
func newEmptyWorktrees(workingDir string) *Worktrees {
	return &Worktrees{
		WorktreePairs:      make(map[string]*WorktreePair),
		OriginalWorkingDir: workingDir,
	}
}

// refsToMaterialize returns the references to check out, in the order given,
// logging the ones left out.
func (s *gitSurvey) refsToMaterialize(l log.Logger, gitExpressions filter.GitExpressions, gitRefs []string) []string {
	needed := s.neededRefs(gitExpressions)

	refs := make([]string, 0, len(needed))

	for _, ref := range gitRefs {
		if _, ok := needed[ref]; !ok {
			l.Debugf("Skipping the worktree for reference %s: nothing reads it", ref)

			continue
		}

		refs = append(refs, ref)
	}

	return refs
}

// checkoutPathspecs returns the directories to check each reference out with,
// and nil when the references are checked out in full.
func (s *gitSurvey) checkoutPathspecs(
	l log.Logger,
	gitExpressions filter.GitExpressions,
	filteredPathsOnly bool,
) map[string][]string {
	if !filteredPathsOnly {
		return nil
	}

	pathspecs, filtered := s.pathspecsPerRef(l, gitExpressions)
	if !filtered {
		return nil
	}

	return pathspecs
}

// surveyGitExpressions reads the diff for every expression and the tree listing
// for every reference, then expands each expression into the filters it stands
// for. Every command it runs reads the repository, never a worktree.
func surveyGitExpressions(
	ctx context.Context,
	v *venv.Venv,
	gitRunner *git.GitRunner,
	gitExpressions filter.GitExpressions,
	gitRefs []string,
	repoRemote string,
) (*gitSurvey, error) {
	var (
		mu      sync.Mutex
		diffs   = make(map[string]*git.Diffs, len(gitExpressions))
		trees   = make(map[string]git.TreePaths, len(gitRefs))
		errs    []error
		g, gCtx = errgroup.WithContext(ctx)
	)

	g.SetLimit(min(runtime.GOMAXPROCS(0), len(gitRefs)+len(gitExpressions)))

	for _, ref := range gitRefs {
		g.Go(func() error {
			tree, err := gitRunner.LsTreeNames(gCtx, v, ref)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errs = append(errs, fmt.Errorf("failed to list the tree at reference %s: %w", ref, err))

				return nil
			}

			trees[ref] = tree

			return nil
		})
	}

	for _, gitExpression := range gitExpressions {
		g.Go(func() error {
			var expressionDiffs *git.Diffs

			diffErr := filter.TraceGitDiff(
				gCtx, gitExpression.FromRef, gitExpression.ToRef, repoRemote,
				func(ctx context.Context) error {
					var err error

					expressionDiffs, err = gitRunner.Diff(ctx, gitExpression.FromRef, gitExpression.ToRef)

					return err
				})

			mu.Lock()
			defer mu.Unlock()

			if diffErr != nil {
				errs = append(errs, diffErr)

				return nil
			}

			diffs[gitExpression.String()] = expressionDiffs

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	survey := &gitSurvey{
		expansions: make(map[string]expansion, len(gitExpressions)),
		trees:      trees,
	}

	for _, gitExpression := range gitExpressions {
		pair := WorktreePair{GitExpression: gitExpression, Diffs: diffs[gitExpression.String()]}

		fromFilters, toFilters, err := pair.Expand(trees[gitExpression.ToRef])
		if err != nil {
			return nil, fmt.Errorf("failed to expand the Git expression %s: %w", gitExpression, err)
		}

		survey.expansions[gitExpression.String()] = expansion{
			diffs:       pair.Diffs,
			fromFilters: fromFilters,
			toFilters:   toFilters,
		}
	}

	return survey, nil
}

// NewWorktrees creates a new Worktrees for a given set of Git filters.
//
// Note that it is the responsibility of the caller to call Cleanup on the Worktrees object when it is no longer needed.
//
// Set FilteredPathsOnly only when the caller reads the worktrees to discover
// the components the filters name and reads nothing else. A worktree created
// for such a caller may contain those directories alone, leaving out the module
// sources a run needs and the files a configuration being parsed can read.
func NewWorktrees(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts WorktreeOpts,
) (*Worktrees, error) {
	workingDir := opts.WorkingDir
	gitExpressions := opts.GitExpressions
	experiments := opts.Experiments

	if len(gitExpressions) == 0 {
		return newEmptyWorktrees(workingDir), nil
	}

	gitRefs := gitExpressions.UniqueGitRefs()

	var (
		worktrees *Worktrees
		outerErr  error
	)

	v.RequireExec()

	gitRunner, err := git.NewGitRunner(v)
	if err != nil {
		return nil, fmt.Errorf("failed to create Git runner for worktree creation: %w", err)
	}

	gitRunner = gitRunner.WithWorkDir(workingDir)

	// Get repo info for telemetry
	repoRemote := gitRunner.GetRemoteURL(ctx)
	repoBranch := gitRunner.GetCurrentBranch(ctx)
	repoCommit := gitRunner.GetHeadCommit(ctx)

	// Wrap entire worktree creation process with telemetry
	traceErr := filter.TraceGitWorktreesCreate(
		ctx, workingDir, len(gitRefs), repoRemote, repoBranch, repoCommit,
		func(ctx context.Context) error {
			// The survey decides which references get a worktree and how much
			// of each one is checked out, so it runs before any of them are.
			survey, err := surveyGitExpressions(ctx, v, gitRunner, gitExpressions, gitRefs, repoRemote)
			if err != nil {
				worktrees = newEmptyWorktrees(workingDir)
				outerErr = err

				return err
			}

			// A symlink in a checked-out directory can point anywhere in the
			// reference, so discovery that follows one walks out of the
			// directories the filters name.
			filteredPaths := opts.FilteredPathsOnly && !experiments.Evaluate(experiment.Symlinks)

			pathspecs := survey.checkoutPathspecs(l, gitExpressions, filteredPaths)

			refsToPaths, err := createGitWorktrees(
				ctx,
				l,
				v,
				gitRunner,
				survey.refsToMaterialize(l, gitExpressions, gitRefs),
				repoRemote,
				repoBranch,
				repoCommit,
				experiments,
				pathspecs,
			)

			worktrees = &Worktrees{
				WorktreePairs:      survey.worktreePairs(gitExpressions, refsToPaths),
				OriginalWorkingDir: workingDir,
			}

			if err != nil {
				outerErr = err

				return err
			}

			for _, gitExpression := range gitExpressions {
				if diffs := survey.expansions[gitExpression.String()].diffs; diffs != nil {
					recordDiffTelemetry(ctx, diffs)
				}
			}

			return nil
		})

	if traceErr != nil && outerErr == nil {
		l.Warnf("telemetry trace error during worktree creation: %v", traceErr)
	}

	// cleanup worktrees
	if outerErr != nil && worktrees != nil {
		if cleanupErr := worktrees.Cleanup(ctx, l, v); cleanupErr != nil {
			l.Warnf("failed to cleanup worktrees: %v", cleanupErr)
		}
	}

	return worktrees, outerErr
}

// worktreePairs pairs each Git expression with its expansion and the worktrees
// of its references. A reference absent from refsToPaths gets a worktree with
// no path.
func (s *gitSurvey) worktreePairs(
	gitExpressions filter.GitExpressions,
	refsToPaths map[string]string,
) map[string]*WorktreePair {
	pairs := make(map[string]*WorktreePair, len(gitExpressions))

	for _, gitExpression := range gitExpressions {
		expansion := s.expansions[gitExpression.String()]

		pairs[gitExpression.String()] = &WorktreePair{
			GitExpression: gitExpression,
			Diffs:         expansion.diffs,
			FromFilters:   expansion.fromFilters,
			ToFilters:     expansion.toFilters,
			FromWorktree: Worktree{
				Ref:  gitExpression.FromRef,
				Path: refsToPaths[gitExpression.FromRef],
			},
			ToWorktree: Worktree{
				Ref:  gitExpression.ToRef,
				Path: refsToPaths[gitExpression.ToRef],
			},
		}
	}

	return pairs
}

// expandDiffPaths processes a list of added or removed paths from a worktree diff, creating filter
// expressions for affected units and stacks. primaryExprs receives filters for config files (units/stacks)
// and for non-config files read by other units via a glob (e.g. mark_glob_as_read), since those must be
// matched against the same worktree the file exists in. fallbackExprs receives path filters for non-config
// files adjacent to a unit in the "to" worktree.
func expandDiffPaths(
	toTree git.TreePaths,
	paths []string,
	primaryExprs, fallbackExprs *filter.Expressions,
) error {
	for _, path := range paths {
		dir := filepath.Dir(path)

		switch filepath.Base(path) {
		case config.DefaultTerragruntConfigPath:
			expr, err := filter.NewPathFilter(dir)
			if err != nil {
				return fmt.Errorf("failed to create path filter for %s: %w", dir, err)
			}

			*primaryExprs = append(*primaryExprs, expr)
		case config.DefaultStackFile:
			dirExpr, err := filter.NewPathFilter(dir)
			if err != nil {
				return fmt.Errorf("failed to create path filter for %s: %w", dir, err)
			}

			globExpr, err := filter.NewPathFilter(filepath.Join(dir, "**"))
			if err != nil {
				return fmt.Errorf("failed to create path filter for %s/**: %w", dir, err)
			}

			*primaryExprs = append(*primaryExprs, dirExpr, globExpr)
		default:
			if toTree.Has(unitConfigBeside(path)) {
				expr, err := filter.NewPathFilter(dir)
				if err != nil {
					return fmt.Errorf("failed to create path filter for %s: %w", dir, err)
				}

				*fallbackExprs = append(*fallbackExprs, expr)

				continue
			}

			// The file isn't adjacent to a unit, but a unit may still read it via a glob
			// (e.g. mark_glob_as_read). Track it with a reading filter so units that read it are
			// selected. primaryExprs targets the worktree the file exists in: the "to" worktree for
			// added files, the "from" worktree for removed files (where the deleted file is still present).
			expr, err := filter.NewAttributeExpression(filter.AttributeReading, path)
			if err != nil {
				return fmt.Errorf("failed to create reading filter for %s: %w", path, err)
			}

			*primaryExprs = append(*primaryExprs, expr)
		}
	}

	return nil
}

// unitConfigBeside returns the path a unit configuration would have in the same
// directory as diffPath. Git reports paths with forward slashes on every
// platform, and the result is compared against a tree listing, so it is built
// in that form too.
func unitConfigBeside(diffPath string) string {
	return path.Join(path.Dir(diffPath), config.DefaultTerragruntConfigPath)
}

// recordDiffTelemetry records telemetry metrics for git diff results.
func recordDiffTelemetry(ctx context.Context, diffs *git.Diffs) {
	telemeter := telemetry.TelemeterFromContext(ctx)
	if telemeter == nil || telemeter.Meter == nil {
		return
	}

	telemeter.Count(ctx, "git_diff_files_added", int64(len(diffs.Added)))
	telemeter.Count(ctx, "git_diff_files_removed", int64(len(diffs.Removed)))
	telemeter.Count(ctx, "git_diff_files_changed", int64(len(diffs.Changed)))
}

// createGitWorktrees creates detached worktrees for each unique Git reference needed by filters.
// The worktrees are created in temporary directories and tracked in refsToPaths.
//
// A reference whose files can come from `git archive` is registered as a
// worktree without a checkout and filled from that archive, which reads the
// tree once and writes it with several workers. References are materialized
// concurrently, apart from the `git worktree add` that registers each one,
// since concurrent calls race on the repository's `.git/worktrees/` directory.
func createGitWorktrees(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	gitRunner *git.GitRunner,
	gitRefs []string,
	repoRemote, repoBranch, repoCommit string,
	experiments experiment.Experiments,
	pathspecs map[string][]string,
) (map[string]string, error) {
	refsToPaths := make(map[string]string, len(gitRefs))

	if len(gitRefs) == 0 {
		return refsToPaths, nil
	}

	var (
		mu         sync.Mutex
		registerMu sync.Mutex
		errs       []error
	)

	// Every reference materializing at once shares the filesystem worker
	// ceiling, so refs do not multiply into disk contention.
	writers := max(1, vfs.FSWorkersFor(v.FS, v.Platform.TempDir())/len(gitRefs))

	create := func() error {
		g, groupCtx := errgroup.WithContext(ctx)
		g.SetLimit(min(runtime.GOMAXPROCS(0), len(gitRefs)))

		for _, ref := range gitRefs {
			g.Go(func() error {
				dir, err := createGitWorktree(
					groupCtx, l, v, gitRunner, &registerMu,
					&worktreeOpts{
						ref:        ref,
						writers:    writers,
						repoRemote: repoRemote,
						repoBranch: repoBranch,
						repoCommit: repoCommit,
						pathspecs:  pathspecs[ref],
					},
				)

				mu.Lock()
				defer mu.Unlock()

				if err != nil {
					errs = append(errs, err)

					return nil
				}

				refsToPaths[ref] = dir

				l.Debugf("Created Git worktree for reference %s at %s", ref, dir)

				return nil
			})
		}

		return g.Wait()
	}

	if experiments.Evaluate(experiment.SlowTaskReporting) {
		if err := util.NotifyIfSlow(
			ctx,
			l,
			util.SpinnerWriter(v),
			time.Second,
			slowWorktreeMsg(gitRefs),
			create,
		); err != nil {
			errs = append(errs, err)
		}
	} else if err := create(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return refsToPaths, errors.Join(errs...)
	}

	return refsToPaths, nil
}

// slowWorktreeMsg returns the progress messages shown while worktrees are being
// created for gitRefs.
func slowWorktreeMsg(gitRefs []string) util.SlowNotifyMsg {
	if len(gitRefs) == 1 {
		return util.SlowNotifyMsg{
			Spinner: fmt.Sprintf("Creating Git worktree for reference %s...", gitRefs[0]),
			Done:    "Created Git worktree for reference " + gitRefs[0],
		}
	}

	return util.SlowNotifyMsg{
		Spinner: fmt.Sprintf("Creating Git worktrees for %d references...", len(gitRefs)),
		Done:    fmt.Sprintf("Created Git worktrees for %d references", len(gitRefs)),
	}
}

// worktreeOpts carries the per-reference parameters of a worktree creation.
type worktreeOpts struct {
	ref        string
	repoRemote string
	repoBranch string
	repoCommit string
	pathspecs  []string
	writers    int
}

// createGitWorktree materializes a single reference in a new temporary
// directory and returns the path to it. The directory is removed again if the
// reference cannot be materialized.
func createGitWorktree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	gitRunner *git.GitRunner,
	registerMu *sync.Mutex,
	opts *worktreeOpts,
) (string, error) {
	// `git archive` honors gitattributes that a checkout ignores, so a
	// reference with them is checked out in full.
	altering, err := gitRunner.HasArchiveAlteringAttributes(ctx, v, opts.ref)
	if err != nil {
		return "", fmt.Errorf("failed to read gitattributes for reference %s: %w", opts.ref, err)
	}

	if altering {
		// opts belongs to this reference alone, so clearing it here leaves the
		// other references as they were.
		opts.pathspecs = nil
	}

	tmpDir, err := worktreeTempDir(v, opts.ref)
	if err != nil {
		return "", err
	}

	err = filter.TraceGitWorktreeCreate(
		ctx, opts.ref, tmpDir, opts.repoRemote, opts.repoBranch, opts.repoCommit,
		func(ctx context.Context) error {
			return materializeGitWorktree(ctx, l, v, gitRunner, registerMu, tmpDir, opts, altering)
		})
	if err != nil {
		if cleanErr := v.FS.RemoveAll(tmpDir); cleanErr != nil {
			l.Warnf("failed to clean worktree directory %s: %v", tmpDir, cleanErr)
		}

		return "", fmt.Errorf("failed to create Git worktree for reference %s: %w", opts.ref, err)
	}

	return tmpDir, nil
}

// worktreeTempDir returns a new temporary directory to materialize a reference
// into.
func worktreeTempDir(v *venv.Venv, ref string) (string, error) {
	tmpDir, err := vfs.MkdirTemp(
		v.FS,
		v.Platform.TempDir(),
		"terragrunt-worktree-"+sanitizeRef(ref)+"-",
	)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory for worktree: %w", err)
	}

	// macOS will create the temporary directory with symlinks, so we need to evaluate them.
	tmpDir, err = vfs.EvalSymlinks(v.FS, tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate symlinks for temporary directory: %w", err)
	}

	return tmpDir, nil
}

// materializeGitWorktree registers dir as a worktree for the reference and puts
// the reference's files in it. A worktree registered but left unfilled is
// removed again.
func materializeGitWorktree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	gitRunner *git.GitRunner,
	registerMu *sync.Mutex,
	dir string,
	opts *worktreeOpts,
	altering bool,
) error {
	checkout := git.SkipCheckout
	if altering {
		checkout = git.CheckoutFiles
	}

	if err := registerWorktree(ctx, v, gitRunner, registerMu, dir, opts.ref, checkout); err != nil {
		return err
	}

	if altering {
		return nil
	}

	if err := fillGitWorktree(ctx, l, v, gitRunner, registerMu, dir, opts); err != nil {
		unregisterWorktree(ctx, l, gitRunner, registerMu, dir)

		return err
	}

	return nil
}

// fillGitWorktree puts the reference's files in dir, a worktree registered
// without a checkout.
func fillGitWorktree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	gitRunner *git.GitRunner,
	registerMu *sync.Mutex,
	dir string,
	opts *worktreeOpts,
) error {
	if err := extractGitWorktree(ctx, v, gitRunner, dir, opts); err != nil {
		if ctx.Err() != nil || isExtractionRefusal(err) {
			return err
		}

		l.Debugf("Extracting the archive of %s into %s failed, checking it out instead: %v", opts.ref, dir, err)

		if checkoutErr := recreateWorktreeWithCheckout(ctx, v, gitRunner, registerMu, dir, opts.ref); checkoutErr != nil {
			return errors.Join(err, checkoutErr)
		}

		return nil
	}

	if len(opts.pathspecs) > 0 {
		// A worktree with only some of the reference's paths has no index to
		// fill, since every path left out would read as a deletion.
		return nil
	}

	// The worktree was registered without a checkout, which leaves its index
	// empty. Filling the index from the reference makes the files that were
	// just written read as committed content rather than as deletions.
	return gitRunner.WithWorkDir(dir).ReadTree(ctx, "HEAD")
}

// registerWorktree runs the `git worktree add` that registers dir in the
// repository. The adds are serialized because concurrent ones race on the
// repository's `.git/worktrees/` directory.
func registerWorktree(
	ctx context.Context,
	v *venv.Venv,
	gitRunner *git.GitRunner,
	registerMu *sync.Mutex,
	dir, ref string,
	checkout git.WorktreeCheckout,
) error {
	registerMu.Lock()
	defer registerMu.Unlock()

	return gitRunner.CreateDetachedWorktree(ctx, v, dir, ref, checkout)
}

// unregisterWorktree removes the worktree registered at dir, logging a failure
// to do so. It holds registerMu because git deletes the repository's
// `.git/worktrees/` directory along with its last registration, while a
// concurrent `git worktree add` may be creating an entry in it.
func unregisterWorktree(
	ctx context.Context,
	l log.Logger,
	gitRunner *git.GitRunner,
	registerMu *sync.Mutex,
	dir string,
) {
	registerMu.Lock()
	defer registerMu.Unlock()

	if err := gitRunner.RemoveWorktree(ctx, dir); err != nil {
		l.Warnf("failed to remove Git worktree %s: %v", dir, err)
	}
}

// recreateWorktreeWithCheckout drops the worktree registered at dir without a
// checkout, files and all, and registers dir again with one. Both commands
// write to the repository's `.git/worktrees/`, so they run under the lock the
// adds take.
func recreateWorktreeWithCheckout(
	ctx context.Context,
	v *venv.Venv,
	gitRunner *git.GitRunner,
	registerMu *sync.Mutex,
	dir, ref string,
) error {
	registerMu.Lock()
	defer registerMu.Unlock()

	if err := gitRunner.RemoveWorktree(ctx, dir); err != nil {
		return err
	}

	return gitRunner.CreateDetachedWorktree(ctx, v, dir, ref, git.CheckoutFiles)
}

// isExtractionRefusal reports whether err is one of the refusals extraction
// raises on purpose. A checkout would materialize what they refuse, so they
// are never routed around the fallback.
func isExtractionRefusal(err error) bool {
	for _, refusal := range []error{
		vfs.ErrSymlinkEscapes,
		git.ErrArchiveEntryOutsideDest,
		git.ErrArchiveTooManyEntries,
		git.ErrArchiveTooDeep,
	} {
		if errors.Is(err, refusal) {
			return true
		}
	}

	return false
}

// extractGitWorktree streams the archive of ref into dir. Git writes the
// archive as it is read, so the two run together.
func extractGitWorktree(
	ctx context.Context,
	v *venv.Venv,
	gitRunner *git.GitRunner,
	dir string,
	opts *worktreeOpts,
) error {
	pr, pw := io.Pipe()

	g, groupCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		err := gitRunner.ArchiveTree(groupCtx, v, opts.ref, pw, opts.pathspecs...)

		// Closing carries the outcome to the reader, which would otherwise wait
		// on content that is not coming.
		return errors.Join(err, pw.CloseWithError(err))
	})

	g.Go(func() error {
		err := git.ExtractArchive(groupCtx, v, pr, dir, opts.writers)

		// Closing unblocks git if extraction stopped early, so the archive is
		// not left writing into a pipe nothing reads.
		return errors.Join(err, pr.CloseWithError(err))
	})

	return g.Wait()
}

// sanitizeRef sanitizes a Git reference string for use in file paths.
// It replaces invalid characters with underscores.
func sanitizeRef(ref string) string {
	result := strings.Builder{}

	for _, r := range ref {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' ||
			r == '_' {
			result.WriteRune(r)

			continue
		}

		result.WriteRune('_')
	}

	return result.String()
}
