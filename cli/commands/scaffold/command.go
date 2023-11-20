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
			Usage:       "Boilerplate variables variable.",
		},
		&cli.SliceFlag[string]{
			Name:        VarFile,
			Destination: &opts.ScaffoldVarFiles,
			Usage:       "Boilerplate variables file.",
		},
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Scaffold a new Terragrunt module.",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
