// Package catalog provides the ability to interact with a catalog of OpenTofu/Terraform modules
// via the `terragrunt catalog` command.
package catalog

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "catalog"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.BoolFlag{
			Name:        scaffold.NoIncludeRoot,
			Destination: &opts.ScaffoldNoIncludeRoot,
			Usage:       "Do not include root unit in scaffolding done by catalog.",
		},
		&cli.GenericFlag[string]{
			Name:        scaffold.RootFileName,
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
		DisallowUndefinedFlags: true,
		Usage:                  "Launch the user interface for searching and managing your module catalog.",
		Action: func(ctx *cli.Context) error {
			var repoPath string

			if val := ctx.Args().Get(0); val != "" {
				repoPath = val
			}

			return Run(ctx, opts.OptionsFromContext(ctx), repoPath)
		},
	}
}
