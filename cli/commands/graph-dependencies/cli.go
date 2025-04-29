// Package graphdependencies provides the command to print the terragrunt dependency graph to stdout.
// Deprecated: This package is deprecated and will be removed in a future version.
// Use the 'terragrunt dag graph' command instead.
package graphdependencies

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "graph-dependencies"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:   CommandName,
		Flags:  run.NewFlags(opts, nil),
		Usage:  "Prints the terragrunt dependency graph to stdout.",
		Action: func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}

	cmd = runall.WrapCommand(opts, cmd, run.Run)
	cmd = graph.WrapCommand(opts, cmd, run.Run)

	return cmd
}
