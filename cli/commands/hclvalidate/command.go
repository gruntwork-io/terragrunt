// `hclvalidate` command recursively looks for hcl files in the directory tree starting at workingDir, and validates them
// based on the language style guides provided by Hashicorp. This is done using the official hcl2 library.

package hclvalidate

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "hclvalidate"

	TerragruntShowInvalidFlagName   = "terragrunt-invalid"
	TerragruntShowInvalidEnvVarName = "TERRAGRUNT_INVALID"

	TerragruntJSONFlagName   = "terragrunt-json"
	TerragruntJSONEnvVarName = "TERRAGRUNT_JSON"
)

func NewFlags(opts *Options) cli.Flags {
	return cli.Flags{
		&cli.BoolFlag{
			Name:        TerragruntShowInvalidFlagName,
			EnvVar:      TerragruntShowInvalidEnvVarName,
			Usage:       "Show the list of invalid configuration paths.",
			Destination: &opts.ShowInvalidConfigPath,
		},
		&cli.BoolFlag{
			Name:        TerragruntJSONFlagName,
			EnvVar:      TerragruntJSONEnvVarName,
			Destination: &opts.JSONOutput,
			Usage:       "Output the result in JSON format.",
		},
	}
}

func NewCommand(generalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(generalOpts)

	return &cli.Command{
		Name:   CommandName,
		Usage:  "Recursively find hcl files and validate them.",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(ctx, opts) },
	}
}
