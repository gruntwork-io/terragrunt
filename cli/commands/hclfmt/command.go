package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "hclfmt"

	FlagNameTerragruntHCLFmt = "terragrunt-hclfmt-file"
	FlagNameTerragruntCheck  = "terragrunt-check"
	FlagNameTerragruntDiff   = "terragrunt-diff"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntHCLFmt,
			Destination: &opts.HclFile,
			Usage:       "The path to a single hcl file that the hclfmt command should run on.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntCheck,
			Destination: &opts.Check,
			EnvVar:      "TERRAGRUNT_CHECK",
			Usage:       "Enable check mode in the hclfmt command.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntDiff,
			Destination: &opts.Diff,
			EnvVar:      "TERRAGRUNT_DIFF",
			Usage:       "Print diff between original and modified file versions when running with 'hclfmt'.",
		},
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
