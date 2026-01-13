package print

import (
	"context"

	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "print"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdFlags := runcmd.NewFlags(l, opts, nil)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil))

	cmd := &clihelper.Command{
		Name:      CommandName,
		Usage:     "Print out a short description of Terragrunt context.",
		UsageText: "terragrunt info print",
		Flags:     cmdFlags,
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
