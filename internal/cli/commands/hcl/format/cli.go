package format

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt hcl format` command.
	CommandName = "format"
	// CommandNameAlias is the short alias of `terragrunt hcl format`.
	CommandNameAlias = "fmt"

	// FileFlagName is the name of the flag that formats one file in place of
	// the whole working directory tree.
	FileFlagName = "file"
	// ExcludeDirFlagName is the name of the flag that skips the given
	// directories.
	ExcludeDirFlagName = "exclude-dir"
	// CheckFlagName is the name of the flag that fails on an unformatted file
	// without rewriting it.
	CheckFlagName = "check"
	// DiffFlagName is the name of the flag that prints a diff for each file
	// that formatting changes.
	DiffFlagName = "diff"
	// StdinFlagName is the name of the flag that formats HCL read from stdin and
	// writes the result to stdout.
	StdinFlagName = "stdin"
)

// NewFlags returns the flags of `terragrunt hcl format`, bound to opts.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	flagSet := clihelper.Flags{
		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        FileFlagName,
				EnvVars:     tgPrefix.EnvVars(FileFlagName),
				Destination: &opts.HclFile,
				Usage:       "The path to a single HCL file that the command should run on.",
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("hclfmt-file"),
				opts.StrictControls,
			), // `TG_HCLFMT_FILE`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("hclfmt-file"),
				opts.StrictControls,
			), // `TERRAGRUNT_HCLFMT_FILE`
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:        ExcludeDirFlagName,
				EnvVars:     tgPrefix.EnvVars(ExcludeDirFlagName),
				Destination: &opts.HclExclude,
				Usage:       "Skip HCL formatting in given directories.",
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("hclfmt-exclude-dir"),
				opts.StrictControls,
			), // `TG_HCLFMT_EXCLUDE_DIR`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("hclfmt-exclude-dir"),
				opts.StrictControls,
			), // `TERRAGRUNT_EXCLUDE_DIR`
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        CheckFlagName,
				EnvVars:     tgPrefix.EnvVars(CheckFlagName),
				Destination: &opts.Check,
				Usage:       "Return a status code of zero when all files are formatted correctly, and a status code of one when they aren't.",
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("hclfmt-check"),
				opts.StrictControls,
			), // `TG_HCLFMT_CHECK`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("check"),
				opts.StrictControls,
			), // `TERRAGRUNT_CHECK`
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        DiffFlagName,
				EnvVars:     tgPrefix.EnvVars(DiffFlagName),
				Destination: &opts.Diff,
				Usage:       "Print diff between original and modified file versions.",
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("hclfmt-diff"),
				opts.StrictControls,
			), // `TG_HCLFMT_DIFF`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("diff"),
				opts.StrictControls,
			), // `TERRAGRUNT_DIFF`
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        StdinFlagName,
				EnvVars:     tgPrefix.EnvVars(StdinFlagName),
				Destination: &opts.HclFromStdin,
				Usage:       "Format HCL from stdin and print result to stdout.",
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("hclfmt-stdin"),
				opts.StrictControls,
			), // `TG_HCLFMT_STDIN`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("hclfmt-stdin"),
				opts.StrictControls,
			), // `TERRAGRUNT_HCLFMT_STDIN`
		),
	}

	flagSet = flagSet.Add(shared.NewQueueFlags(opts, nil)...)
	flagSet = flagSet.Add(shared.NewFilterFlags(l, opts, v)...)
	flagSet = flagSet.Add(shared.NewParallelismFlag(opts))

	return flagSet
}

// NewCommand returns the `terragrunt hcl format` command.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	cmd := &clihelper.Command{
		Name:    CommandName,
		Aliases: []string{CommandNameAlias},
		Usage:   "Recursively find HashiCorp Configuration Language (HCL) files and rewrite them into a canonical format.",
		Flags:   NewFlags(l, opts, v, nil),
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
