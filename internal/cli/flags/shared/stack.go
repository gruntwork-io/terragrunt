package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// NoStackGenerateFlagName is the flag name for `--no-stack-generate`, which skips regenerating stacks
	// before the command runs.
	NoStackGenerateFlagName = "no-stack-generate"
)

// NewNoStackGenerateFlag creates the flag that skips regenerating stacks before the command runs.
func NewNoStackGenerateFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoStackGenerateFlagName,
		EnvVars:     tgPrefix.EnvVars(NoStackGenerateFlagName),
		Destination: &opts.NoStackGenerate,
		Usage:       "Disable automatic stack regeneration before running the command.",
	})
}
