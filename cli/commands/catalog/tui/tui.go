// Package tui provides a text-based user interface for the Terragrunt catalog command.
package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, svc catalog.CatalogService) error {
	if _, err := tea.NewProgram(NewModel(l, opts, svc), tea.WithAltScreen(), tea.WithContext(ctx)).Run(); err != nil {
		if err := context.Cause(ctx); errors.Is(err, context.Canceled) {
			return nil
		} else if err != nil {
			return err
		}

		return err
	}

	return nil
}
