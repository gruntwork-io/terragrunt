// Package tui provides a text-based user interface for the Terragrunt catalog command.
package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, modules module.Modules, opts *options.TerragruntOptions) error {
	if _, err := tea.NewProgram(newModel(modules, opts), tea.WithAltScreen(), tea.WithContext(ctx)).Run(); err != nil {
		if causeErr := context.Cause(ctx); errors.Is(causeErr, context.Canceled) {
			return nil
		} else if causeErr != nil {
			return causeErr
		}

		return err
	}

	return nil
}
