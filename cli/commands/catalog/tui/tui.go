// Package tui provides a text-based user interface for the Terragrunt catalog command.
package tui

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
)

// Run starts the text-based user interface for the Terragrunt catalog command.
func Run(ctx context.Context, modules module.Modules, opts *options.TerragruntOptions) error {
	if _, err := tea.NewProgram(newModel(modules, opts), tea.WithAltScreen(), tea.WithContext(ctx)).Run(); err != nil {
		if err := context.Cause(ctx); errors.Is(err, context.Canceled) {
			return nil
		} else if err != nil {
			return fmt.Errorf("error running TUI: %w", err)
		}

		// TODO: Work out why there's two of these.
		// Can't we just remove the one above?

		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
