// Package version represents the version CLI command that works the same as the `--version` flag.
package version

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli"
)

const (
	CommandName = "version"
)

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:                         CommandName,
		Usage:                        "Show terragrunt version.",
		Hidden:                       true,
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx context.Context, cliCtx *cli.Context) error {
			return cli.NewExitError(Action(ctx, cliCtx), 0)
		},
	}
}

func Action(ctx context.Context, cliCtx *cli.Context) error {
	return cli.ShowVersion(ctx, cliCtx)
}
