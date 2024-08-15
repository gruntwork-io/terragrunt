// Package renderjson provides the command to render the final terragrunt config,
// with all variables, includes, and functions resolved, as json.
package renderjson

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName is the name of the command.
	CommandName = "render-json"

	// FlagNameTerragruntJSONOut is the name of the flag that specifies the file path that terragrunt should use when
	// rendering the terragrunt.hcl config as json.
	FlagNameTerragruntJSONOut = "terragrunt-json-out"

	// FlagNameWithMetadata is the name of the flag that specifies whether to add metadata to the rendered JSON file.
	FlagNameWithMetadata = "with-metadata"

	// FlagNameDisableDependentModules is the name of the flag that specifies whether to disable identification of
	// dependent modules rendering json config.
	FlagNameDisableDependentModules = "terragrunt-json-disable-dependent-modules"
)

// NewFlags returns the flags for the render-json command.
func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntJSONOut,
			Destination: &opts.JSONOut,
			Usage:       "The file path that terragrunt should use when rendering the terragrunt.hcl config as json.",
		},
		&cli.BoolFlag{
			Name:        FlagNameWithMetadata,
			Destination: &opts.RenderJSONWithMetadata,
			Usage:       "Add metadata to the rendered JSON file.",
		},
		&cli.BoolFlag{
			Name:        FlagNameDisableDependentModules,
			EnvVar:      "TERRAGRUNT_JSON_DISABLE_DEPENDENT_MODULES",
			Destination: &opts.JSONDisableDependentModules,
			Usage:       "Disable identification of dependent modules rendering json config.",
		},
	}
}

// NewCommand returns the command for the render-json command.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, as json.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.", //nolint:lll
		Flags:       NewFlags(opts).Sort(),
		Action:      func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}
