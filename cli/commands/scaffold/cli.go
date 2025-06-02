// Package scaffold provides the command to scaffold a new Terragrunt module.
package scaffold

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "scaffold"

	RootFileNameFlagName  = "root-file-name"
	NoIncludeRootFlagName = "no-include-root"
	OutputFolderFlagName  = "output-folder"
	VarFlagName           = "var"
	VarFileFlagName       = "var-file"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        RootFileNameFlagName,
			EnvVars:     tgPrefix.EnvVars(RootFileNameFlagName),
			Destination: &opts.ScaffoldRootFileName,
			Usage:       "Name of the root Terragrunt configuration file, if used.",
			Action: func(ctx *cli.Context, value string) error {
				if value == "" {
					return errors.New("root-file-name flag cannot be empty")
				}

				if value != opts.TerragruntConfigPath {
					opts.ScaffoldRootFileName = value

					return nil
				}

				if err := opts.StrictControls.FilterByNames(controls.RootTerragruntHCL).Evaluate(ctx); err != nil {
					return cli.NewExitError(err, cli.ExitCodeGeneralError)
				}

				return nil
			},
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoIncludeRootFlagName,
			EnvVars:     tgPrefix.EnvVars(NoIncludeRootFlagName),
			Destination: &opts.ScaffoldNoIncludeRoot,
			Usage:       "Do not include root unit in scaffolding done by catalog.",
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutputFolderFlagName,
			Destination: &opts.ScaffoldOutputFolder,
			Usage:       "Output folder for scaffold output.",
		}),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        VarFlagName,
			EnvVars:     tgPrefix.EnvVars(VarFlagName),
			Destination: &opts.ScaffoldVars,
			Usage:       "Variables for usage in scaffolding.",
		}),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        VarFileFlagName,
			EnvVars:     tgPrefix.EnvVars(VarFileFlagName),
			Destination: &opts.ScaffoldVarFiles,
			Usage:       "Files with variables to be used in unit scaffolding.",
		}),
	}
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Usage: "Scaffold a new Terragrunt module.",
		Flags: NewFlags(opts, nil),
		Action: func(ctx *cli.Context) error {
			var moduleURL, templateURL string

			if val := ctx.Args().Get(0); val != "" {
				moduleURL = val
			}

			if val := ctx.Args().Get(1); val != "" {
				templateURL = val
			}

			if opts.ScaffoldRootFileName == "" {
				opts.ScaffoldRootFileName = GetDefaultRootFileName(ctx, opts)
			}

			return Run(ctx, l, opts.OptionsFromContext(ctx), moduleURL, templateURL)
		},
	}
}

func GetDefaultRootFileName(ctx context.Context, opts *options.TerragruntOptions) string {
	if err := opts.StrictControls.FilterByNames(controls.RootTerragruntHCL).SuppressWarning().Evaluate(ctx); err != nil {
		return config.RecommendedParentConfigName
	}

	// Check to see if you can find the recommended parent config name first,
	// if a user has it defined, go ahead and use it.
	dir := opts.WorkingDir

	prevDir := ""
	for foldersToCheck := opts.MaxFoldersToCheck; dir != prevDir && dir != "" && foldersToCheck > 0; foldersToCheck-- {
		prevDir = dir

		_, err := os.Stat(filepath.Join(dir, config.RecommendedParentConfigName))
		if err == nil {
			return config.RecommendedParentConfigName
		}

		dir = filepath.Dir(dir)
	}

	return config.DefaultTerragruntConfigPath
}
