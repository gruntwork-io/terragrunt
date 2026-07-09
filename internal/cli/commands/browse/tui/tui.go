package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Run launches the Miller-columns browser over the given tree, blocking until
// the user quits. fs backs the on-demand reads of surrounding entries and file
// previews. resultCh delivers the background discovery result, which annotates
// the tree once it arrives, and warnCh the warnings logged while it runs,
// shown as toasts. A cancelled context is treated as a clean exit.
func Run(
	ctx context.Context,
	l log.Logger,
	fs vfs.FS,
	root *Node,
	shouldColor bool,
	resultCh <-chan DiscoveryResult,
	warnCh <-chan viewtui.Warning,
) error {
	_, err := tea.NewProgram(NewModel(l, fs, root, shouldColor, resultCh, warnCh), tea.WithContext(ctx)).Run()
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
