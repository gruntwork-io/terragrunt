// Package strict provides utilities used by Terragrunt to support a "strict" mode.
// By default strict mode is disabled, but when enabled, any breaking changes
// to Terragrunt behavior that is not backwards compatible will result in an error.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/strict-mode.md
//
// That is how users will know what to expect when they enable strict mode, and how to customize it.
package strict

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/puzpuzpuz/xsync/v3"

	"github.com/gruntwork-io/terragrunt/options"
)

// Control represents a control that can be enabled or disabled in strict mode.
// When the control is enabled, Terragrunt will behave in a way that is not backwards compatible.
type Control struct {
	// Error is the error that will be returned when the control is enabled.
	Error error
	// Warning is a warning that will be logged when the control is not enabled.
	Warning string
	// Status of the strict control.
	Status int
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
	// DisableLogFormattingName is the control that prevents the deprecated `--terragrunt-disable-log-formatting` flag from being used.
	DisableLogFormatting = "terragrunt-disable-log-formatting"
	// JSONLog is the control that prevents the deprecated `--terragrunt-json-log` flag from being used.
	JSONLog = "terragrunt-json-log"
	// TfLogJSON is the control that prevents the deprecated `--terragrunt-tf-logs-to-json` flag from being used.
	TfLogJSON = "terragrunt-tf-logs-to-json"
	// RootTerragruntHCL is the control that prevents usage of a `terragrunt.hcl` file as the root of Terragrunt configurations.
	RootTerragruntHCL = "root-terragrunt-hcl"
)

const (
	// StatusOngoing is the status of a control that is ongoing.
	StatusOngoing = iota
	// StatusCompleted is the status of a control that is completed.
	StatusCompleted
)

// GetStrictControl returns the strict control with the given name.
func GetStrictControl(name string) (*Control, bool) {
	control, ok := StrictControls[name]

	return control, ok
}

// Evaluate returns a warning if the control is not enabled, an indication of whether the control has already been triggered,
// and an error if the control is enabled.
func (control *Control) Evaluate(opts *options.TerragruntOptions) (string, bool, error) {
	_, triggered := TriggeredControls.LoadAndStore(control, true)

	if opts.StrictMode {
		return "", triggered, control.Error
	}

	for _, controlName := range opts.StrictControls {
		strictControl, ok := StrictControls[controlName]
		if !ok {
			// This should never happen, but if it does, it's a bug in Terragrunt.
			// The slice of StrictControls should be validated before they're used.
			return "", false, errors.New("Invalid strict control: " + controlName)
		}

		if strictControl == control {
			return "", triggered, control.Error
		}
	}

	return control.Warning, triggered, nil
}

type Controls map[string]*Control

//nolint:lll,gochecknoglobals,stylecheck,revive
var StrictControls = Controls{
	SpinUp: {
		Error:   errors.Errorf("The `%s` command is no longer supported. Use `terragrunt run-all apply` instead.", SpinUp),
		Warning: fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.", SpinUp),
	},
	TearDown: {
		Error:   errors.Errorf("The `%s` command is no longer supported. Use `terragrunt run-all destroy` instead.", TearDown),
		Warning: fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.", TearDown),
	},
	PlanAll: {
		Error:   errors.Errorf("The `%s` command is no longer supported. Use `terragrunt run-all plan` instead.", PlanAll),
		Warning: fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.", PlanAll),
	},
	ApplyAll: {
		Error:   errors.Errorf("The `%s` command is no longer supported. Use `terragrunt run-all apply` instead.", ApplyAll),
		Warning: fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.", ApplyAll),
	},
	DestroyAll: {
		Error:   errors.Errorf("The `%s` command is no longer supported. Use `terragrunt run-all destroy` instead.", DestroyAll),
		Warning: fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.", DestroyAll),
	},
	OutputAll: {
		Error:   errors.Errorf("The `%s` command is no longer supported. Use `terragrunt run-all output` instead.", OutputAll),
		Warning: fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version. Use `terragrunt run-all output` instead.", OutputAll),
	},
	ValidateAll: {
		Error:   errors.Errorf("The `%s` command is no longer supported. Use `terragrunt run-all validate` instead.", ValidateAll),
		Warning: fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version. Use `terragrunt run-all validate` instead.", ValidateAll),
	},
	SkipDependenciesInputs: {
		Error:   errors.Errorf("The `%s` option is deprecated. Reading inputs from dependencies has been deprecated and will be removed in a future version of Terragrunt. To continue using inputs from dependencies, forward them as outputs.", SkipDependenciesInputs),
		Warning: fmt.Sprintf("The `%s` option is deprecated and will be removed in a future version of Terragrunt. Reading inputs from dependencies has been deprecated. To continue using inputs from dependencies, forward them as outputs.", SkipDependenciesInputs),
	},
	DisableLogFormatting: {
		Error:   errors.Errorf("The `--%s` flag is no longer supported. Use `--terragrunt-log-format=key-value` instead.", DisableLogFormatting),
		Warning: fmt.Sprintf("The `--%s` flag is deprecated and will be removed in a future version. Use `--terragrunt-log-format=key-value` instead.", DisableLogFormatting),
	},
	JSONLog: {
		Error:   errors.Errorf("The `--%s` flag is no longer supported. Use `--terragrunt-log-format=json` instead.", JSONLog),
		Warning: fmt.Sprintf("The `--%s` flag is deprecated and will be removed in a future version. Use `--terragrunt-log-format=json` instead.", JSONLog),
	},
	TfLogJSON: {
		Error:   errors.Errorf("The `--%s` flag is no longer supported. Use `--terragrunt-log-format=json` instead.", TfLogJSON),
		Warning: fmt.Sprintf("The `--%s` flag is deprecated and will be removed in a future version. Use `--terragrunt-log-format=json` instead.", TfLogJSON),
	},
	RootTerragruntHCL: {
		Error:   errors.Errorf("Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern, and no longer supported. Use a differently named file like `root.hcl` instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/migrating-from-root-terragrunt-hcl"),
		Warning: "Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern, and no longer recommended. In a future version of Terragrunt, this will result in an error. You are advised to use a differently named file like `root.hcl` instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/migrating-from-root-terragrunt-hcl",
	},
}

var TriggeredControls = xsync.NewMapOf[*Control, bool]()

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
func (controls Controls) ValidateControlNames(strictControlNames []string) (string, error) {
	completedControls := []string{}
	invalidControls := []string{}
	validControls := controls.Names()

	for _, controlName := range strictControlNames {
		control, ok := controls[controlName]
		if !ok {
			invalidControls = append(invalidControls, controlName)
			continue
		}

		if control.Status == StatusCompleted {
			completedControls = append(completedControls, controlName)
		}
	}

	var warning string
	if len(completedControls) > 0 {
		warning = fmt.Sprintf("The following strict control(s) are already completed: %s. Please remove any completed strict controls, as setting them no longer does anything. For a list of all ongoing strict controls, and the outcomes of previous strict controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode",
			strings.Join(completedControls, ", "))
	}

	var err error
	if len(invalidControls) > 0 {
		err = errors.Errorf("%w\nInvalid control(s):\n- %s\nAllowed control(s):\n- %s",
			ErrInvalidStrictControl,
			strings.Join(invalidControls, "\n- "),
			strings.Join(validControls, "\n- "),
		)
	}

	return warning, err
}
