package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/models/list"
	"github.com/gruntwork-io/terragrunt/options"
	"os"
)

func Run(ctx context.Context, modules module.Modules, opts *options.TerragruntOptions) error {
	ctx, cancel := context.WithCancelCause(ctx)
	quitFn := func(err error) {
		go cancel(err)
		if err != nil {
			// explicit exit from application
			os.Exit(1)
		}
	}

	list := list.NewModel(modules, quitFn, opts)

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
