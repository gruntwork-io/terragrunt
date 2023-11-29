package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/service"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/models/list"
)

func Run(ctx context.Context, modules service.Modules) error {
	ctx, cancel := context.WithCancelCause(ctx)
	quitFn := func(err error) {
		go cancel(err)
	}

	list := list.NewModel(modules, quitFn)

	if _, err := tea.NewProgram(list, tea.WithAltScreen(), tea.WithContext(ctx)).Run(); err != nil {
		if err := context.Cause(ctx); err == context.Canceled {
			return nil
		} else if err != nil {
			return err
		}

		return err
	}

	return nil
}
