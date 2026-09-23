package discovery

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
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
	walkBoundary := StackWalkBoundary(fsys, opts)

	if walkBoundary != "" {
		d = d.WithWalkRoot(walkBoundary)
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

// StackWalkBoundary returns the outermost boundary when it lies inside the working directory, or "".
func StackWalkBoundary(fsys vfs.FS, opts StackGenerateOptions) string {
	root := outermostBoundary(fsys, opts)
	if root == "" || !vfs.Within(fsys, opts.WorkingDir, root) {
		return ""
	}

	return root
}

// WorktreeBoundary returns the outermost boundary relative to the Git root that worktrees mirror, or "" when unbounded.
func WorktreeBoundary(ctx context.Context, v *venv.Venv, opts StackGenerateOptions) string {
	root := outermostBoundary(v.FS, opts)
	if root == "" || isExternal(v.FS, root, opts.WorkingDir) && isExternal(v.FS, opts.WorkingDir, root) {
		return ""
	}

	gitRoot, err := git.GoRepoRoot(ctx, v, opts.WorkingDir)
	if err != nil || !vfs.Within(v.FS, gitRoot, root) {
		return ""
	}

	rel, err := filepath.Rel(vfs.ResolveForCompare(v.FS, gitRoot), vfs.ResolveForCompare(v.FS, root))
	if err != nil || rel == "." {
		return ""
	}

	return rel
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

// outermostBoundary returns the boundary enclosing every positive filter, or "" when unbounded or disjoint.
func outermostBoundary(fsys vfs.FS, opts StackGenerateOptions) string {
	var dirs []string

	for _, flt := range opts.Filters {
		if filter.IsPureNegation(flt.Expression()) {
			continue
		}

		bounds := filter.Filters{flt}.InlineGraphBoundaries()
		if len(bounds) == 0 {
			if opts.DiscoveryBoundary == "" {
				return ""
			}

			bounds = []string{opts.DiscoveryBoundary}
		}

		dirs = append(dirs, bounds...)
	}

	if len(dirs) == 0 && opts.DiscoveryBoundary != "" {
		dirs = []string{opts.DiscoveryBoundary}
	}

	var root string

	for _, dir := range dirs {
		resolved, err := resolveDiscoveryBoundary(fsys, opts.WorkingDir, dir, boundaryEnclosureOptional)
		if err != nil {
			return ""
		}

		switch {
		case root == "", vfs.Within(fsys, resolved, root):
			root = resolved
		case !vfs.Within(fsys, root, resolved):
			return ""
		}
	}

	return root
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
