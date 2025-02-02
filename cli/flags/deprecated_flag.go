package flags

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
)

var _ = cli.Flag(new(DeprecatedFlag))

// DeprecatedFlags are multiple of DeprecatedFlag flags.
type DeprecatedFlags []*DeprecatedFlag

// DeprecatedFlag represents a deprecated flag that is not shown in the CLI help, but its names, envVars, are registered.
type DeprecatedFlag struct {
	cli.Flag
	names      []string
	envVars    []string
	controls   strict.Controls
	newValueFn NewValueFunc
}

// GetEnvVars implements `cli.Flag` interface.
func (flag *DeprecatedFlag) GetEnvVars() []string {
	if len(flag.envVars) == 0 && flag.Flag != nil {
		return flag.Flag.GetEnvVars()
	}

	return flag.envVars
}

// Names implements `cli.Flag` interface.
func (flag *DeprecatedFlag) Names() []string {
	if len(flag.names) == 0 && flag.Flag != nil {
		return flag.Flag.Names()
	}

	return flag.names
}

// SetStrictControls creates a strict control for the flag and registers it.
func (flag *DeprecatedFlag) SetStrictControls(mainFlag *Flag, regControlsFn RegisterStrictControlsFunc) {
	if regControlsFn == nil {
		return
	}

	var newValue string

	if flag.newValueFn != nil {
		newValue = flag.newValueFn(nil)
	}

	flagNameControl := controls.NewDeprecatedFlagName(flag, mainFlag, newValue)
	envVarControl := controls.NewDeprecatedEnvVar(flag, mainFlag, newValue)

	flag.controls = strict.Controls{flagNameControl, envVarControl}

	regControlsFn(flagNameControl, envVarControl)
}

// NewValueFunc represents a function that returns a new value for the current flag if a stale flag is called.
// Used when the current flag and the deprecated flag have different types. For example, the string `log-format` current flag
// need to be assigned the `json` value when the bool `terragrunt-json-log` is specified.

// NewValueFunc represents a function that returns a new value for the current flag if a deprecated flag is called.
// Used when the current flag and the deprecated flag are of different types. For example, the string `log-format` flag
// must be set to `json` when deprecated bool `terragrunt-json-log` flag is used. More examples:
//
// terragrunt-disable-log-formatting  replaced with: log-format=key-value
// terragrunt-json-log                replaced with: log-format=json
// terragrunt-tf-logs-to-json         replaced with: log-format=json
type NewValueFunc func(flagValue cli.FlagValue) string

// NewValue returns a callback function that is used to get a new value for the current flag.
func NewValue(val string) NewValueFunc {
	return func(_ cli.FlagValue) string {
		return val
	}
}

// RegisterStrictControlsFunc represents a callback func that registers the given controls in the `opts.StrictControls` stict control tree .
type RegisterStrictControlsFunc func(flagNameControl, envVarControl strict.Control)

// StrictControlsByGroup returns a callback function that adds the taken controls as subcontrols for the given `controlNames`.
// Using the given `commandName` as categories.
func StrictControlsByGroup(strcitControls strict.Controls, commandName string, controlNames ...string) RegisterStrictControlsFunc {
	return func(flagNameControl, envVarControl strict.Control) {
		flagNamesCategory := fmt.Sprintf(controls.CommandFlagsCategoryNameFmt, commandName)
		envVarsCategory := fmt.Sprintf(controls.CommandEnvVarsCategoryNameFmt, commandName)

		strcitControls.FilterByNames(append(controlNames, controls.DeprecatedFlags)...).AddSubcontrolsToCategory(flagNamesCategory, flagNameControl)
		strcitControls.FilterByNames(append(controlNames, controls.DeprecatedEnvVars)...).AddSubcontrolsToCategory(envVarsCategory, envVarControl)
	}
}

// StrictControls returns a callback function that adds the taken controls as subcontrols for the given `controlNames`.
// And assigns the "Global Flag" category to these controls.
func StrictControls(strcitControls strict.Controls, controlNames ...string) RegisterStrictControlsFunc {
	return func(flagNameControl, envVarControl strict.Control) {
		strcitControls.FilterByNames(append(controlNames, controls.DeprecatedFlags)...).AddSubcontrolsToCategory(controls.GlobalFlagsCategoryName, flagNameControl)
		strcitControls.FilterByNames(append(controlNames, controls.DeprecatedEnvVars)...).AddSubcontrolsToCategory(controls.GlobalEnvVarsCategoryName, envVarControl)
	}
}
