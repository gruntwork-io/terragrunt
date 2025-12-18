package print

import (
	runcmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "print"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmdFlags := runcmd.NewFlags(l, opts, nil)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil))

	cmd := &cli.Command{
		Name:      CommandName,
		Usage:     "Print out a short description of Terragrunt context.",
		UsageText: "terragrunt info print",
		Flags:     cmdFlags,
		Action: func(ctx *cli.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)
			tgOpts.SummaryDisable = true

			if tgOpts.RunAll {
				return runall.Run(ctx.Context, l, tgOpts)
			}

			return Run(ctx, l, opts)
		},
	}

	return cmd
}
