// Package scaffold provides the command to scaffold a new Terragrunt module.
package scaffold

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName   = "scaffold"
	Var           = "var"
	VarFile       = "var-file"
	NoIncludeRoot = "no-include-root"

	// This is being deprecated.
	RootFileName = "root-file-name"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.SliceFlag[string]{
			Name:        Var,
			Destination: &opts.ScaffoldVars,
			Usage:       "Variables for usage in scaffolding.",
		},
		&cli.SliceFlag[string]{
			Name:        VarFile,
			Destination: &opts.ScaffoldVarFiles,
			Usage:       "Files with variables to be used in unit scaffolding.",
		},
		&cli.BoolFlag{
			Name:        NoIncludeRoot,
			Destination: &opts.ScaffoldNoIncludeRoot,
			Usage:       "Do not include root unit in scaffolding.",
		},
		&cli.GenericFlag[string]{
			Name:        RootFileName,
			Destination: &opts.ScaffoldRootFileName,
			Usage:       "Name of the root Terragrunt configuration file, if used.",
			Action: func(ctx *cli.Context, value string) error {
				if value == config.DefaultTerragruntConfigPath {
					if control, ok := strict.GetStrictControl(strict.RootTerragruntHCL); ok {
						warn, triggered, err := control.Evaluate(opts)
						if err != nil {
							return err
						}

						if !triggered {
							opts.Logger.Warnf(warn)
						}
					}
				}

				return nil
			},
		},
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Scaffold a new Terragrunt module.",
		DisallowUndefinedFlags: true,
		Flags:                  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error {
			var moduleURL, templateURL string

			if val := ctx.Args().Get(0); val != "" {
				moduleURL = val
			}

			if val := ctx.Args().Get(1); val != "" {
				templateURL = val
			}

			return Run(ctx, opts.OptionsFromContext(ctx), moduleURL, templateURL)
		},
	}
}
