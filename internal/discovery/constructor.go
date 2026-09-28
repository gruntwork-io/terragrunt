package discovery

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DiscoveryCommandOptions contains options for discovery commands like find and list.
type DiscoveryCommandOptions struct {
	WorkingDir        string
	QueueConstructAs  string
	DiscoveryBoundary string
	Filters           filter.Filters
	NoHidden          bool
	Exclude           bool
	Include           bool
	Reading           bool
	TrackReads        bool
	ParseStackConfigs bool
	WithRequiresParse bool
	WithRelationships bool
}

// HCLCommandOptions contains options for HCL commands like hcl validate & format.
type HCLCommandOptions struct {
	WorkingDir        string
	DiscoveryBoundary string
	Filters           filter.Filters
}

// StackGenerateOptions contains options for stack generate commands.
type StackGenerateOptions struct {
	WorkingDir        string
	DiscoveryBoundary string
	Filters           filter.Filters
}

// NewForDiscoveryCommand creates a Discovery configured for discovery commands (find/list).
func NewForDiscoveryCommand(l log.Logger, fsys vfs.FS, opts *DiscoveryCommandOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir).
		WithSuppressParseErrors().
		WithBreakCycles()

	if opts.NoHidden {
		d = d.WithNoHidden()
	}

	if opts.WithRequiresParse {
		d = d.WithRequiresParse()
	}

	if opts.WithRelationships {
		d = d.WithRelationships()
	}

	if opts.Exclude {
		d = d.WithParseExclude()
	}

	if opts.Include {
		d = d.WithParseIncludes()
	}

	if opts.Reading {
		d = d.WithReadFiles()
	}

	if opts.TrackReads {
		d = d.WithTrackReads()
	}

	if opts.ParseStackConfigs {
		d = d.WithParseStackConfigs()
	}

	if opts.QueueConstructAs != "" {
		d = d.WithParseExclude()

		args, err := split.Command(opts.QueueConstructAs)
		if err != nil {
			return nil, err
		}

		if len(args) == 0 || args[0] == "" {
			return nil, NewEmptyQueueConstructAsError(opts.QueueConstructAs)
		}

		cmd := args[0]
		if len(args) > 1 {
			args = args[1:]
		} else {
			args = nil
		}

		d = d.WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: opts.WorkingDir,
			Cmd:        cmd,
			Args:       args,
		})
	}

	if len(opts.Filters) > 0 {
		d = d.WithFilters(opts.Filters)
	}

	if opts.DiscoveryBoundary != "" {
		if _, err := resolveDiscoveryBoundary(
			fsys,
			opts.WorkingDir,
			opts.DiscoveryBoundary,
			boundaryEnclosureFor(opts.Filters),
		); err != nil {
			return nil, err
		}

		d = d.WithDiscoveryBoundary(opts.DiscoveryBoundary)
	}

	return d, nil
}

// NewForHCLCommand creates a Discovery configured for HCL commands (hcl validate/format).
func NewForHCLCommand(l log.Logger, fsys vfs.FS, opts HCLCommandOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir)

	if len(opts.Filters) > 0 {
		d = d.WithFilters(opts.Filters)
	}

	if opts.DiscoveryBoundary != "" {
		if _, err := resolveDiscoveryBoundary(
			fsys,
			opts.WorkingDir,
			opts.DiscoveryBoundary,
			boundaryEnclosureFor(opts.Filters),
		); err != nil {
			return nil, err
		}

		d = d.WithDiscoveryBoundary(opts.DiscoveryBoundary)
	}

	return d, nil
}

// NewForStackGenerate creates a Discovery configured for `stack generate`.
// The walk root is narrowed to the effective boundary when it falls inside the
// working directory, so only stacks within the boundary are generated. Inline
// "(dir)" graph boundaries override the --discovery-boundary flag, matching the
// precedence used during filter evaluation.
func NewForStackGenerate(l log.Logger, fsys vfs.FS, opts StackGenerateOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir)

	if len(opts.Filters) > 0 {
		d = d.WithFilters(opts.Filters.RestrictToStacks())
	}

	// Inline "(dir)" operands override the flag, matching filter evaluation precedence.
	if walkRoot := StackWalkBoundary(l, fsys, opts); walkRoot != "" {
		d = d.WithWalkRoot(walkRoot)
	}

	// Set discoveryBoundary so dropOutsideBoundary prunes graph-traversed stacks.
	if opts.DiscoveryBoundary != "" {
		if _, err := resolveDiscoveryBoundary(
			fsys,
			opts.WorkingDir,
			opts.DiscoveryBoundary,
			boundaryEnclosureFor(opts.Filters),
		); err != nil {
			return nil, err
		}

		d = d.WithDiscoveryBoundary(opts.DiscoveryBoundary)
	}

	return d, nil
}

// StackWalkBoundary returns the outermost positive-filter boundary when it lies inside the working directory, or "".
func StackWalkBoundary(l log.Logger, fsys vfs.FS, opts StackGenerateOptions) string {
	dirs, ok := positiveBoundaryDirs(opts, false)
	if !ok {
		return ""
	}

	root := ""

	for _, dir := range dirs {
		resolved, err := resolveDiscoveryBoundary(fsys, opts.WorkingDir, dir, boundaryEnclosureOptional)
		if err != nil {
			l.Debugf("Discovery: cannot resolve boundary %s (%v); walking the whole working directory", dir, err)
			return ""
		}

		prev := root

		if root, ok = outermost(fsys, root, resolved); !ok {
			l.Debugf("Discovery: boundaries %s and %s do not nest; walking the whole working directory", prev, resolved)
			return ""
		}
	}

	if !vfs.Within(fsys, opts.WorkingDir, root) {
		l.Debugf("Discovery: boundary %s is not inside %s; walking the whole working directory", root, opts.WorkingDir)
		return ""
	}

	return root
}

// WorktreeBoundary returns the outermost positive-filter boundary relative to the Git root, or "" when unbounded.
func WorktreeBoundary(ctx context.Context, l log.Logger, v *venv.Venv, opts StackGenerateOptions) string {
	dirs, ok := positiveBoundaryDirs(opts, true)
	if !ok {
		return ""
	}

	gitRoot, err := git.GoRepoRoot(ctx, v, opts.WorkingDir)
	if err != nil {
		l.Debugf("Discovery: no Git root for %s (%v); not narrowing worktree discovery", opts.WorkingDir, err)
		return ""
	}

	root := ""

	for _, dir := range dirs {
		path := filter.WorktreeBoundaryPath(gitRoot, gitRoot, dir)
		prev := root

		if root, ok = outermost(v.FS, root, path); !ok {
			l.Debugf("Discovery: boundaries %s and %s do not nest; not narrowing worktree discovery", prev, path)
			return ""
		}
	}

	rel, err := filepath.Rel(vfs.ResolveForCompare(v.FS, gitRoot), vfs.ResolveForCompare(v.FS, root))
	if err != nil || rel == "." {
		l.Debugf("Discovery: boundary %s covers Git root %s; not narrowing worktree discovery", root, gitRoot)
		return ""
	}

	return rel
}

// CheckWorktreeBoundaries fails when a Git-targeted graph boundary is a directory at neither of its references.
func CheckWorktreeBoundaries(
	ctx context.Context,
	v *venv.Venv,
	w *worktrees.Worktrees,
	filters filter.Filters,
	flag, workingDir string,
) error {
	if w == nil || len(w.WorktreePairs) == 0 {
		return nil
	}

	checker := &boundaryChecker{v: v, w: w, trees: make(map[string]git.TreePaths)}
	checker.gitRoot, _ = git.GoRepoRoot(ctx, v, workingDir)

	var checkErr error

	for _, flt := range filters {
		filter.WalkExpressions(flt.Expression(), func(e filter.Expression) bool {
			g, ok := e.(*filter.GraphExpression)
			if !ok {
				return checkErr == nil
			}

			for _, bound := range []filter.GraphBound{g.Dependents, g.Dependencies} {
				boundary := bound.Boundary
				if boundary == "" {
					boundary = flag
				}

				if !bound.Include || boundary == "" {
					continue
				}

				filter.WalkExpressions(g.Target, func(target filter.Expression) bool {
					if gitExpr, ok := target.(*filter.GitExpression); ok && checkErr == nil {
						checkErr = checker.check(ctx, w.WorktreePairs[gitExpr.String()], boundary)
					}

					return checkErr == nil
				})
			}

			return checkErr == nil
		})
	}

	return checkErr
}

// WorktreeWalkRoot returns boundary mirrored into a worktree; ok is false when it is not a directory at that ref.
func WorktreeWalkRoot(fsys vfs.FS, worktreePath, boundary string) (string, bool, error) {
	root := filepath.Join(worktreePath, boundary)

	info, err := fsys.Stat(root)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return "", false, nil
	}

	if err != nil {
		return "", false, err
	}

	if !info.IsDir() {
		return "", false, nil
	}

	return root, true, nil
}

// WithinWorktreeBoundary reports whether a worktree component lies inside boundary, relative to its worktree root.
func WithinWorktreeBoundary(fsys vfs.FS, c component.Component, boundary string) bool {
	dc := c.DiscoveryContext()
	if boundary == "" || dc == nil || dc.WorkingDir == "" {
		return true
	}

	return vfs.Within(fsys, filepath.Join(dc.WorkingDir, boundary), c.Path())
}

// positiveBoundaryDirs returns the boundaries of every positive filter, and false when one is unbounded.
// In a Git worktree walk the flag only bounds filters that traverse dependents.
func positiveBoundaryDirs(opts StackGenerateOptions, worktreeWalk bool) ([]string, bool) {
	var dirs []string

	for _, flt := range opts.Filters {
		if filter.IsPureNegation(flt.Expression()) {
			continue
		}

		bounds := filter.Filters{flt}.InlineDependentBoundaries()
		if len(bounds) == 0 {
			if opts.DiscoveryBoundary == "" || (worktreeWalk && !flt.HasDependents()) {
				return nil, false
			}

			bounds = []string{opts.DiscoveryBoundary}
		}

		dirs = append(dirs, bounds...)
	}

	if len(dirs) == 0 {
		if opts.DiscoveryBoundary == "" {
			return nil, false
		}

		dirs = []string{opts.DiscoveryBoundary}
	}

	return dirs, true
}

// relOrEmpty returns path relative to base, or "" when no relative path exists, as across Windows volumes.
func relOrEmpty(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return ""
	}

	return rel
}

// outermost returns whichever of root and dir encloses the other, and false when they do not nest.
func outermost(fsys vfs.FS, root, dir string) (string, bool) {
	switch {
	case root == "", vfs.Within(fsys, dir, root):
		return dir, true
	case vfs.Within(fsys, root, dir):
		return root, true
	default:
		return "", false
	}
}

// NewDiscovery creates a new Discovery with sensible defaults.
func NewDiscovery(dir string) *Discovery {
	// Clamp worker count between defaultDiscoveryWorkers and maxDiscoveryWorkers, bounded by available CPUs.
	numWorkers := max(min(runtime.GOMAXPROCS(0), maxDiscoveryWorkers), defaultDiscoveryWorkers)

	return &Discovery{
		numWorkers:         numWorkers,
		maxDependencyDepth: defaultMaxDependencyDepth,
		workingDir:         dir,
		configFilenames:    DefaultConfigFilenames,
		discoveryContext: &component.DiscoveryContext{
			WorkingDir: dir,
		},
	}
}

// boundaryChecker looks a boundary up in each compared reference, reading the Git tree of a reference not checked out.
type boundaryChecker struct {
	v       *venv.Venv
	w       *worktrees.Worktrees
	trees   map[string]git.TreePaths
	gitRoot string
}

// check fails when boundary is a directory at neither reference of pair; one covering the repository always passes.
func (b *boundaryChecker) check(ctx context.Context, pair *worktrees.WorktreePair, boundary string) error {
	if pair == nil {
		return nil
	}

	base := b.gitRoot
	if base == "" {
		base = string(filepath.Separator)
	}

	rel := relOrEmpty(base, filter.WorktreeBoundaryPath(base, b.gitRoot, boundary))
	if rel == "" || rel == "." {
		return nil
	}

	refs := []worktrees.Worktree{pair.FromWorktree, pair.ToWorktree}

	// Checked-out references answer first, so Git is only asked about a reference not fully on disk.
	for _, wt := range refs {
		if wt.Path == "" {
			continue
		}

		if _, ok, err := WorktreeWalkRoot(b.v.FS, wt.Path, rel); err != nil || ok {
			return err
		}
	}

	for _, wt := range refs {
		if wt.Ref == "" {
			continue
		}

		if ok, err := b.inTree(ctx, wt.Ref, rel); err != nil || ok {
			return err
		}
	}

	return NewDiscoveryBoundaryDirError(
		boundary,
		errors.New("not a directory at either compared reference, resolved against the repository root"),
	)
}

// inTree reports whether rel is a directory in the Git tree of ref, meaning the tree holds a file under it.
func (b *boundaryChecker) inTree(ctx context.Context, ref, rel string) (bool, error) {
	tree, ok := b.trees[ref]
	if !ok {
		runner, err := git.NewGitRunner(b.v)
		if err != nil {
			return false, err
		}

		if tree, err = runner.WithWorkDir(b.w.OriginalWorkingDir).LsTreeNames(ctx, b.v, ref); err != nil {
			return false, err
		}

		b.trees[ref] = tree
	}

	prefix := filepath.ToSlash(rel) + "/"

	for path := range tree {
		if strings.HasPrefix(path, prefix) {
			return true, nil
		}
	}

	return false, nil
}
