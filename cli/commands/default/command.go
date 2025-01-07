// Package defaultcmd represents the default CLI command.
package defaultcmd

import (
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"

	runCmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
)

const (
	CommandName     = ""
	CommandHelpName = "*"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:     CommandName,
		HelpName: CommandHelpName,
		Usage:    "Terragrunt forwards all other commands directly to OpenTofu/Terraform",
		Flags:    runCmd.NewFlags(opts),
		Action:   Action(opts),
	}
}

func Action(opts *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if control, ok := strict.GetStrictControl(strict.DefaultCommand); ok {
			warn, triggered, err := control.Evaluate(opts)
			if err != nil {
				return err
			}

			if !triggered {
				opts.Logger.Warnf(warn)
			}
		}

		return runCmd.Action(opts)(ctx)
	}
}
