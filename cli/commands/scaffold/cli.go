// Package scaffold provides the command to scaffold a new Terragrunt module.
package scaffold

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "scaffold"

	OutputFolderFlagName = "output-folder"
	VarFlagName          = "var"
	VarFileFlagName      = "var-file"
	NoDependencyPrompt   = "no-dependency-prompt"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	// Start with shared scaffolding flags
	scaffoldFlags := shared.NewScaffoldingFlags(opts, prefix)

	// Add scaffold-specific flags
	scaffoldFlags = append(scaffoldFlags,
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

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoDependencyPrompt,
			EnvVars:     tgPrefix.EnvVars(NoDependencyPrompt),
			Destination: &opts.NoDependencyPrompt,
			Usage:       "Do not prompt for confirmation to include dependencies.",
		}),
	)

	return scaffoldFlags
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	flags := NewFlags(opts, nil)
	// Accept backend and feature flags for scaffold as well
	flags = append(flags, shared.NewBackendFlags(opts, nil)...)
	flags = append(flags, shared.NewFeatureFlags(opts, nil)...)

	return &cli.Command{
		Name:  CommandName,
		Usage: "Scaffold a new Terragrunt module.",
		Flags: flags,
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
