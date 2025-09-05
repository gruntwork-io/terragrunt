package flags

import (
	"context"
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
	newValueFn             NewValueFunc
	controls               strict.Controls
	names                  []string
	envVars                []string
	allowedSubcommandScope bool
}

// GetHidden implements `cli.Flag` interface.
func (flag *DeprecatedFlag) GetHidden() bool {
	return true
}

// AllowedSubcommandScope implements `cli.Flag` interface.
func (flag *DeprecatedFlag) AllowedSubcommandScope() bool {
	return flag.allowedSubcommandScope
}

// GetEnvVars implements `cli.Flag` interface.
func (flag *DeprecatedFlag) GetEnvVars() []string {
	return flag.envVars
}

// Names implements `cli.Flag` interface.
func (flag *DeprecatedFlag) Names() []string {
	return flag.names
}

// Evaluate returns an error if the one of the controls is enabled otherwise logs warning messages and returns nil.
func (flag *DeprecatedFlag) Evaluate(ctx context.Context) error {
	return flag.controls.Evaluate(ctx)
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

	if ok := regControlsFn(flagNameControl, envVarControl); ok {
		flag.controls = strict.Controls{flagNameControl, envVarControl}
	}
}

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
type RegisterStrictControlsFunc func(flagNameControl, envVarControl strict.Control) bool

// StrictControlsByCommand returns a callback function that adds the taken controls as subcontrols for the given `controlNames`.
// Using the given `commandName` as categories.
func StrictControlsByCommand(strictControls strict.Controls, commandName string, controlNames ...string) RegisterStrictControlsFunc {
	return func(flagNameControl, envVarControl strict.Control) bool {
		flagNamesCategory := fmt.Sprintf(controls.CommandFlagsCategoryNameFmt, commandName)
		envVarsCategory := fmt.Sprintf(controls.CommandEnvVarsCategoryNameFmt, commandName)

		return registerStrictControls(strictControls, flagNameControl, envVarControl, flagNamesCategory, envVarsCategory, controlNames...)
	}
}

// StrictControlsByGlobalFlags returns a callback function that adds the taken controls as subcontrols for the given `controlNames`.
// And assigns the "Global Flag" category to these controls.
func StrictControlsByGlobalFlags(strictControls strict.Controls, controlNames ...string) RegisterStrictControlsFunc {
	return func(flagNameControl, envVarControl strict.Control) bool {
		return registerStrictControls(strictControls, flagNameControl, envVarControl, controls.GlobalFlagsCategoryName, controls.GlobalEnvVarsCategoryName, controlNames...)
	}
}

func registerStrictControls(strictControls strict.Controls,
	flagNameControl, envVarControl strict.Control,
	flagNamesCategory, envVarsCategory string,
	controlNames ...string) bool {
	if strictControls == nil {
		return false
	}

	if flagNameControl != nil {
		strictControls.FilterByNames(append(
			controlNames,
			controls.TerragruntPrefixFlags,
			controls.DeprecatedFlags,
		)...).AddSubcontrolsToCategory(flagNamesCategory, flagNameControl)
	}

	if envVarControl != nil {
		strictControls.FilterByNames(append(
			controlNames,
			controls.TerragruntPrefixEnvVars,
			controls.DeprecatedEnvVars,
		)...).AddSubcontrolsToCategory(envVarsCategory, envVarControl)
	}

	return true
}
