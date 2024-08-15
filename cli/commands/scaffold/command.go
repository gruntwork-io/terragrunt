// Package scaffold provides functionality to scaffold OpenTofu/Terraform modules
// based on boilerplate templates for Terragrunt.
package scaffold

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName is the name of the command.
	CommandName = "scaffold"

	// Var is the name of the flag to pass variables to the scaffolding process.
	Var = "var"

	// VarFile is the name of the flag to pass files with variables to the scaffolding process.
	VarFile = "var-file"
)

// NewFlags returns the flags for the command.
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
			Usage:       "Files with variables to be used in modules scaffolding.",
		},
	}
}

// NewCommand returns the command.
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
