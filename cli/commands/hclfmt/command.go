// Package hclfmt provides a command to recursively find hcl files and rewrite them into a canonical format
// based on the language style guides provided by Hashicorp. This is done using the official hcl2 library.
package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName is the name of the command.
	CommandName = "hclfmt"

	// FlagNameTerragruntHCLFmt is the name of the flag that specifies a single hcl file to run the hclfmt command on.
	FlagNameTerragruntHCLFmt = "terragrunt-hclfmt-file"

	// FlagNameTerragruntCheck is the name of the flag that enables check mode in the hclfmt command.
	FlagNameTerragruntCheck = "terragrunt-check"

	// FlagNameTerragruntDiff is the name of the flag that prints the diff between original and modified file versions.
	FlagNameTerragruntDiff = "terragrunt-diff"
)

// NewFlags returns the flags for the hclfmt command.
func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntHCLFmt,
			Destination: &opts.HclFile,
			Usage:       "The path to a single hcl file that the hclfmt command should run on.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntCheck,
			Destination: &opts.Check,
			EnvVar:      "TERRAGRUNT_CHECK",
			Usage:       "Enable check mode in the hclfmt command.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntDiff,
			Destination: &opts.Diff,
			EnvVar:      "TERRAGRUNT_DIFF",
			Usage:       "Print diff between original and modified file versions when running with 'hclfmt'.",
		},
	}
}

// NewCommand returns the hclfmt command.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
