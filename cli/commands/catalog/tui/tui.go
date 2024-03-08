package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/models/list"
	"github.com/gruntwork-io/terragrunt/options"
	"os/exec"
	"runtime"
)

func Run(ctx context.Context, modules module.Modules, opts *options.TerragruntOptions) error {
	ctx, cancel := context.WithCancelCause(ctx)
	quitFn := func(err error) {
		go cancel(err)
		go func() {
			// Reset the terminal to a sane state
			if runtime.GOOS == "darwin" {
				cmd := exec.Command("stty", "sane")
				_ = cmd.Run()
			}
		}()
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
