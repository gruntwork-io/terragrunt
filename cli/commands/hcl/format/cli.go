package format

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
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

func NewFlags(opts *options.TerragruntOptions, cmdPrefix flags.Name) cli.Flags {
	strictControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	flags := cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FileFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(FileFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(FileFlagName),
			Destination: &opts.HclFile,
			Usage:       "The path to a single HCL file that the command should run on.",
		},
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("hclfmt-file"), strictControl),         // `TG_HCLFMT_FILE`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("hclfmt-file"), strictControl), // `--terragrunt-hclfmt-file`, `TERRAGRUNT_HCLFMT_FILE`
		),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        ExcludeDirFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ExcludeDirFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(ExcludeDirFlagName),
			Destination: &opts.HclExclude,
			Usage:       "Skip HCL formatting in given directories.",
		},
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("hclfmt-exclude-dir"), strictControl),         // `TG_HCLFMT_EXCLUDE_DIR`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("hclfmt-exclude-dir"), strictControl), // `--terragrunt-hclfmt-exclude-dir`, `TERRAGRUNT_EXCLUDE_DIR`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        CheckFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(CheckFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(CheckFlagName),
			Destination: &opts.Check,
			Usage:       "Return a status code of zero when all files are formatted correctly, and a status code of one when they aren't.",
		},
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("hclfmt-check"), strictControl),  // `TG_HCLFMT_CHECK`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("check"), strictControl), // `--terragrunt-check`, `TERRAGRUNT_CHECK`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DiffFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DiffFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(DiffFlagName),
			Destination: &opts.Diff,
			Usage:       "Print diff between original and modified file versions.",
		},
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("hclfmt-diff"), strictControl),  // `TG_HCLFMT_DIFF`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("diff"), strictControl), // `--terragrunt-diff`, `TERRAGRUNT_DIFF`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        StdinFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(StdinFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(StdinFlagName),
			Destination: &opts.HclFromStdin,
			Usage:       "Format HCL from stdin and print result to stdout.",
		},
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("hclfmt-stdin"), strictControl),         // `TG_HCLFMT_STDIN`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("hclfmt-stdin"), strictControl), // `--terragrunt-hclfmt-stdin`, `TERRAGRUNT_HCLFMT_STDIN`
		),
	}

	return flags
}

func NewCommand(opts *options.TerragruntOptions, cmdPrefix flags.Name) *cli.Command {
	cmdPrefix = cmdPrefix.Append(CommandName)

	cmd := &cli.Command{
		Name:    CommandName,
		Aliases: []string{CommandNameAlias},
		Usage:   "Recursively find HashiCorp Configuration Language (HCL) files and rewrite them into a canonical format.",
		Flags:   NewFlags(opts, cmdPrefix),
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(opts, cmd, run.Run)

	return cmd
}
