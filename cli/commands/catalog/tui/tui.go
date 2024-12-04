// Package tui provides a text-based user interface for the Terragrunt catalog command.
package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, modules module.Modules, opts *options.TerragruntOptions) error {
	if opts.ScaffoldRootFileName == "" {
		if control, ok := strict.GetStrictControl(strict.RootTerragruntHCL); ok {
			warn, triggered, err := control.Evaluate(opts)
			if err != nil {
				opts.ScaffoldRootFileName = config.RecommendedParentConfigName
			}

			if !triggered {
				opts.Logger.Warnf(warn)
			}

			opts.ScaffoldRootFileName = config.DefaultTerragruntConfigPath
		}
	}

	if _, err := tea.NewProgram(newModel(modules, opts), tea.WithAltScreen(), tea.WithContext(ctx)).Run(); err != nil {
		if err := context.Cause(ctx); errors.Is(err, context.Canceled) {
			return nil
		} else if err != nil {
			return err
		}

		return err
	}

	return nil
}
