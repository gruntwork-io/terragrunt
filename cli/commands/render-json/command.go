package renderjson

import (
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "render-json"
)

var (
	TerragruntFlagNames = []string{
		flags.FlagNameTerragruntJSONOut,
		flags.FlagNameWithMetadata,
	}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, as json.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:       flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Action:      commands.Action(opts, Run),
	}
}
