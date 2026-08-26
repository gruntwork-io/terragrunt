package scaffold

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/internal/view/tui/form"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// RunInteractive scaffolds moduleURL, asking the user for the values it needs
// through the same form the catalog user interface shows, so reaching a
// component from the command line collects what browsing to it would.
//
// The form is skipped, leaving [Run]'s placeholders to be filled in by hand,
// when the user asked for --non-interactive, when there is no terminal to
// draw it on, or when the source asks for nothing.
func RunInteractive(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	moduleURL, templateURL string,
) error {
	plan, err := Prepare(ctx, l, v, opts, moduleURL, templateURL)
	if err != nil {
		return err
	}

	defer plan.Cleanup(v.FS)

	values, err := promptForValues(ctx, l, opts, plan, moduleURL)

	if errors.Is(err, form.ErrCancelled) {
		l.Info("Scaffolding cancelled. Nothing was written.")

		return nil
	}

	if err != nil {
		return err
	}

	return plan.Generate(ctx, l, v, opts, values)
}

// promptForValues returns the values the user filled in, or nil when they
// cannot be asked. Nil is what [Plan.Generate] already takes to mean "write
// the placeholders", so every path that skips the form lands on the behavior
// scaffolding had before there was one.
func promptForValues(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	plan *Plan,
	moduleURL string,
) (map[string]string, error) {
	if opts.NonInteractive {
		return nil, nil
	}

	// A CI job, a script, and a tool running Terragrunt all land here, so a
	// missing terminal falls back rather than failing: scaffolding has a
	// perfectly good non-interactive result to write.
	if err := viewtui.EnsureOSTTY(); err != nil {
		l.Debugf("Scaffolding without the form: %v", err)

		return nil, nil
	}

	fields := plan.FormFields()
	if len(fields) == 0 {
		l.Debug("Scaffolding without the form: the source declares no values to fill in")

		return nil, nil
	}

	return form.Run(ctx, sourceTitle(moduleURL), fields)
}

// sourceTitle names the component a form is collecting values for. A source
// URL is too long for the title strip, and its last segment is what the user
// recognizes it by.
func sourceTitle(moduleURL string) string {
	title, _, _ := strings.Cut(moduleURL, "?")
	title = strings.TrimSuffix(title, "/")

	if idx := strings.LastIndexAny(title, "/"); idx >= 0 {
		title = title[idx+1:]
	}

	if title == "" {
		return filepath.Base(moduleURL)
	}

	return title
}
