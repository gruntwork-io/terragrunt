// Package hclvalidate provides the `hclvalidate` command for Terragrunt.
//
// `hclvalidate` command recursively looks for hcl files in the directory tree starting at workingDir, and validates them
// based on the language style guides provided by Hashicorp. This is done using the official hcl2 library.
package hclvalidate

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "hclvalidate"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	prefix := flags.Prefix{CommandName}

	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Find all hcl files from the config stack and validate them.",
		Flags:                validate.NewFlags(opts, prefix).Filter(validate.ShowConfigPathFlagName, validate.JSONFlagName),
		ErrorOnUndefinedFlag: true,
		Action:               func(ctx *cli.Context) error { return validate.Run(ctx, opts) },
	}
}
