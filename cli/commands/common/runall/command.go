// Package runall provides the feature that runs a terraform command
// against a 'stack' by running the specified command in each subfolder.
package runall

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	AllFlagName        = "all"
	OutDirFlagName     = "out-dir"
	JSONOutDirFlagName = "json-out-dir"

	DeprecatedOutDirFlagName     = "out-dir"
	DeprecatedJSONOutDirFlagName = "json-out-dir"
)

func NewFlags(opts *options.TerragruntOptions, commandName string, allFlag *bool, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, commandName)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        AllFlagName,
			EnvVars:     tgPrefix.EnvVars(AllFlagName),
			Destination: allFlag,
			Usage:       `Run the specified OpenTofu/Terraform command on the stack of units in the current directory.`,
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
	var allFlag bool

	cmd = cmd.WrapAction(func(cliCtx *cli.Context, action cli.ActionFunc) error {
		if !allFlag {
			return action(cliCtx)
		}

		opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
			cliCtx := cliCtx.WithValue(options.ContextKey, opts)
			return action(cliCtx)
		}

		return Run(cliCtx, opts.OptionsFromContext(cliCtx))
	})

	flags := append(cmd.Flags, NewFlags(opts, cmd.Name, &allFlag, nil)...)
	cmd.Flags = flags.Sort()

	return cmd
}
