package discovery

import (
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
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
	walkBoundary := stackWalkBoundary(fsys, opts)

	if walkBoundary != "" {
		d = d.WithWalkRoot(walkBoundary)
	}

	// Set discoveryBoundary so dropOutsideBoundary prunes graph-traversed stacks.
	if opts.DiscoveryBoundary != "" {
		boundary, err := resolveDiscoveryBoundary(
			fsys,
			opts.WorkingDir,
			opts.DiscoveryBoundary,
			boundaryEnclosureOptional,
		)
		if err != nil {
			return nil, err
		}

		d = d.WithDiscoveryBoundary(boundary)
	}

	return d, nil
}

// stackWalkBoundary derives the filesystem walk root for stack generation.
// Inline graph boundaries take precedence over the flag. When multiple inline
// boundaries exist and do not nest, the walk root is not narrowed so all
// targets are reachable.
func stackWalkBoundary(fsys vfs.FS, opts StackGenerateOptions) string {
	dirs := opts.Filters.InlineGraphBoundaries()

	if len(dirs) > 0 {
		return resolveStackWalkRoot(fsys, opts.WorkingDir, dirs)
	}

	if opts.DiscoveryBoundary != "" {
		return resolveStackWalkRoot(fsys, opts.WorkingDir, []string{opts.DiscoveryBoundary})
	}

	return ""
}

// resolveStackWalkRoot resolves boundary directories to an effective walk root.
// When a single boundary falls inside the working directory, it becomes the walk
// root. When multiple disjoint boundaries exist, the walk root stays empty
// (meaning the working directory) so all targets remain reachable.
func resolveStackWalkRoot(fsys vfs.FS, workingDir string, dirs []string) string {
	if len(dirs) == 0 {
		return ""
	}

	first, err := resolveDiscoveryBoundary(fsys, workingDir, dirs[0], boundaryEnclosureOptional)
	if err != nil || !vfs.Within(fsys, workingDir, first) {
		return ""
	}

	// Single boundary: use it as the walk root.
	if len(dirs) == 1 {
		return first
	}

	// Multiple boundaries: use the first only when all others nest inside it.
	for _, dir := range dirs[1:] {
		resolved, err := resolveDiscoveryBoundary(fsys, workingDir, dir, boundaryEnclosureOptional)
		if err != nil || !vfs.Within(fsys, first, resolved) {
			return ""
		}
	}

	return first
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
