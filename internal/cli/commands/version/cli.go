// Package version represents the version CLI command that works the same as the `--version` flag.
package version

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
)

const (
	// CommandName is the name of the `terragrunt version` command.
	CommandName = "version"
)

// NewCommand returns the hidden `terragrunt version` command, which prints
// the same version as --version.
func NewCommand() *clihelper.Command {
	return &clihelper.Command{
		Name:                         CommandName,
		Usage:                        "Show terragrunt version.",
		Hidden:                       true,
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			return clihelper.NewExitError(Action(ctx, cliCtx), 0)
		},
	}
}

// Action prints the Terragrunt version.
func Action(ctx context.Context, cliCtx *clihelper.Context) error {
	return clihelper.ShowVersion(ctx, cliCtx)
}
