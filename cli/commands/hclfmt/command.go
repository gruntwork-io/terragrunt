package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	cmdNewHclFmt = "hclfmt"

	flagTerragruntHCLFmt = "terragrunt-hclfmt-file"
)

func NewCommand(globalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(globalOpts)

	command := &cli.Command{
		Name:   cmdNewHclFmt,
		Usage:  "Recursively find hcl files and rewrite them into a canonical format.",
		Action: func(ctx *cli.Context) error { return Run(opts) },
	}

	command.AddFlags(
		&cli.GenericFlag[string]{
			Name:        flagTerragruntHCLFmt,
			Destination: &opts.HclFile,
			Usage:       "The path to a single hcl file that the hclfmt command should run on.",
		},
	)

	return command
}
