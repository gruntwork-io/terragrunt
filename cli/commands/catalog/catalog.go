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
	catalogService := service.NewCatalogService(opts, repoURL)

	modules, err := catalogService.ListModules(ctx)
	if err != nil {
		// The service layer now handles scenarios like no repos configured, returning an error.
		// It also handles logging for individual repo errors if it continues processing others.
		return errors.Errorf("failed to list modules: %w", err)
	}

	// The service returns an error if no modules are found after processing all repositories.
	// This check might be redundant if the service guarantees an error, but good for safety.
	if len(modules) == 0 {
		return errors.Errorf("no modules found by the catalog service")
	}

	opts.Logger.Debugf("Total modules collected by service: %d. Launching TUI.", len(modules))

	return tui.Run(ctx, modules, opts)
}
