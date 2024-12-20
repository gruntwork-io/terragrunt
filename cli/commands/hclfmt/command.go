// Package hclfmt provides the hclfmt command for formatting HCL files.
package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "hclfmt"

	FileFlagName       = "file"
	ExcludeDirFlagName = "exclude-dir"
	CheckFlagName      = "check"
	DiffFlagName       = "diff"
	StdinFlagName      = "stdin"

	TerragruntHCLFmtFileFlagName       = flags.DeprecatedFlagNamePrefix + CommandName + "-file"
	TerragruntHCLFmtExcludeDirFlagName = flags.DeprecatedFlagNamePrefix + CommandName + "-exclude-dir"
	TerragruntHCLFmtStdinFlagName      = flags.DeprecatedFlagNamePrefix + CommandName + "-stdin"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        FileFlagName,
			EnvVars:     flags.EnvVars(FileFlagName),
			Destination: &opts.HclFile,
			Usage:       "The path to a single hcl file that the hclfmt command should run on.",
		}, TerragruntHCLFmtFileFlagName),
		flags.SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        ExcludeDirFlagName,
			EnvVars:     flags.EnvVars(ExcludeDirFlagName),
			Destination: &opts.HclExclude,
			Usage:       "Skip HCL formatting in given directories.",
		}, TerragruntHCLFmtExcludeDirFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        CheckFlagName,
			EnvVars:     flags.EnvVars(CheckFlagName),
			Destination: &opts.Check,
			Usage:       "Enable check mode in the hclfmt command.",
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DiffFlagName,
			EnvVars:     flags.EnvVars(DiffFlagName),
			Destination: &opts.Diff,
			Usage:       "Print diff between original and modified file versions when running with 'hclfmt'.",
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        StdinFlagName,
			EnvVars:     flags.EnvVars(StdinFlagName),
			Destination: &opts.HclFromStdin,
			Usage:       "Format HCL from stdin and print result to stdout.",
		}, TerragruntHCLFmtStdinFlagName),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:                  NewFlags(opts).Sort(),
		DisallowUndefinedFlags: true,
		Action:                 func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
