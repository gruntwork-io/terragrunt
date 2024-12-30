package commands

import (
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"

	runCmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
)

const DefaultCommandName = "*"

func NewDefaultCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := runCmd.NewCommand(opts)
	cmd.Name = DefaultCommandName
	cmd.Hidden = true
	cmd.ErrorOnUndefinedFlag = false

	action := cmd.Action
	cmd.Action = func(ctx *cli.Context) error {
		if control, ok := strict.GetStrictControl(strict.DefaultCommand); ok {
			warn, triggered, err := control.Evaluate(opts)
			if err != nil {
				return err
			}

			if !triggered {
				opts.Logger.Warnf(warn)
			}
		}

		return action(ctx)
	}

	return cmd
}
