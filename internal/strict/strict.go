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
	"strings"

	"github.com/gruntwork-io/terragrunt/options"
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

//nolint:stylecheck,lll,revive
var (
	ErrSpinUp            = errors.New("The `spin-up` command is no longer supported. Use `terragrunt run-all apply` instead.")
	ErrTearDown          = errors.New("The `tear-down` command is no longer supported. Use `terragrunt run-all destroy` instead.")
	ErrStrictPlanAll     = errors.New("The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.")
	ErrStrictApplyAll    = errors.New("The `apply-all` command is no longer supported. Use `terragrunt run-all apply` instead.")
	ErrStrictDestroyAll  = errors.New("The `destroy-all` command is no longer supported. Use `terragrunt run-all destroy` instead.")
	ErrStrictOutputAll   = errors.New("The `output-all` command is no longer supported. Use `terragrunt run-all output` instead.")
	ErrStrictValidateAll = errors.New("The `validate-all` command is no longer supported. Use `terragrunt run-all validate` instead.")
)

type Controls map[string]Control

//nolint:lll,gochecknoglobals
var StrictControls = Controls{
	SpinUp: {
		Error:   ErrSpinUp,
		Warning: "The `spin-up` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	TearDown: {
		Error:   ErrTearDown,
		Warning: "The `tear-down` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	PlanAll: {
		Error:   ErrStrictPlanAll,
		Warning: "The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.",
	},
	ApplyAll: {
		Error:   ErrStrictApplyAll,
		Warning: "The `apply-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	DestroyAll: {
		Error:   ErrStrictDestroyAll,
		Warning: "The `destroy-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	OutputAll: {
		Error:   ErrStrictOutputAll,
		Warning: "The `output-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all output` instead.",
	},
	ValidateAll: {
		Error:   ErrStrictValidateAll,
		Warning: "The `validate-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all validate` instead.",
	},
}

func (controls Controls) ValidateControlNames(strictControlNames []string) error {
	invalidControls := []string{}

	for _, controlName := range strictControlNames {
		_, ok := controls[controlName]
		if !ok {
			invalidControls = append(invalidControls, controlName)
		}
	}

	if len(invalidControls) > 0 {
		return errors.New("Invalid strict controls: " + strings.Join(invalidControls, ", "))
	}

	return nil
}
