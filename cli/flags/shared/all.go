package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	AllFlagName  = "all"
	AllFlagAlias = "a"
)

// NewAllFlag creates the --all flag for running commands across all units in a stack.
func NewAllFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return flags.NewFlag(&cli.BoolFlag{
		Name:        AllFlagName,
		Aliases:     []string{AllFlagAlias},
		EnvVars:     tgPrefix.EnvVars(AllFlagName),
		Destination: &opts.RunAll,
		Usage:       `Run the specified command on the stack of units in the current directory.`,
		Action: func(_ context.Context, _ *cli.Context, _ bool) error {
			if opts.Graph {
				return errors.New(new(AllGraphFlagsError))
			}

			return nil
		},
	})
}
