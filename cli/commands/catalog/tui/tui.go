package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, modules module.Modules, opts *options.TerragruntOptions) error {
	if _, err := tea.NewProgram(newModel(modules, opts), tea.WithAltScreen(), tea.WithContext(ctx)).Run(); err != nil {
		if err := context.Cause(ctx); err == context.Canceled {
			return nil
		} else if err != nil {
			return err
		}

		return err
	}

	return nil
}
