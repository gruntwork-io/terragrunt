package renderjson

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "render-json"

	FlagNameTerragruntJSONOut = "terragrunt-json-out"
	FlagNameWithMetadata      = "with-metadata"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntJSONOut,
			Destination: &opts.JSONOut,
			Usage:       "The file path that terragrunt should use when rendering the terragrunt.hcl config as json.",
		},
		&cli.BoolFlag{
			Name:        FlagNameWithMetadata,
			Destination: &opts.RenderJsonWithMetadata,
			Usage:       "Add metadata to the rendered JSON file.",
		},
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, as json.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:       NewFlags(opts).Sort(),
		Action:      func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
