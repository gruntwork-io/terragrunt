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
	AllFlagName        = "all"
	OutDirFlagName     = "out-dir"
	JSONOutDirFlagName = "json-out-dir"

	DeprecatedOutDirFlagName     = "out-dir"
	DeprecatedJSONOutDirFlagName = "json-out-dir"
)

func NewFlags(opts *options.TerragruntOptions, commandName string, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, commandName)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        AllFlagName,
			EnvVars:     tgPrefix.EnvVars(AllFlagName),
			Destination: &opts.RunAll,
			Usage:       `Run the specified OpenTofu/Terraform command on the stack of units in the current directory.`,
			Action: func(_ *cli.Context, _ bool) error {
				if opts.Graph {
					return errors.New(new(common.AllGraphFlagsError))
				}

				return nil
			},
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutDirFlagName,
			EnvVars:     tgPrefix.EnvVars(OutDirFlagName),
			Destination: &opts.OutputFolder,
			Usage:       "Directory to store plan files.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedOutDirFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        JSONOutDirFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONOutDirFlagName),
			Destination: &opts.JSONOutputFolder,
			Usage:       "Directory to store json plan files.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedJSONOutDirFlagName), terragruntPrefixControl)),
	}
}

// WrapCommand appends flags to the given `cmd` and wraps its action.
func WrapCommand(opts *options.TerragruntOptions, cmd *cli.Command) *cli.Command {
	cmd = cmd.WrapAction(func(cliCtx *cli.Context, action cli.ActionFunc) error {
		if !opts.RunAll {
			return action(cliCtx)
		}

		opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
			cliCtx := cliCtx.WithValue(options.ContextKey, opts)
			return action(cliCtx)
		}

		return Run(cliCtx, opts.OptionsFromContext(cliCtx))
	})

	flags := append(cmd.Flags, NewFlags(opts, cmd.Name, nil)...)
	cmd.Flags = flags.Sort()

	return cmd
}
