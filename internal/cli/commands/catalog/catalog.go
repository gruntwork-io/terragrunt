package catalog

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Run is the main entry point for the catalog command.
// It dispatches to either the default or redesigned catalog experience
// based on the catalog-redesign experiment.
func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, repoURL string) error {
	if opts.Experiments.Evaluate(experiment.CatalogRedesign) {
		return runRedesign(ctx, l, opts, repoURL)
	}

	return runDefault(ctx, l, opts, repoURL)
}

// runDefault is the default catalog experience.
// It initializes the catalog service, retrieves modules, and then launches the TUI.
func runDefault(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, repoURL string) error {
	svc := catalog.NewCatalogService(opts)

	if repoURL != "" {
		svc.WithRepoURL(repoURL)
	}

	err := svc.Load(ctx, l)
	if err != nil {
		l.Error(err)
	}

	if len(svc.Modules()) == 0 {
		return errors.New("no modules found by the catalog service")
	}

	return tui.Run(ctx, l, opts, svc)
}
