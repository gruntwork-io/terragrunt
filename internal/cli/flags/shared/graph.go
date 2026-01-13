package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	GraphFlagName = "graph"
)

// NewGraphFlag creates the --graph flag for running commands following the DAG.
func NewGraphFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        GraphFlagName,
		EnvVars:     tgPrefix.EnvVars(GraphFlagName),
		Destination: &opts.Graph,
		Usage:       "Run the specified OpenTofu/Terraform command following the Directed Acyclic Graph (DAG) of dependencies.",
		Action: func(_ context.Context, _ *clihelper.Context, _ bool) error {
			if opts.RunAll {
				return errors.New(new(AllGraphFlagsError))
			}

			return nil
		},
	})
}
