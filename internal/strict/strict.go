// Package strict provides utilities used by Terragrunt to support a "strict" mode.
// By default strict mode is disabled, but when enabled, any breaking changes
// to Terragrunt behavior that is not backwards compatible will result in an error.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/strict-mode.md
//
// That is how users will know what to expect when they enable strict mode, and how to customize it.
package strict

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/util"
)

type Controls map[string]Control

// Get returns the strict control with the given name.
func (controls Controls) Get(name string) (Control, bool) {
	control, ok := controls[name]

	return control, ok
}

// Control represents a control that can be enabled or disabled in strict mode.
// When the control is enabled, Terragrunt will behave in a way that is not backwards compatible.
type Control struct {
	// Error is the error that will be returned when the control is enabled.
	Error error
	// Warning is a warning that will be logged when the control is not enabled.
	Warning string
}

// Evaluate returns a warning if the control is not enabled, and an error if the control is enabled.
func (control *Control) Evaluate(name string, opts *options.TerragruntOptions) (string, error) {
	if opts.StrictMode {
		return "", control.Error
	}

	if len(opts.StrictControls) > 0 && !util.ListContainsElement(opts.StrictControls, name) {
		return "", control.Error
	}

	return control.Warning, nil
}

type Command struct {
	Control
	*cli.Command
}

func (cmd *Command) CLICommand(opts *options.TerragruntOptions) *cli.Command {
	actionFn := cmd.Command.Action

	cmd.Command.Action = func(ctx *cli.Context) error {
		warning, err := cmd.Control.Evaluate(cmd.Name, opts)
		if err != nil {
			return err
		}

		opts.Logger.Warn(warning)

		return actionFn(ctx)
	}

	return cmd.Command
}
