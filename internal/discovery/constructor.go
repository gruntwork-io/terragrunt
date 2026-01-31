package discovery

import (
	"path/filepath"
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mattn/go-shellwords"
)

// DiscoveryCommandOptions contains options for discovery commands like find and list.
type DiscoveryCommandOptions struct {
	WorkingDir        string
	QueueConstructAs  string
	FilterQueries     []string
	Experiments       experiment.Experiments
	NoHidden          bool
	Exclude           bool
	Include           bool
	Reading           bool
	WithRequiresParse bool
	WithRelationships bool
}

// HCLCommandOptions contains options for HCL commands like hcl validate & format.
type HCLCommandOptions struct {
	WorkingDir    string
	FilterQueries []string
	Experiments   experiment.Experiments
}

// StackGenerateOptions contains options for stack generate commands.
type StackGenerateOptions struct {
	WorkingDir    string
	FilterQueries []string
	Experiments   experiment.Experiments
}

// NewForDiscoveryCommand creates a Discovery configured for discovery commands (find/list).
func NewForDiscoveryCommand(l log.Logger, opts *DiscoveryCommandOptions) (*Discovery, error) {
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

	if opts.Reading {
		d = d.WithReadFiles()
	}

	if opts.QueueConstructAs != "" {
		d = d.WithParseExclude()

		parser := shellwords.NewParser()

		// Normalize Windows paths before parsing - shellwords treats backslashes as escape characters
		args, err := parser.Parse(filepath.ToSlash(opts.QueueConstructAs))
		if err != nil {
			return nil, err
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

	if len(opts.FilterQueries) > 0 {
		filters, err := filter.ParseFilterQueries(l, opts.FilterQueries)
		if err != nil {
			return nil, err
		}

		d = d.WithFilters(filters)
	}

	return d, nil
}

// NewForHCLCommand creates a Discovery configured for HCL commands (hcl validate/format).
func NewForHCLCommand(l log.Logger, opts HCLCommandOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir)

	if len(opts.FilterQueries) > 0 {
		filters, err := filter.ParseFilterQueries(l, opts.FilterQueries)
		if err != nil {
			return nil, err
		}

		d = d.WithFilters(filters)
	}

	return d, nil
}

// NewForStackGenerate creates a Discovery configured for `stack generate`.
func NewForStackGenerate(l log.Logger, opts StackGenerateOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir)

	if len(opts.FilterQueries) > 0 {
		filters, err := filter.ParseFilterQueries(l, opts.FilterQueries)
		if err != nil {
			return nil, err
		}

		d = d.WithFilters(filters.RestrictToStacks())
	}

	return d, nil
}

// NewDiscovery creates a new Discovery with sensible defaults.
func NewDiscovery(dir string) *Discovery {
	numWorkers := max(min(runtime.NumCPU(), maxDiscoveryWorkers), defaultDiscoveryWorkers)

	return &Discovery{
		numWorkers:         numWorkers,
		maxDependencyDepth: defaultMaxDependencyDepth,
		workingDir:         dir,
		discoveryContext: &component.DiscoveryContext{
			WorkingDir: dir,
		},
	}
}
