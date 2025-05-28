// Package version represents the version CLI command that works the same as the `--version` flag.
package version

import (
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "version"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                         CommandName,
		Usage:                        "Show terragrunt version.",
		Hidden:                       true,
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context, l log.Logger) error {
			return cli.NewExitError(Action(ctx), 0)
		},
	}
}

func Action(ctx *cli.Context) error {
	return cli.ShowVersion(ctx)
}
