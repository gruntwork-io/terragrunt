// `hclvalidate` command recursively looks for hcl files in the directory tree starting at workingDir, and validates them
// based on the language style guides provided by Hashicorp. This is done using the official hcl2 library.

package hclvalidate

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "hclvalidate"

	InvalidFlagName   = "terragrunt-hclvalidate-invalid"
	InvalidEnvVarName = "TERRAGRUNT_HCLVALIDATE_INVALID"

	JSONOutputFlagName   = "terragrunt-hclvalidate-json"
	JSONOutputEnvVarName = "TERRAGRUNT_HCLVALIDATE_JSON"
)

func NewFlags(opts *Options) cli.Flags {
	return cli.Flags{
		&cli.BoolFlag{
			Name:        InvalidFlagName,
			EnvVar:      InvalidEnvVarName,
			Usage:       "Show the list of invalid configuration paths.",
			Destination: &opts.InvalidConfigPath,
		},
		&cli.BoolFlag{
			Name:        JSONOutputFlagName,
			EnvVar:      JSONOutputEnvVarName,
			Destination: &opts.JSONOutput,
			Usage:       "Output the result in JSON format.",
		},
	}
}

func NewCommand(generalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(generalOpts)

	return &cli.Command{
		Name:   CommandName,
		Usage:  "Find all hcl files from the config stack and validate them.",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(ctx, opts) },
	}
}
