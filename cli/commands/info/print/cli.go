package print

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "print"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:      CommandName,
		Usage:     "Print out a short description of Terragrunt context.",
		UsageText: "terragrunt info print",
		Flags:     run.NewFlags(opts, nil),
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts)
		},
	}

	cmd = runall.WrapCommand(opts, cmd, run.Run)

	return cmd
}
