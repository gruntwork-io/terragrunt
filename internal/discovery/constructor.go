package discovery

import (
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
	Hidden           bool
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

// NewForDiscoveryCommand creates a Discovery configured for discovery commands (find/list).
func NewForDiscoveryCommand(opts DiscoveryCommandOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir).
		WithSuppressParseErrors()

	if opts.Hidden {
		d = d.WithHidden()
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
			Cmd:  cmd,
			Args: args,
		})
	}

	if opts.Experiments.Evaluate(experiment.FilterFlag) && len(opts.FilterQueries) > 0 {
		filters, err := filter.ParseFilterQueries(opts.FilterQueries, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		d = d.WithFilters(filters)
	}

	return d, nil
}

// NewForHCLCommand creates a Discovery configured for HCL commands (hcl validate/format).
func NewForHCLCommand(opts HCLCommandOptions) (*Discovery, error) {
	d := NewDiscovery(opts.WorkingDir)

	if opts.Experiments.Evaluate(experiment.FilterFlag) && len(opts.FilterQueries) > 0 {
		filters, err := filter.ParseFilterQueries(opts.FilterQueries, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		d = d.WithFilters(filters)
	}

	return d, nil
}
