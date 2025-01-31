// Package hclvalidate provides the `hclvalidate` command for Terragrunt.
//
// `hclvalidate` command recursively looks for hcl files in the directory tree starting at workingDir, and validates them
// based on the language style guides provided by Hashicorp. This is done using the official hcl2 library.
package hclvalidate

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "hclvalidate"

	ShowConfigPathFlagName = "show-config-path"
	JSONFlagName           = "json"

	DeprecatedHclvalidateShowConfigPathFlagName = "hclvalidate-show-config-path"
	DeprecatedHclvalidateJSONFlagName           = "hclvalidate-json"
)

func NewFlags(opts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	cliRedesignControl := flags.StrictControlsByGroup(opts.StrictControls, CommandName, controls.CLIRedesign)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowConfigPathFlagName,
			EnvVars:     tgPrefix.EnvVars(ShowConfigPathFlagName),
			Usage:       "Show a list of files with invalid configuration.",
			Destination: &opts.ShowConfigPath,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedHclvalidateShowConfigPathFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONFlagName),
			Destination: &opts.JSONOutput,
			Usage:       "Output the result in JSON format.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedHclvalidateJSONFlagName), cliRedesignControl)),
	}
}

func NewCommand(generalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(generalOpts)
	prefix := flags.Prefix{CommandName}

	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Find all hcl files from the config stack and validate them.",
		Flags:                NewFlags(opts, prefix).Sort(),
		ErrorOnUndefinedFlag: true,
		Action:               func(ctx *cli.Context) error { return Run(ctx, opts) },
	}
}
