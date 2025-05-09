// Package runall provides the feature that runs a terraform command
// against a 'stack' by running the specified command in each subfolder.
package runall

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/common"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	AllFlagName  = "all"
	AllFlagAlias = "a"
)

func NewFlags(opts *options.TerragruntOptions, commandName string) cli.Flags {
	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        AllFlagName,
			Aliases:     []string{AllFlagAlias},
			EnvVars:     flags.EnvVarsWithTgPrefix(AllFlagName),
			Destination: &opts.RunAll,
			Usage:       `Run the specified command on the stack of units in the current directory.`,
			Action: func(_ *cli.Context, _ bool) error {
				if opts.Graph {
					return errors.New(new(common.AllGraphFlagsError))
				}

				return nil
			},
		}),
	}
}

// WrapCommand appends flags to the given `cmd` and wraps its action.
func WrapCommand(opts *options.TerragruntOptions, cmd *cli.Command, runFn func(ctx context.Context, opts *options.TerragruntOptions) error) *cli.Command {
	cmd = cmd.WrapAction(func(cliCtx *cli.Context, action cli.ActionFunc) error {
		if !opts.RunAll {
			return action(cliCtx)
		}

		opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
			if opts.TerraformCommand == cmd.Name {
				cliCtx := cliCtx.WithValue(options.ContextKey, opts)

				return action(cliCtx)
			}

			return runFn(ctx, opts)
		}

		return Run(cliCtx, opts.OptionsFromContext(cliCtx))
	})

	cmd.Flags = append(cmd.Flags, NewFlags(opts, cmd.Name)...)

	return cmd
}
