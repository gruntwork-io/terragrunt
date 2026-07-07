package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Run launches the Miller-columns browser over the given tree, blocking until
// the user quits. fs backs the on-demand reads of surrounding entries and file
// previews. resultCh delivers the background discovery result, which annotates
// the tree once it arrives. A cancelled context is treated as a clean exit.
func Run(ctx context.Context, fs vfs.FS, root *Node, shouldColor bool, resultCh <-chan DiscoveryResult) error {
	_, err := tea.NewProgram(NewModel(fs, root, shouldColor, resultCh), tea.WithContext(ctx)).Run()
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
