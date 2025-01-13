// Package scaffold provides the command to scaffold a new Terragrunt module.
package scaffold

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "scaffold"

	RootFileNameFlagName  = "root-file-name"
	NoIncludeRootFlagName = "no-include-root"
	VarFlagName           = "var"
	VarFileFlagName       = "var-file"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.GenericFlag[string]{
			Name:        RootFileNameFlagName,
			Destination: &opts.ScaffoldRootFileName,
			Usage:       "Name of the root Terragrunt configuration file, if used.",
			Action: func(_ *cli.Context, value string) error {
				if value == "" {
					return errors.New("root-file-name flag cannot be empty")
				}

				if value != opts.TerragruntConfigPath {
					opts.ScaffoldRootFileName = value
				}

				if err := opts.StrictControls.Evaluate(opts.Logger, strict.RootTerragruntHCL); err != nil {
					return cli.NewExitError(err, cli.ExitCodeGeneralError)
				}

				return nil
			},
		},
		&cli.BoolFlag{
			Name:        NoIncludeRootFlagName,
			Destination: &opts.ScaffoldNoIncludeRoot,
			Usage:       "Do not include root unit in scaffolding done by catalog.",
		},
		&cli.SliceFlag[string]{
			Name:        VarFlagName,
			EnvVars:     flags.EnvVars(VarFlagName),
			Destination: &opts.ScaffoldVars,
			Usage:       "Variables for usage in scaffolding.",
		},
		&cli.SliceFlag[string]{
			Name:        VarFileFlagName,
			EnvVars:     flags.EnvVars(VarFileFlagName),
			Destination: &opts.ScaffoldVarFiles,
			Usage:       "Files with variables to be used in unit scaffolding.",
		},
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Scaffold a new Terragrunt module.",
		ErrorOnUndefinedFlag: true,
		Flags:                NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error {
			var moduleURL, templateURL string

			if val := ctx.Args().Get(0); val != "" {
				moduleURL = val
			}

			if val := ctx.Args().Get(1); val != "" {
				templateURL = val
			}

			if opts.ScaffoldRootFileName == "" {
				opts.ScaffoldRootFileName = GetDefaultRootFileName(opts)
			}

			return Run(ctx, opts.OptionsFromContext(ctx), moduleURL, templateURL)
		},
	}
}

func GetDefaultRootFileName(opts *options.TerragruntOptions) string {
	if err := opts.StrictControls.Evaluate(opts.Logger, strict.RootTerragruntHCL); err != nil {
		return config.RecommendedParentConfigName
	}

	return config.DefaultTerragruntConfigPath
}
