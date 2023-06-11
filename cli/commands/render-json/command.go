package renderjson

import (
	"sort"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	cmdRenderJSONFmt = "render-json"

	flagTerragruntJSONOut = "terragrunt-json-out"
	flagWithMetadata      = "with-metadata"
)

func NewCommand(globalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(globalOpts)

	command := &cli.Command{
		Name:        cmdRenderJSONFmt,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, as json.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Action:      func(ctx *cli.Context) error { return Run(opts) },
	}

	command.AddFlags(
		&cli.GenericFlag[string]{
			Name:        flagTerragruntJSONOut,
			Destination: &opts.JSONOut,
			Usage:       "The file path that terragrunt should use when rendering the terragrunt.hcl config as json. Only used in the render-json command. Defaults to terragrunt_rendered.json.",
		},
		&cli.BoolFlag{
			Name:        flagWithMetadata,
			Destination: &opts.RenderJsonWithMetadata,
			Usage:       "Add metadata to the rendered JSON file.",
		},
	)
	sort.Sort(cli.Flags(command.Flags))

	return command
}
