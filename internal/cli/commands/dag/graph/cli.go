// Package graph implements the terragrunt dag graph command which generates a visual
// representation of the Terragrunt dependency graph in DOT language format.
//
// Alias for 'list --format=dot --dag --dependencies --external'.
package graph

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName = "graph"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	sharedFlags := shared.NewQueueFlags(opts, nil)
	sharedFlags = append(sharedFlags, shared.NewBackendFlags(opts, nil)...)
	sharedFlags = append(sharedFlags, shared.NewFeatureFlags(opts, nil)...)
	sharedFlags = append(sharedFlags, shared.NewFilterFlags(l, opts)...)

	return &clihelper.Command{
		Name: CommandName,
		Usage: "Graph the Directed Acyclic Graph (DAG) in DOT language." +
			" Alias for 'list --format=dot --dag --dependencies --external'.",
		UsageText: "terragrunt dag graph",
		Flags:     sharedFlags,
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, v.Writers.Writer, opts)
		},
	}
}

func Run(ctx context.Context, l log.Logger, v *venv.Venv, out io.Writer, opts *options.TerragruntOptions) error {
	listOpts := list.NewOptions(opts)
	listOpts.Format = list.FormatDot
	listOpts.Mode = list.ModeDAG
	listOpts.Dependencies = true

	return list.Run(ctx, l, v, out, listOpts)
}
