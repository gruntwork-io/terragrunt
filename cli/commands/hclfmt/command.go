// Package hclfmt provides the hclfmt command for formatting HCL files.
package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "hclfmt"

	HCLFmtFlagName           = "hclfmt-file"
	HCLFmtExcludeDirFlagName = "hclfmt-exclude-dir"
	CheckFlagName            = "check"
	DiffFlagName             = "diff"
	HCLFmtStdinFlagName      = "hclfmt-stdin"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		flags.NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        HCLFmtFlagName,
			Destination: &opts.HclFile,
			Usage:       "The path to a single hcl file that the hclfmt command should run on.",
		}),
		flags.NewSliceFlag(opts, &cli.SliceFlag[string]{
			Name:        HCLFmtExcludeDirFlagName,
			Destination: &opts.HclExclude,
			Usage:       "Skip HCL formatting in given directories.",
		}),
		flags.NewBoolFlag(opts, &cli.BoolFlag{
			Name:        CheckFlagName,
			Destination: &opts.Check,
			Usage:       "Enable check mode in the hclfmt command.",
		}),
		flags.NewBoolFlag(opts, &cli.BoolFlag{
			Name:        DiffFlagName,
			Destination: &opts.Diff,
			Usage:       "Print diff between original and modified file versions when running with 'hclfmt'.",
		}),
		flags.NewBoolFlag(opts, &cli.BoolFlag{
			Name:        HCLFmtStdinFlagName,
			Destination: &opts.HclFromStdin,
			Usage:       "Format HCL from stdin and print result to stdout.",
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:  append(flags.NewCommonFlags(opts), NewFlags(opts)...).Sort(),
		Action: func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
