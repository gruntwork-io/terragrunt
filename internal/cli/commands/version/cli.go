// Package version represents the version CLI command that works the same as the `--version` flag.
package version

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
)

const (
	CommandName = "version"
)

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

func Action(ctx context.Context, cliCtx *clihelper.Context) error {
	return clihelper.ShowVersion(ctx, cliCtx)
}
