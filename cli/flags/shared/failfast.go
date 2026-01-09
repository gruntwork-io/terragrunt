package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	FailFastFlagName = "fail-fast"
)

// NewFailFastFlag creates the --fail-fast flag for stopping execution on the first error.
func NewFailFastFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return flags.NewFlag(&cli.BoolFlag{
		Name:        FailFastFlagName,
		EnvVars:     tgPrefix.EnvVars(FailFastFlagName),
		Destination: &opts.FailFast,
		Usage:       "Fail immediately if any unit fails, rather than continuing to process remaining units.",
	})
}
