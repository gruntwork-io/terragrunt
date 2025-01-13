// Package strict provides utilities used by Terragrunt to support a "strict" mode.
// By default strict mode is disabled, but when Enabled, any breaking changes
// to Terragrunt behavior that is not backwards compatible will result in an error.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/strict-mode.md
//
// That is how users will know what to expect when they enable strict mode, and how to customize it.
package strict

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/exp/slices"
)

const (
	// DeprecatedFlags is the control that prevents the use of deprecated flag names.
	DeprecatedFlags ControlName = "deprecated-flags"
	// DeprecatedEnvVars is the control that prevents the use of deprecated env vars.
	DeprecatedEnvVars ControlName = "deprecated-env-vars"
	// DeprecatedCommands is the control that prevents the use of deprecated commands.
	DeprecatedCommands ControlName = "deprecated-commands"
	// DefaultCommand is the control that prevents the deprecated default command from being used.
	DefaultCommand ControlName = "default-command"
	// RootTerragruntHCL is the control that prevents usage of a `terragrunt.hcl` file as the root of Terragrunt configurations.
	RootTerragruntHCL ControlName = "root-terragrunt-hcl"
)

const (
	// StatusOngoing is the Status of a control that is ongoing.
	StatusOngoing byte = iota
	// StatusCompleted is the Status of a control that is completed.
	StatusCompleted
)

const (
	WarningCompletedControlsFmt = "The following strict control(s) are already completed: %s. Please remove any completed strict controls, as setting them no longer does anything. For a list of all ongoing strict controls, and the outcomes of previous strict controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode"
)

type ControlName string

type Controls map[ControlName]*Control

//nolint:lll
func NewControls() Controls {
	return Controls{
		DeprecatedFlags: {
			ErrorFmt: "--%s` flag is no longer supported. Use `--%s` instead.",
			WarnFmt:  "`--%s` flag is deprecated and will be removed in a future version. Use `--%s` instead.",
		},
		DeprecatedEnvVars: {
			ErrorFmt: "`--%s` env var is no longer supported. Use `--%s` instead.",
			WarnFmt:  "`--%s` env var is deprecated and will be removed in a future version. Use `--%s` instead.",
		},
		DeprecatedCommands: {
			ErrorFmt: "`%s` command is no longer supported. Use `%s` instead.",
			WarnFmt:  "`%s` command is deprecated and will be removed in a future version. Use `%s` instead.",
		},
		DefaultCommand: {
			ErrorFmt: "`%[1]s` command is not a valid Terragrunt command. Use `terragrunt run` to explicitly pass commands to OpenTofu/Terraform instead. e.g. `terragrunt run -- %[1]s`",
			WarnFmt:  "`%[1]s` command is deprecated and will be removed in a future version. Use `terragrunt run -- %[1]s` instead.",
		},
		RootTerragruntHCL: {
			ErrorFmt: fmt.Sprintf("Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern, and no longer supported. Use a differently named file like `root.hcl` instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/migrating-from-root-terragrunt-hcl"),
			WarnFmt:  "Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern, and no longer recommended. In a future version of Terragrunt, this will result in an error. You are advised to use a differently named file like `root.hcl` instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/migrating-from-root-terragrunt-hcl",
		},
	}
}

// Names returns the names of all strict controls.
func (controls Controls) Names() []string {
	names := []string{}

	for name := range controls {
		names = append(names, string(name))
	}

	slices.Sort(names)

	return names
}

// FindByStatus returns controls that have the given `Status`.
func (controls Controls) FindByStatus(Status byte) Controls {
	var found = make(Controls)

	for name, control := range controls {
		if control.Status == Status {
			found[name] = control
		}
	}

	return found
}

// EnableStrictMode enables the strict mode.
func (controls Controls) EnableStrictMode() {
	for _, control := range controls.FindByStatus(StatusOngoing) {
		control.Enabled = true
	}
}

// EnableControl validates that the specified control name is valid and sets the Enabled Status for this control.
func (controls Controls) EnableControl(name string) error {
	if control, ok := controls[ControlName(name)]; ok {
		control.Enabled = true

		return nil
	}

	return NewInvalidControlNameError(controls.FindByStatus(StatusOngoing).Names())
}

// NotifyCompletedControls logs the control names that are Enabled and have completed Status.
func (controls Controls) NotifyCompletedControls(logger log.Logger) {
	var completed = make(Controls)

	for name, control := range controls.FindByStatus(StatusCompleted) {
		if control.Enabled {
			completed[name] = control
		}
	}

	if len(completed) == 0 {
		return
	}

	logger.Warnf(WarningCompletedControlsFmt, strings.Join(completed.Names(), ", "))
}

// Evaluate returns an error if the control is Enabled otherwise print a warning once.
// If the control is not found, returns nil.
func (controls Controls) Evaluate(logger log.Logger, name ControlName, args ...any) error {
	if control, ok := controls.FindByStatus(StatusOngoing)[ControlName(name)]; ok {
		if err := control.Evaluate(logger, args...); err != nil {
			return err
		}
	}

	return nil
}

// Control represents a control that can be Enabled or disabled in strict mode.
// When the control is Enabled, Terragrunt will behave in a way that is not backwards compatible.
type Control struct {
	// ErrorFmt is the error that will be returned when the control is Enabled.
	ErrorFmt string
	// WarnFmt is a warning that will be logged when the control is not Enabled.
	WarnFmt string
	// Status of the strict control.
	Status byte
	// Enabled indicates that the control is Enabled.
	Enabled bool
	// TriggeredArgs keeps the args for which a warning has been previously issued.
	TriggeredArgs [][]any
}

// Evaluate returns an error if the control is Enabled otherwise print a warning once.
func (control *Control) Evaluate(logger log.Logger, args ...any) error {
	if control.Status == StatusCompleted {
		return nil
	}

	if control.Enabled && control.ErrorFmt != "" {
		return errors.Errorf(control.ErrorFmt, args...)
	}

	if control.WarnFmt == "" || logger == nil {
		return nil
	}

	for _, TriggeredArgs := range control.TriggeredArgs {
		if reflect.DeepEqual(TriggeredArgs, args) {
			return nil
		}
	}

	control.TriggeredArgs = append(control.TriggeredArgs, args)

	logger.Warnf(control.WarnFmt, args...)

	return nil
}
