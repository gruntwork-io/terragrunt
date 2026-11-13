package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// NoStackGenerateFlagName is the name of the flag that skips regenerating stacks
// before the command runs.
const NoStackGenerateFlagName = "no-stack-generate"

// NewNoStackGenerateFlag binds the automatic stack generation opt-out to opts.
func NewNoStackGenerateFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoStackGenerateFlagName,
		EnvVars:     prefix.Prepend(flags.TgPrefix).EnvVars(NoStackGenerateFlagName),
		Destination: &opts.NoStackGenerate,
		Usage:       "Disable automatic stack regeneration before running the command.",
	})
}
