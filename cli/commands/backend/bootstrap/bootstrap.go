// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// If --all is set, discover components and iterate
	if opts.RunAll {
		return runOnDiscoveredComponents(ctx, l, opts)
	}

	// Otherwise, run on single component
	return runBootstrap(ctx, l, opts)
}

func runBootstrap(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	remoteState, err := config.ParseRemoteState(ctx, l, opts)
	if err != nil || remoteState == nil {
		return err
	}

	return remoteState.Bootstrap(ctx, l, opts)
}

func runOnDiscoveredComponents(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// Create discovery
	d, err := discovery.NewForDiscoveryCommand(discovery.DiscoveryCommandOptions{
		WorkingDir:    opts.WorkingDir,
		FilterQueries: opts.FilterQueries,
		Experiments:   opts.Experiments,
		Hidden:        true,
		Dependencies:  false,
		External:      false,
		Exclude:       true,
		Include:       true,
	})
	if err != nil {
		return errors.New(err)
	}

	components, err := d.Discover(ctx, l, opts)
	if err != nil {
		return errors.New(err)
	}

	// Run the bootstrap logic on each component
	var errs []error

	for _, c := range components {
		if _, ok := c.(*component.Stack); ok {
			continue // Skip stacks
		}

		componentOpts := opts.Clone()
		componentOpts.WorkingDir = c.Path()

		// Determine config path for this component
		configFilename := config.DefaultTerragruntConfigPath
		if len(opts.TerragruntConfigPath) > 0 {
			configFilename = filepath.Base(opts.TerragruntConfigPath)
		}

		componentOpts.TerragruntConfigPath = filepath.Join(c.Path(), configFilename)

		// Run the bootstrap logic for this component
		if err := runBootstrap(ctx, l, componentOpts); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
