// Package graph implements the terragrunt dag graph command which generates a visual
// representation of the Terragrunt dependency graph in DOT language format.
//
// Alias for 'list --format=dot --dag --dependencies --external'.
package graph

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "graph"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	sharedFlags := shared.NewQueueFlags(opts, nil)
	sharedFlags = append(sharedFlags, shared.NewBackendFlags(opts, nil)...)
	sharedFlags = append(sharedFlags, shared.NewFeatureFlags(opts, nil)...)
	sharedFlags = append(sharedFlags, shared.NewFilterFlags(opts)...)

	return &cli.Command{
		Name:      CommandName,
		Usage:     "Graph the Directed Acyclic Graph (DAG) in DOT language. Alias for 'list --format=dot --dag --dependencies --external'.",
		UsageText: "terragrunt dag graph",
		Flags:     sharedFlags,
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts)
		},
	}
}

func Run(ctx *cli.Context, l log.Logger, opts *options.TerragruntOptions) error {
	listOpts := list.NewOptions(opts)
	listOpts.Format = list.FormatDot
	listOpts.Mode = list.ModeDAG
	listOpts.Dependencies = true
	listOpts.Hidden = true

	// By default, graph includes external dependencies.
	// Respect queue flags to override this behavior.
	if opts.IgnoreExternalDependencies {
		listOpts.External = false
	} else {
		// Default to true, or explicitly set if --queue-include-external is used
		listOpts.External = true
	}

	return list.Run(ctx, l, listOpts)
}
