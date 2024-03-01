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
		if err != nil {
			// write error to /tmp/log.txt file
			file, _ := os.OpenFile("/tmp/error.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
			file.WriteString(err.Error())
			file.Close()
		}
		go cancel(err)
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
