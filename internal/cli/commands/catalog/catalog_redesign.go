package catalog

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// runRedesign is the entry point for the redesigned catalog experience.
// It is invoked when the catalog-redesign experiment is enabled.
//
// For now, this delegates to the default implementation. As the redesign
// evolves, this function will be replaced with a completely different
// execution path (different service, different TUI, different orchestration).
func runRedesign(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, repoURL string) error {
	return runDefault(ctx, l, opts, repoURL)
}
