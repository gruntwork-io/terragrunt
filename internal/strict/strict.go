// Package strict provides utilities used by Terragrunt to support a "strict" mode.
// By default strict mode is disabled, but when enabled, any breaking changes
// to Terragrunt behavior that is not backwards compatible will result in an error.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/strict-mode.md
//
// That is how users will know what to expect when they enable strict mode, and how to customize it.
package strict

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// Control represents a control that can be enabled or disabled in strict mode.
// When the control is enabled, Terragrunt will behave in a way that is not backwards compatible.
type Control struct {
	// Error is the error that will be returned when the control is enabled.
	Error error
	// Warning is a warning that will be logged when the control is not enabled.
	Warning string
}

const (
	// SpinUp is the control that prevents the deprecated `spin-up` command from being used.
	SpinUp = "spin-up"
	// TearDown is the control that prevents the deprecated `tear-down` command from being used.
	TearDown = "tear-down"
	// PlanAll is the control that prevents the deprecated `plan-all` command from being used.
	PlanAll = "plan-all"
	// ApplyAll is the control that prevents the deprecated `apply-all` command from being used.
	ApplyAll = "apply-all"
	// DestroyAll is the control that prevents the deprecated `destroy-all` command from being used.
	DestroyAll = "destroy-all"
	// OutputAll is the control that prevents the deprecated `output-all` command from being used.
	OutputAll = "output-all"
	// ValidateAll is the control that prevents the deprecated `validate-all` command from being used.
	ValidateAll = "validate-all"

	// SkipDependenciesInputs is the control that prevents reading dependencies inputs and get performance boost.
	SkipDependenciesInputs = "skip-dependencies-inputs"
)

// GetStrictControl returns the strict control with the given name.
func GetStrictControl(name string) (Control, bool) {
	control, ok := StrictControls[name]

	return control, ok
}

// Evaluate returns a warning if the control is not enabled, and an error if the control is enabled.
func (control Control) Evaluate(opts *options.TerragruntOptions) (string, error) {
	if opts.StrictMode {
		return "", control.Error
	}

	for _, controlName := range opts.StrictControls {
		strictControl, ok := StrictControls[controlName]
		if !ok {
			// This should never happen, but if it does, it's a bug in Terragrunt.
			// The slice of StrictControls should be validated before they're used.
			return "", errors.New("Invalid strict control: " + controlName)
		}

		if strictControl == control {
			return "", control.Error
		}
	}

	return control.Warning, nil
}

type Controls map[string]Control

//nolint:lll,gochecknoglobals,stylecheck
var StrictControls = Controls{
	SpinUp: {
		Error:   errors.New("The `spin-up` command is no longer supported. Use `terragrunt run-all apply` instead."),
		Warning: "The `spin-up` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	TearDown: {
		Error:   errors.New("The `tear-down` command is no longer supported. Use `terragrunt run-all destroy` instead."),
		Warning: "The `tear-down` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	PlanAll: {
		Error:   errors.New("The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead."),
		Warning: "The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.",
	},
	ApplyAll: {
		Error:   errors.New("The `apply-all` command is no longer supported. Use `terragrunt run-all apply` instead."),
		Warning: "The `apply-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	DestroyAll: {
		Error:   errors.New("The `destroy-all` command is no longer supported. Use `terragrunt run-all destroy` instead."),
		Warning: "The `destroy-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	OutputAll: {
		Error:   errors.New("The `output-all` command is no longer supported. Use `terragrunt run-all output` instead."),
		Warning: "The `output-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all output` instead.",
	},
	ValidateAll: {
		Error:   errors.New("The `validate-all` command is no longer supported. Use `terragrunt run-all validate` instead."),
		Warning: "The `validate-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all validate` instead.",
	},
	SkipDependenciesInputs: {
		Error:   errors.New(fmt.Sprintf("The `%s` option is deprecated. Reading inputs from dependencies has been deprecated and will be removed in a future version of Terragrunt. To continue using inputs from dependencies, forward them as outputs.", SkipDependenciesInputs)),
		Warning: fmt.Sprintf("The `%s` option is deprecated and will be removed in a future version of Terragrunt. Reading inputs from dependencies has been deprecated. To continue using inputs from dependencies, forward them as outputs.", SkipDependenciesInputs),
	},
}

// Names returns the names of all strict controls.
func (controls Controls) Names() []string {
	names := []string{}

	for name := range controls {
		names = append(names, name)
	}

	return names
}

var (
	// ErrInvalidStrictControl is returned when an invalid strict control is used.
	ErrInvalidStrictControl = errors.New("Invalid value(s) used for --strict-control.") //nolint:stylecheck,revive
)

// ValidateControlNames validates that the given control names are valid.
func (controls Controls) ValidateControlNames(strictControlNames []string) error {
	invalidControls := []string{}
	validControls := controls.Names()

	for _, controlName := range strictControlNames {
		if !util.ListContainsElement(validControls, controlName) {
			invalidControls = append(invalidControls, controlName)
		}
	}

	if len(invalidControls) > 0 {
		return fmt.Errorf("%w\nInvalid value(s):\n- %s\nAllowed value(s):\n- %s",
			ErrInvalidStrictControl,
			strings.Join(invalidControls, "\n- "),
			strings.Join(validControls, "\n- "),
		)
	}

	return nil
}
