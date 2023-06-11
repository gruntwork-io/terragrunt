package runall

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	cmdRunAll = "run-all"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	command := &cli.Command{
		Name:        cmdRunAll,
		Usage:       "Run a terraform command against a 'stack' by running the specified command in each subfolder.",
		Description: "Run a terraform command against a 'stack' by running the specified command in each subfolder. E.g., to run 'terragrunt apply' in each subfolder, use 'terragrunt run-all apply'.",
		Action:      func(ctx *cli.Context) error { return Run(opts) },
	}

	return command
}
