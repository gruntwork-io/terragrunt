// Package hclfmt provides the hclfmt command for formatting HCL files.
package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "hclfmt"

	FileFlagName       = "file"
	ExcludeDirFlagName = "exclude-dir"
	CheckFlagName      = "check"
	DiffFlagName       = "diff"
	StdinFlagName      = "stdin"

	DeprecatedHclfmtFileFlagName        = "hclfmt-file"
	DeprecatedHclfmtcExcludeDirFlagName = "hclfmt-exclude-dir"
	DeprecatedHclfmtStdinFlagName       = "hclfmt-stdin"
	DeprecatedCheckFlagName             = "check"
	DeprecatedDiffFlagName              = "diff"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FileFlagName,
			EnvVars:     tgPrefix.EnvVars(FileFlagName),
			Destination: &opts.HclFile,
			Usage:       "The path to a single hcl file that the hclfmt command should run on.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedHclfmtFileFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        ExcludeDirFlagName,
			EnvVars:     tgPrefix.EnvVars(ExcludeDirFlagName),
			Destination: &opts.HclExclude,
			Usage:       "Skip HCL formatting in given directories.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedHclfmtcExcludeDirFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        CheckFlagName,
			EnvVars:     tgPrefix.EnvVars(CheckFlagName),
			Destination: &opts.Check,
			Usage:       "Enable check mode in the hclfmt command.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedCheckFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DiffFlagName,
			EnvVars:     tgPrefix.EnvVars(DiffFlagName),
			Destination: &opts.Diff,
			Usage:       "Print diff between original and modified file versions when running with 'hclfmt'.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedDiffFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        StdinFlagName,
			EnvVars:     tgPrefix.EnvVars(StdinFlagName),
			Destination: &opts.HclFromStdin,
			Usage:       "Format HCL from stdin and print result to stdout.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedHclfmtStdinFlagName), terragruntPrefixControl)),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	prefix := flags.Prefix{CommandName}

	cmd := &cli.Command{
		Name:                 CommandName,
		Usage:                "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:                NewFlags(opts, prefix),
		ErrorOnUndefinedFlag: true,
		Action:               func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}

	cmd = runall.WrapCommand(opts, cmd)
	cmd = graph.WrapCommand(opts, cmd)

	return cmd
}
