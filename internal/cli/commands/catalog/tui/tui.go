// Package tui provides a text-based user interface for the Terragrunt catalog command.
package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, svc catalog.CatalogService) error {
	if _, err := tea.NewProgram(NewModel(l, opts, svc), tea.WithContext(ctx)).Run(); err != nil {
		if cause := context.Cause(ctx); errors.Is(cause, context.Canceled) {
			return nil
		} else if cause != nil {
			return cause
		}

		return err
	}

	return nil
}
