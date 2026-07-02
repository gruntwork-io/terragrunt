package shared

import (
	"context"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// DiscoveryBoundaryFlagName is the name of the --discovery-boundary flag.
const DiscoveryBoundaryFlagName = "discovery-boundary"

// NewDiscoveryBoundaryFlag creates the shared --discovery-boundary flag. It
// constrains dependent discovery (leading '...' filters) to a directory subtree
// instead of walking up to the git root, and validates that the value is an
// existing directory.
func NewDiscoveryBoundaryFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return flags.NewFlag(&clihelper.GenericFlag[string]{
		Name:        DiscoveryBoundaryFlagName,
		EnvVars:     tgPrefix.EnvVars(DiscoveryBoundaryFlagName),
		Destination: &opts.DiscoveryBoundary,
		Usage: "Constrain dependent discovery (leading '...' filters) to this directory" +
			" subtree instead of walking up to the git root.",
		Action: func(_ context.Context, _ *clihelper.Context, val string) error {
			if val == "" {
				return nil
			}

			info, err := os.Stat(val)
			if err != nil {
				return fmt.Errorf("--%s: %w", DiscoveryBoundaryFlagName, err)
			}

			if !info.IsDir() {
				return fmt.Errorf("--%s %q is not a directory", DiscoveryBoundaryFlagName, val)
			}

			return nil
		},
	})
}
