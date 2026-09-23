package discovery

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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
		boundary, err := resolveDiscoveryBoundary(
			fsys,
			opts.WorkingDir,
			opts.DiscoveryBoundary,
			boundaryEnclosureFor(opts.Filters),
		)
		if err != nil {
			return nil, err
		}

		d = d.WithDiscoveryBoundary(boundary)
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
		boundary, err := resolveDiscoveryBoundary(
			fsys,
			opts.WorkingDir,
			opts.DiscoveryBoundary,
			boundaryEnclosureFor(opts.Filters),
		)
		if err != nil {
			return nil, err
		}

		d = d.WithDiscoveryBoundary(boundary)
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
	if walkRoots := StackWalkRoots(fsys, opts); len(walkRoots) > 0 {
		d = d.WithWalkRoots(walkRoots)
	}

	// Set discoveryBoundary so dropOutsideBoundary prunes graph-traversed stacks.
	if opts.DiscoveryBoundary != "" {
		boundary, err := resolveDiscoveryBoundary(
			fsys,
			opts.WorkingDir,
			opts.DiscoveryBoundary,
			boundaryEnclosureFor(opts.Filters),
		)
		if err != nil {
			return nil, err
		}

		d = d.WithDiscoveryBoundary(boundary)
	}

	return d, nil
}

// StackWalkRoots returns the positive filters' boundaries when all lie inside the working directory, or nil.
func StackWalkRoots(fsys vfs.FS, opts StackGenerateOptions) []string {
	roots, ok := boundaryRoots(fsys, opts)
	if !ok || slices.ContainsFunc(roots, func(root string) bool { return !vfs.Within(fsys, opts.WorkingDir, root) }) {
		return nil
	}

	return roots
}

// WorktreeBoundaries returns the positive filters' disjoint boundaries relative to the Git root, or nil if unbounded.
func WorktreeBoundaries(ctx context.Context, v *venv.Venv, opts StackGenerateOptions) []string {
	roots, ok := boundaryRoots(v.FS, opts)
	if !ok {
		return nil
	}

	gitRoot, err := git.GoRepoRoot(ctx, v, opts.WorkingDir)
	if err != nil {
		return nil
	}

	for _, root := range roots {
		if !vfs.Within(v.FS, gitRoot, root) ||
			isExternal(v.FS, root, opts.WorkingDir) && isExternal(v.FS, opts.WorkingDir, root) {
			return nil
		}
	}

	rels := make([]string, 0, len(roots))

	for _, root := range roots {
		rel, err := filepath.Rel(vfs.ResolveForCompare(v.FS, gitRoot), vfs.ResolveForCompare(v.FS, root))
		if err != nil || rel == "." {
			return nil
		}

		rels = append(rels, rel)
	}

	slices.Sort(rels)

	return rels
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

// WorktreeWalkRoots mirrors boundaries into a worktree, skipping those absent at its ref; no boundaries yield its root.
func WorktreeWalkRoots(fsys vfs.FS, worktreePath string, boundaries []string) ([]string, error) {
	if len(boundaries) == 0 {
		return []string{worktreePath}, nil
	}

	var roots []string

	for _, boundary := range boundaries {
		root, ok, err := WorktreeWalkRoot(fsys, worktreePath, boundary)
		if err != nil {
			return nil, err
		}

		if ok {
			roots = append(roots, root)
		}
	}

	return roots, nil
}

// WithinWorktreeBoundary reports whether a worktree component lies inside any boundary, relative to its worktree root.
func WithinWorktreeBoundary(fsys vfs.FS, c component.Component, boundaries []string) bool {
	dc := c.DiscoveryContext()
	if len(boundaries) == 0 || dc == nil || dc.WorkingDir == "" {
		return true
	}

	return slices.ContainsFunc(boundaries, func(boundary string) bool {
		return vfs.Within(fsys, filepath.Join(dc.WorkingDir, boundary), c.Path())
	})
}

// boundaryRoots returns the resolved, outermost boundaries of all positive filters, and false when one is unbounded.
func boundaryRoots(fsys vfs.FS, opts StackGenerateOptions) ([]string, bool) {
	dirs, ok := positiveBoundaryDirs(opts)
	if !ok {
		return nil, false
	}

	var roots []string

	for _, dir := range dirs {
		resolved, err := resolveDiscoveryBoundary(fsys, opts.WorkingDir, dir, boundaryEnclosureOptional)
		if err != nil {
			return nil, false
		}

		roots = appendOutermost(fsys, roots, resolved)
	}

	return roots, true
}

// positiveBoundaryDirs returns the boundaries of every positive filter, and false when one is unbounded.
func positiveBoundaryDirs(opts StackGenerateOptions) ([]string, bool) {
	var dirs []string

	for _, flt := range opts.Filters {
		if filter.IsPureNegation(flt.Expression()) {
			continue
		}

		bounds := filter.Filters{flt}.InlineGraphBoundaries()
		if len(bounds) == 0 {
			if opts.DiscoveryBoundary == "" {
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

// appendOutermost adds dir to roots unless a root encloses it, dropping the roots it encloses.
func appendOutermost(fsys vfs.FS, roots []string, dir string) []string {
	if slices.ContainsFunc(roots, func(root string) bool { return vfs.Within(fsys, root, dir) }) {
		return roots
	}

	kept := make([]string, 0, len(roots)+1)

	for _, root := range roots {
		if !vfs.Within(fsys, dir, root) {
			kept = append(kept, root)
		}
	}

	return append(kept, dir)
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
