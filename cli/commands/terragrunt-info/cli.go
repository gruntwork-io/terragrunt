// Package terragruntinfo provides the command to emit limited terragrunt state on stdout and exits.
package terragruntinfo

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "terragrunt-info"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:   CommandName,
		Flags:  run.NewFlags(opts, nil),
		Usage:  "Emits limited terragrunt state on stdout and exits.",
		Action: func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}

	cmd = runall.WrapCommand(opts, cmd, run.Run)
	cmd = graph.WrapCommand(opts, cmd, run.Run)

	return cmd
}
