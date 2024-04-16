package scaffold

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "scaffold"
	Var         = "var"
	VarFile     = "var-file"
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
			Usage:       "Files with variables to be used in modules scaffolding.",
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
			var moduleUrl, templateUrl string

			if val := ctx.Args().Get(0); val != "" {
				moduleUrl = val
			}

			if val := ctx.Args().Get(1); val != "" {
				templateUrl = val
			}

			return Run(ctx, opts.OptionsFromContext(ctx), moduleUrl, templateUrl)
		},
	}
}
