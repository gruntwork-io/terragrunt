package graph

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "graph"
)

func NewListFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	return run.NewFlags(opts, prefix)
}

func NewCommand(opts *options.TerragruntOptions, prefix flags.Prefix) *cli.Command {
	prefix = prefix.Append(CommandName)

	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Graph the Directed Acyclic Graph (DAG) in DOT language.",
		UsageText:            "terragrunt dag graph",
		Flags:                NewListFlags(opts, prefix),
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return nil
		},
	}
}
