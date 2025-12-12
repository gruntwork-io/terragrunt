package format

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

	flagSet := cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FileFlagName,
			EnvVars:     tgPrefix.EnvVars(FileFlagName),
			Destination: &opts.HclFile,
			Usage:       "The path to a single HCL file that the command should run on.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-file"), terragruntPrefixControl),         // `TG_HCLFMT_FILE`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("hclfmt-file"), terragruntPrefixControl), // `TERRAGRUNT_HCLFMT_FILE`
		),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        ExcludeDirFlagName,
			EnvVars:     tgPrefix.EnvVars(ExcludeDirFlagName),
			Destination: &opts.HclExclude,
			Usage:       "Skip HCL formatting in given directories.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-exclude-dir"), terragruntPrefixControl),         // `TG_HCLFMT_EXCLUDE_DIR`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("hclfmt-exclude-dir"), terragruntPrefixControl), // `TERRAGRUNT_EXCLUDE_DIR`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        CheckFlagName,
			EnvVars:     tgPrefix.EnvVars(CheckFlagName),
			Destination: &opts.Check,
			Usage:       "Return a status code of zero when all files are formatted correctly, and a status code of one when they aren't.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-check"), terragruntPrefixControl),  // `TG_HCLFMT_CHECK`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("check"), terragruntPrefixControl), // `TERRAGRUNT_CHECK`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DiffFlagName,
			EnvVars:     tgPrefix.EnvVars(DiffFlagName),
			Destination: &opts.Diff,
			Usage:       "Print diff between original and modified file versions.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-diff"), terragruntPrefixControl),  // `TG_HCLFMT_DIFF`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("diff"), terragruntPrefixControl), // `TERRAGRUNT_DIFF`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        StdinFlagName,
			EnvVars:     tgPrefix.EnvVars(StdinFlagName),
			Destination: &opts.HclFromStdin,
			Usage:       "Format HCL from stdin and print result to stdout.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-stdin"), terragruntPrefixControl),         // `TG_HCLFMT_STDIN`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("hclfmt-stdin"), terragruntPrefixControl), // `TERRAGRUNT_HCLFMT_STDIN`
		),
	}

	flagSet = flagSet.Add(shared.NewQueueFlags(opts, nil)...)
	flagSet = flagSet.Add(shared.NewFilterFlags(opts)...)
	flagSet = flagSet.Add(shared.NewParallelismFlag(opts))

	return flagSet
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:    CommandName,
		Aliases: []string{CommandNameAlias},
		Usage:   "Recursively find HashiCorp Configuration Language (HCL) files and rewrite them into a canonical format.",
		Flags:   NewFlags(opts, nil),
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(l, opts, cmd, run.Run, true)

	return cmd
}
