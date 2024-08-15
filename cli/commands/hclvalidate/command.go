// Package hclvalidate provides the `hclvalidate` command that recursively looks for hcl files in the directory tree
// starting at workingDir, and validates them based on the language style guides provided by Hashicorp. This is done
// using the official hcl2 library.
package hclvalidate

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName is the name of the command.
	CommandName = "hclvalidate"

	// ShowConfigPathFlagName is the name of the flag that controls whether to show a list of files with invalid
	// configuration.
	ShowConfigPathFlagName = "terragrunt-hclvalidate-show-config-path"
	// ShowConfigPathEnvVarName is the name of the environment variable that controls whether to show a list of files
	// with invalid configuration.
	ShowConfigPathEnvVarName = "TERRAGRUNT_HCLVALIDATE_SHOW_CONFIG_PATH"

	// JSONOutputFlagName is the name of the flag that controls whether to output the result in JSON format.
	JSONOutputFlagName = "terragrunt-hclvalidate-json"
	// JSONOutputEnvVarName is the name of the environment variable that controls whether to output the result in JSON
	// format.
	JSONOutputEnvVarName = "TERRAGRUNT_HCLVALIDATE_JSON"
)

// NewFlags returns the flags for the `hclvalidate` command.
func NewFlags(opts *Options) cli.Flags {
	return cli.Flags{
		&cli.BoolFlag{
			Name:        ShowConfigPathFlagName,
			EnvVar:      ShowConfigPathEnvVarName,
			Usage:       "Show a list of files with invalid configuration.",
			Destination: &opts.ShowConfigPath,
		},
		&cli.BoolFlag{
			Name:        JSONOutputFlagName,
			EnvVar:      JSONOutputEnvVarName,
			Destination: &opts.JSONOutput,
			Usage:       "Output the result in JSON format.",
		},
	}
}

// NewCommand returns the `hclvalidate` command.
func NewCommand(generalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(generalOpts)

	return &cli.Command{
		Name:   CommandName,
		Usage:  "Find all hcl files from the config stack and validate them.",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(ctx, opts) },
	}
}
