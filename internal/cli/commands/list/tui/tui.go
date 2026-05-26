package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Run launches the Miller-columns browser over the given tree, blocking until
// the user quits. fs backs the on-demand reads of surrounding entries and file
// previews. A cancelled context is treated as a clean exit.
func Run(ctx context.Context, fs vfs.FS, root *Node, shouldColor bool) error {
	_, err := tea.NewProgram(NewModel(fs, root, shouldColor), tea.WithContext(ctx)).Run()
	if err == nil {
		return nil
	}

	cause := context.Cause(ctx)
	if errors.Is(cause, context.Canceled) {
		return nil
	}

	if cause != nil {
		return cause
	}

	return err
}
