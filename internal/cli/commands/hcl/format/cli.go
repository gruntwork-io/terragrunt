package format

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
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

func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	flagSet := clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        FileFlagName,
			EnvVars:     tgPrefix.EnvVars(FileFlagName),
			Destination: &opts.HclFile,
			Usage:       "The path to a single HCL file that the command should run on.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-file"), terragruntPrefixControl),         // `TG_HCLFMT_FILE`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("hclfmt-file"), terragruntPrefixControl), // `TERRAGRUNT_HCLFMT_FILE`
		),

		flags.NewFlag(&clihelper.SliceFlag[string]{
			Name:        ExcludeDirFlagName,
			EnvVars:     tgPrefix.EnvVars(ExcludeDirFlagName),
			Destination: &opts.HclExclude,
			Usage:       "Skip HCL formatting in given directories.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-exclude-dir"), terragruntPrefixControl),         // `TG_HCLFMT_EXCLUDE_DIR`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("hclfmt-exclude-dir"), terragruntPrefixControl), // `TERRAGRUNT_EXCLUDE_DIR`
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        CheckFlagName,
			EnvVars:     tgPrefix.EnvVars(CheckFlagName),
			Destination: &opts.Check,
			Usage:       "Return a status code of zero when all files are formatted correctly, and a status code of one when they aren't.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-check"), terragruntPrefixControl),  // `TG_HCLFMT_CHECK`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("check"), terragruntPrefixControl), // `TERRAGRUNT_CHECK`
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        DiffFlagName,
			EnvVars:     tgPrefix.EnvVars(DiffFlagName),
			Destination: &opts.Diff,
			Usage:       "Print diff between original and modified file versions.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclfmt-diff"), terragruntPrefixControl),  // `TG_HCLFMT_DIFF`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("diff"), terragruntPrefixControl), // `TERRAGRUNT_DIFF`
		),

		flags.NewFlag(&clihelper.BoolFlag{
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
	flagSet = flagSet.Add(shared.NewFilterFlags(l, opts)...)
	flagSet = flagSet.Add(shared.NewParallelismFlag(opts))

	return flagSet
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmd := &clihelper.Command{
		Name:    CommandName,
		Aliases: []string{CommandNameAlias},
		Usage:   "Recursively find HashiCorp Configuration Language (HCL) files and rewrite them into a canonical format.",
		Flags:   NewFlags(l, opts, nil),
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
