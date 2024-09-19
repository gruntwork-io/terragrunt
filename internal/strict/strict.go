// Package strict provides utilities used by Terragrunt to support a "strict" mode.
// By default strict mode is disabled, but when enabled, any breaking changes
// to Terragrunt behavior that is not backwards compatible will result in an error.
package strict

import (
	"errors"
	"os"
)

// Control represents a control that can be enabled or disabled in strict mode.
// When the control is enabled, Terragrunt will behave in a way that is not backwards compatible.
type Control struct {
	// FlagName is the environment variable that will enable this control.
	FlagName string
	// Error is the error that will be returned when the control is enabled.
	Error error
	// Warning is a warning that will be logged when the control is not enabled.
	Warning string
}

const (
	// StrictModeEnvVar is the environment variable that will enable strict mode.
	StrictModeEnvVar = "TG_STRICT_MODE"

	// SpinUp is the control that prevents the deprecated `spin-up` command from being used.
	SpinUp string = "spin-up"
	// TearDown is the control that prevents the deprecated `tear-down` command from being used.
	TearDown string = "tear-down"
	// PlanAll is the control that prevents the deprecated `plan-all` command from being used.
	PlanAll string = "plan-all"
	// ApplyAll is the control that prevents the deprecated `apply-all` command from being used.
	ApplyAll string = "apply-all"
	// DestroyAll is the control that prevents the deprecated `destroy-all` command from being used.
	DestroyAll string = "destroy-all"
	// OutputAll is the control that prevents the deprecated `output-all` command from being used.
	OutputAll string = "output-all"
	// ValidateAll is the control that prevents the deprecated `validate-all` command from being used.
	ValidateAll string = "validate-all"
)

// GetStrictControl returns the strict control with the given name.
func GetStrictControl(name string) (Control, bool) {
	control, ok := strictControls[name]

	return control, ok
}

// Evaluate returns a warning if the control is not enabled, and an error if the control is enabled.
func (control Control) Evaluate() (string, error) {
	strictMode := os.Getenv(StrictModeEnvVar)
	if strictMode == "true" {
		return "", control.Error
	}

	enabled := os.Getenv(control.FlagName)
	if enabled == "true" {
		return "", control.Error
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

//nolint:lll,gochecknoglobals
var strictControls = map[string]Control{
	SpinUp: {
		FlagName: "TG_STRICT_SPIN_UP",
		Error:    ErrSpinUp,
		Warning:  "The `spin-up` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	TearDown: {
		FlagName: "TG_STRICT_TEAR_DOWN",
		Error:    ErrTearDown,
		Warning:  "The `tear-down` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	PlanAll: {
		FlagName: "TG_STRICT_PLAN_ALL",
		Error:    ErrStrictPlanAll,
		Warning:  "The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.",
	},
	ApplyAll: {
		FlagName: "TG_STRICT_APPLY_ALL",
		Error:    ErrStrictApplyAll,
		Warning:  "The `apply-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	DestroyAll: {
		FlagName: "TG_STRICT_DESTROY_ALL",
		Error:    ErrStrictDestroyAll,
		Warning:  "The `destroy-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	OutputAll: {
		FlagName: "TG_STRICT_OUTPUT_ALL",
		Error:    ErrStrictOutputAll,
		Warning:  "The `output-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all output` instead.",
	},
	ValidateAll: {
		FlagName: "TG_STRICT_VALIDATE_ALL",
		Error:    ErrStrictValidateAll,
		Warning:  "The `validate-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all validate` instead.",
	},
}
