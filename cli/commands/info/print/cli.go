package print

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	runcmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "print"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:      CommandName,
		Usage:     "Print out a short description of Terragrunt context.",
		UsageText: "terragrunt info print",
		Flags:     runcmd.NewFlags(l, opts, nil),
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts)
		},
	}

	cmd = runall.WrapCommand(l, opts, cmd, run.Run, true)

	return cmd
}
