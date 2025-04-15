// Package hclfmt provides the hclfmt command for formatting HCL files.
package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "hclfmt"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	prefix := flags.Prefix{CommandName}

	cmd := &cli.Command{
		Name:                 CommandName,
		Usage:                "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:                format.NewFlags(opts, prefix),
		ErrorOnUndefinedFlag: true,
		Action:               func(ctx *cli.Context) error { return format.Run(ctx, opts.OptionsFromContext(ctx)) },
	}

	cmd = runall.WrapCommand(opts, cmd)
	cmd = graph.WrapCommand(opts, cmd)

	return cmd
}
