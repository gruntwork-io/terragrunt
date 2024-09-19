// Package strict provides utilities used by Terragrunt to support a "strict" mode.
// By default strict mode is disabled, but when enabled, any breaking changes
// to Terragrunt behavior that is not backwards compatible will result in an error.
package strict

import (
	"errors"
	"os"
)

// StrictControl represents a control that can be enabled or disabled in strict mode.
// When the control is enabled, Terragrunt will behave in a way that is not backwards compatible.
type StrictControl struct {
	// FlagName is the environment variable that will enable this control.
	FlagName string
	// EnabledByDefault is true if the control is enabled by default.
	EnabledByDefault bool
	// Error is the error that will be returned when the control is enabled.
	Error error
	// Warning is a warning that will be logged when the control is not enabled.
	Warning string
}

const (
	StrictModeEnvVar = "TG_STRICT_MODE"

	SpinUp      string = "spin-up"
	TearDown    string = "tear-down"
	PlanAll     string = "plan-all"
	ApplyAll    string = "apply-all"
	DestroyAll  string = "destroy-all"
	OutputAll   string = "output-all"
	ValidateAll string = "validate-all"
)

// GetStrictControl returns the strict control with the given name.
func GetStrictControl(name string) (StrictControl, bool) {
	control, ok := strictControls[name]
	return control, ok
}

// Evaluate returns a warning if the control is not enabled, and an error if the control is enabled.
func (control StrictControl) Evaluate() (string, error) {
	strictMode := os.Getenv(StrictModeEnvVar)
	if strictMode == "true" {
		return "", control.Error
	}

	enabled := os.Getenv(control.FlagName)
	if enabled == "true" {
		return "", control.Error
	}

	if control.EnabledByDefault {
		return "", control.Error
	}

	return control.Warning, nil
}

var (
	ErrSpinUp            = errors.New("the `spin-up` command is no longer supported. Use `terragrunt run-all apply` instead")
	ErrTearDown          = errors.New("the `tear-down` command is no longer supported. Use `terragrunt run-all destroy` instead")
	ErrStrictPlanAll     = errors.New("the `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead")
	ErrStrictApplyAll    = errors.New("the `apply-all` command is no longer supported. Use `terragrunt run-all apply` instead")
	ErrStrictDestroyAll  = errors.New("the `destroy-all` command is no longer supported. Use `terragrunt run-all destroy` instead")
	ErrStrictOutputAll   = errors.New("the `output-all` command is no longer supported. Use `terragrunt run-all output` instead")
	ErrStrictValidateAll = errors.New("the `validate-all` command is no longer supported. Use `terragrunt run-all validate` instead")
)
var strictControls = map[string]StrictControl{
	SpinUp: {
		FlagName: "TG_STRICT_SPIN_UP",
		Error:    errors.New("the `spin-up` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead"),
		Warning:  "The `spin-up` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	TearDown: {
		FlagName: "TG_STRICT_TEAR_DOWN",
		Error:    errors.New("the `tear-down` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead"),
		Warning:  "The `tear-down` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	PlanAll: {
		FlagName: "TG_STRICT_PLAN_ALL",
		Error:    ErrStrictPlanAll,
		Warning:  "The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.",
	},
	ApplyAll: {
		FlagName: "TG_STRICT_APPLY_ALL",
		Error:    errors.New("the `apply-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead"),
		Warning:  "The `apply-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	DestroyAll: {
		FlagName: "TG_STRICT_DESTROY_ALL",
		Error:    errors.New("the `destroy-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead"),
		Warning:  "The `destroy-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	OutputAll: {
		FlagName: "TG_STRICT_OUTPUT_ALL",
		Error:    errors.New("the `output-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all output` instead"),
		Warning:  "The `output-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all output` instead.",
	},
	ValidateAll: {
		FlagName: "TG_STRICT_VALIDATE_ALL",
		Error:    errors.New("the `validate-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all validate` instead"),
		Warning:  "The `validate-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all validate` instead.",
	},
}
