package flags

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
)

var _ = cli.Flag(new(DeprecatedFlag))

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

// Names returns the names of the flag.
func (flag *DeprecatedFlag) Names() []string {
	if len(flag.names) == 0 && flag.Flag != nil {
		return flag.Flag.Names()
	}

	return flag.names
}

func (flag *DeprecatedFlag) SetStrictControls(mainFlag *Flag, applyControlsFn ApplyStrictControlsFunc) {
	var newValue string

	if flag.newValueFn != nil {
		newValue = flag.newValueFn(nil)
	}

	flagNameControl := controls.NewDeprecatedFlagName(flag, mainFlag, newValue)
	envVarControl := controls.NewDeprecatedEnvVar(flag, mainFlag, newValue)

	flag.controls = strict.Controls{flagNameControl, envVarControl}

	applyControlsFn(flagNameControl, envVarControl)
}

type NewValueFunc func(flagValue cli.FlagValue) string

func NewValue(val string) NewValueFunc {
	return func(_ cli.FlagValue) string {
		return val
	}
}

type ApplyStrictControlsFunc func(flagNameControl, envVarControl strict.Control)

func StrictControlsByGroup(strcitControls strict.Controls, commandName string, controlNames ...string) ApplyStrictControlsFunc {
	return func(flagNameControl, envVarControl strict.Control) {
		flagNamesCategory := fmt.Sprintf(controls.CommandFlagsCategoryNameFmt, commandName)
		envVarsCategory := fmt.Sprintf(controls.CommandEnvVarsCategoryNameFmt, commandName)

		strcitControls.FilterByNames(append(controlNames, controls.DeprecatedFlags)...).AddSubcontrolsToCategory(flagNamesCategory, flagNameControl)
		strcitControls.FilterByNames(append(controlNames, controls.DeprecatedEnvVars)...).AddSubcontrolsToCategory(envVarsCategory, envVarControl)
	}
}

func StrictControls(strcitControls strict.Controls, controlNames ...string) ApplyStrictControlsFunc {
	return func(flagNameControl, envVarControl strict.Control) {
		strcitControls.FilterByNames(append(controlNames, controls.DeprecatedFlags)...).AddSubcontrolsToCategory(controls.GlobalFlagsCategoryName, flagNameControl)
		strcitControls.FilterByNames(append(controlNames, controls.DeprecatedEnvVars)...).AddSubcontrolsToCategory(controls.GlobalEnvVarsCategoryName, envVarControl)
	}
}
