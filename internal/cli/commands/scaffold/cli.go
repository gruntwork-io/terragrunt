// Package scaffold provides the command to scaffold a new Terragrunt module.
package scaffold

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt scaffold` command.
	CommandName = "scaffold"

	// OutputFolderFlagName is the name of the flag that sets the directory the
	// scaffolded files are written to.
	OutputFolderFlagName = "output-folder"
	// VarFlagName is the name of the flag that passes a boilerplate variable.
	VarFlagName = "var"
	// VarFileFlagName is the name of the flag that passes a file of
	// boilerplate variables.
	VarFileFlagName = "var-file"
	// NoDependencyPrompt is the name of the flag that includes boilerplate
	// dependencies without asking for confirmation.
	NoDependencyPrompt = "no-dependency-prompt"
)

// NewFlags returns the flags of `terragrunt scaffold`, bound to opts.
func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	// Start with shared scaffolding flags
	scaffoldFlags := shared.NewScaffoldingFlags(opts, prefix)

	// Add scaffold-specific flags
	scaffoldFlags = append(scaffoldFlags,
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        OutputFolderFlagName,
			Destination: &opts.ScaffoldOutputFolder,
			Usage:       "Folder to write the scaffolded files into. A relative path is resolved against --working-dir.",
		}),

		flags.NewFlag(&clihelper.SliceFlag[string]{
			Name:        VarFlagName,
			EnvVars:     tgPrefix.EnvVars(VarFlagName),
			Destination: &opts.ScaffoldVars,
			Usage:       "Variables for usage in scaffolding.",
		}),

		flags.NewFlag(&clihelper.SliceFlag[string]{
			Name:        VarFileFlagName,
			EnvVars:     tgPrefix.EnvVars(VarFileFlagName),
			Destination: &opts.ScaffoldVarFiles,
			Usage:       "Files with variables for unit scaffolding. A relative path is resolved against --working-dir.",
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoDependencyPrompt,
			EnvVars:     tgPrefix.EnvVars(NoDependencyPrompt),
			Destination: &opts.NoDependencyPrompt,
			Usage:       "Do not prompt for confirmation to include dependencies.",
		}),
	)

	return scaffoldFlags
}

// NewCommand returns the `terragrunt scaffold` command, which takes a module
// URL and an optional template URL as its arguments.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	flags := NewFlags(opts, nil)
	// Accept backend and feature flags for scaffold as well
	flags = append(flags, shared.NewBackendFlags(opts, nil)...)
	flags = append(flags, shared.NewFeatureFlags(opts, nil)...)

	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Scaffold a new Terragrunt module.",
		Flags: flags,
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			var moduleURL, templateURL string

			if val := cliCtx.Args().Get(0); val != "" {
				moduleURL = val
			}

			if val := cliCtx.Args().Get(1); val != "" {
				templateURL = val
			}

			if opts.ScaffoldRootFileName == "" {
				opts.ScaffoldRootFileName = GetDefaultRootFileName(ctx, v.FS, opts)
			}

			return RunInteractive(ctx, l, v, opts.OptionsFromContext(ctx), moduleURL, templateURL)
		},
	}
}

// GetDefaultRootFileName returns the root config file name a scaffolded unit
// includes. It is [config.RecommendedParentConfigName] when the root
// terragrunt.hcl strict control is enabled or that file exists in
// opts.WorkingDir or one of up to opts.MaxFoldersToCheck parents, and
// [config.DefaultTerragruntConfigPath] otherwise.
func GetDefaultRootFileName(ctx context.Context, fsys vfs.FS, opts *options.TerragruntOptions) string {
	if err := opts.StrictControls.FilterByNames(controls.RootTerragruntHCL).
		SuppressWarning().
		Evaluate(ctx); err != nil {
		return config.RecommendedParentConfigName
	}

	// Check to see if you can find the recommended parent config name first,
	// if a user has it defined, go ahead and use it.
	dir := opts.WorkingDir

	prevDir := ""
	for foldersToCheck := opts.MaxFoldersToCheck; dir != prevDir && dir != "" && foldersToCheck > 0; foldersToCheck-- {
		prevDir = dir

		_, err := fsys.Stat(filepath.Join(dir, config.RecommendedParentConfigName))
		if err == nil {
			return config.RecommendedParentConfigName
		}

		dir = filepath.Dir(dir)
	}

	return config.DefaultTerragruntConfigPath
}
