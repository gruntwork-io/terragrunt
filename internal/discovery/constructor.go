package discovery

import (
	"path/filepath"
	"runtime"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/mattn/go-shellwords"
)

// DiscoveryCommandOptions contains options for discovery commands like find and list.
type DiscoveryCommandOptions struct {
	WorkingDir       string
	QueueConstructAs string
	FilterQueries    []string
	Experiments      experiment.Experiments
	NoHidden         bool
	Dependencies     bool
	External         bool
	Exclude          bool
	Include          bool
	Reading          bool
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
func NewForDiscoveryCommand(opts DiscoveryCommandOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir).
		WithSuppressParseErrors().
		WithBreakCycles()

	if opts.NoHidden {
		d = d.WithNoHidden()
	}

	if opts.Dependencies || opts.External {
		d = d.WithDiscoverDependencies()
	}

	if opts.External {
		d = d.WithDiscoverExternalDependencies()
	}

	if opts.Exclude {
		d = d.WithParseExclude()
	}

	if opts.Include {
		d = d.WithParseInclude()
	}

	if opts.Reading {
		d = d.WithReadFiles()
	}

	if opts.QueueConstructAs != "" {
		d = d.WithParseExclude()
		d = d.WithDiscoverDependencies()

		parser := shellwords.NewParser()

		args, err := parser.Parse(opts.QueueConstructAs)
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

	if opts.Experiments.Evaluate(experiment.FilterFlag) {
		d = d.WithFilterFlagEnabled()

		if len(opts.FilterQueries) > 0 {
			filters, err := filter.ParseFilterQueries(opts.FilterQueries)
			if err != nil {
				return nil, err
			}

			d = d.WithFilters(filters)
		}
	}

	return d, nil
}

// NewForHCLCommand creates a Discovery configured for HCL commands (hcl validate/format).
func NewForHCLCommand(opts HCLCommandOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir)

	if opts.Experiments.Evaluate(experiment.FilterFlag) {
		d = d.WithFilterFlagEnabled()

		if len(opts.FilterQueries) > 0 {
			filters, err := filter.ParseFilterQueries(opts.FilterQueries)
			if err != nil {
				return nil, err
			}

			d = d.WithFilters(filters)
		}
	}

	return d, nil
}

// NewForStackGenerate creates a Discovery configured for `stack generate`.
func NewForStackGenerate(opts StackGenerateOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir)

	if opts.Experiments.Evaluate(experiment.FilterFlag) {
		d = d.WithFilterFlagEnabled()

		if len(opts.FilterQueries) > 0 {
			filters, err := filter.ParseFilterQueries(opts.FilterQueries)
			if err != nil {
				return nil, err
			}

			d = d.WithFilters(filters.RestrictToStacks())
		}
	}

	return d, nil
}

// NewDiscovery creates a new Discovery.
func NewDiscovery(dir string, opts ...DiscoveryOption) *Discovery {
	numWorkers := max(min(runtime.NumCPU(), maxDiscoveryWorkers), defaultDiscoveryWorkers)

	discovery := &Discovery{
		includeDirs: []string{
			config.StackDir,
			filepath.Join(config.StackDir, "**"),
		},
		numWorkers:         numWorkers,
		useDefaultExcludes: true,
		maxDependencyDepth: defaultMaxDependencyDepth,
		discoveryContext: &component.DiscoveryContext{
			WorkingDir: dir,
		},
	}

	for _, opt := range opts {
		opt(discovery)
	}

	return discovery
}
