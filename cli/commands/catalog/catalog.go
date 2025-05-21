package catalog

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/service"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// Run is the main entry point for the catalog command.
// It initializes the catalog service, retrieves modules, and then launches the TUI.
func Run(ctx context.Context, opts *options.TerragruntOptions, repoURL string) error {
	svc := service.NewCatalogService(opts)

	if repoURL != "" {
		svc.WithRepoURL(repoURL)
	}

	err := svc.Load(ctx)
	if err != nil {
		opts.Logger.Error(err)
	}

	if len(svc.Modules()) == 0 {
		return errors.New("no modules found by the catalog service")
	}

	return tui.Run(ctx, opts, svc)
}
