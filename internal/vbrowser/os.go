package vbrowser

import (
	"context"
	"fmt"
	"os/exec"
)

type osOpener struct {
	goos string
}

func (o osOpener) Open(ctx context.Context, rawURL string) error {
	name, args, err := OpenCommand(o.goos, rawURL)
	if err != nil {
		return err
	}

	// The launcher is bounded because a host with no way to open a URL can
	// leave xdg-open waiting on a session that never answers. Leaving Stdout
	// and Stderr nil sends the child's output to the null device, so a chatty
	// launcher cannot interleave with what the caller already printed.
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()

	if err := exec.CommandContext(ctx, name, args...).Run(); err != nil {
		return fmt.Errorf("vbrowser: running %s: %w", name, err)
	}

	return nil
}
