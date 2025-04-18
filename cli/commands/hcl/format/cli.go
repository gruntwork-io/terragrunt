package format

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName      = "format"
	CommandNameAlias = "fmt"

	FileFlagName       = "file"
	ExcludeDirFlagName = "exclude-dir"
	CheckFlagName      = "check"
	DiffFlagName       = "diff"
	StdinFlagName      = "stdin"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	flags := cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FileFlagName,
			EnvVars:     tgPrefix.EnvVars(FileFlagName),
			Destination: &opts.HclFile,
			Usage:       "The path to a single HCL file that the command should run on.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("hclfmt-file"), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        ExcludeDirFlagName,
			EnvVars:     tgPrefix.EnvVars(ExcludeDirFlagName),
			Destination: &opts.HclExclude,
			Usage:       "Skip HCL formatting in given directories.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("hclfmt-exclude-dir"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        CheckFlagName,
			EnvVars:     tgPrefix.EnvVars(CheckFlagName),
			Destination: &opts.Check,
			Usage:       "Return a status code of zero when all files are formatted correctly, and a status code of one when they aren't.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("check"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DiffFlagName,
			EnvVars:     tgPrefix.EnvVars(DiffFlagName),
			Destination: &opts.Diff,
			Usage:       "Print diff between original and modified file versions.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("diff"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        StdinFlagName,
			EnvVars:     tgPrefix.EnvVars(StdinFlagName),
			Destination: &opts.HclFromStdin,
			Usage:       "Format HCL from stdin and print result to stdout.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("hclfmt-stdin"), terragruntPrefixControl)),
	}

	return flags
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:                 CommandName,
		Aliases:              []string{CommandNameAlias},
		Usage:                "Recursively find HashiCorp Configuration Language (HCL) files and rewrite them into a canonical format.",
		Flags:                NewFlags(opts, nil),
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(opts, cmd)

	return cmd
}
