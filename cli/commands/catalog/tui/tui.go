package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/models/list"
)

func Run(ctx context.Context) error {

	modules, err := module.ScanModules()
	if err != nil {
		return err
	}

	list := list.NewModel(modules)

	if _, err := tea.NewProgram(list, tea.WithAltScreen(), tea.WithContext(ctx)).Run(); err != nil {
		return err
	}

	return nil
}
