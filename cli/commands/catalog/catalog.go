package catalog

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Run is the main entry point for the catalog command.
// It initializes the catalog service, retrieves modules, and then launches the TUI.
func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, repoURL string) error {
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
