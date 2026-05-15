// Package catalog provides the ability to interact with a catalog of OpenTofu/Terraform modules
// via the `terragrunt catalog` command.
package catalog

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName = "catalog"

	IgnoreFileFlagName = "ignore-file"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	catalogFlags := clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        IgnoreFileFlagName,
			EnvVars:     tgPrefix.EnvVars(IgnoreFileFlagName),
			Destination: &opts.CatalogIgnoreFile,
			Usage:       "Path to an additional ignore file layered on top of a repo's .terragrunt-catalog-ignore during discovery.",
			Action: func(_ context.Context, _ *clihelper.Context, value string) error {
				if value == "" {
					return nil
				}

				resolved := value
				if !filepath.IsAbs(resolved) {
					workDir := opts.WorkingDir
					if workDir == "" {
						workDir = opts.RootWorkingDir
					}

					if workDir != "" {
						resolved = filepath.Join(workDir, resolved)
					}
				}

				info, err := os.Stat(resolved)
				if err != nil {
					return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
				}

				if info.IsDir() {
					return clihelper.NewExitError("--"+IgnoreFileFlagName+" must point to a file, not a directory", clihelper.ExitCodeGeneralError)
				}

				opts.CatalogIgnoreFile = resolved

				return nil
			},
		}),
	}

	return append(shared.NewScaffoldingFlags(opts, prefix), catalogFlags...)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Launch the user interface for searching and managing your module catalog.",
		Flags: NewFlags(opts, nil),
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			var repoPath string

			if val := cliCtx.Args().Get(0); val != "" {
				repoPath = val
			}

			if opts.ScaffoldRootFileName == "" {
				opts.ScaffoldRootFileName = scaffold.GetDefaultRootFileName(ctx, opts)
			}

			return Run(ctx, l, v, opts.OptionsFromContext(ctx), repoPath)
		},
	}
}
