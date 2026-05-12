package flags

import (
	"context"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
)

var _ = clihelper.Flag(new(DeprecatedFlag))

// DeprecatedFlags are multiple of DeprecatedFlag flags.
type DeprecatedFlags []*DeprecatedFlag

// DeprecatedFlag represents a deprecated flag that is not shown in the CLI help, but its names, envVars, are registered.
type DeprecatedFlag struct {
	clihelper.Flag
	newValueFn             NewValueFunc
	controls               strict.Controls
	names                  []string
	envVars                []string
	allowedSubcommandScope bool
}

// GetHidden implements `clihelper.Flag` interface.
func (flag *DeprecatedFlag) GetHidden() bool {
	return true
}

// AllowedSubcommandScope implements `clihelper.Flag` interface.
func (flag *DeprecatedFlag) AllowedSubcommandScope() bool {
	return flag.allowedSubcommandScope
}

// GetEnvVars implements `clihelper.Flag` interface.
func (flag *DeprecatedFlag) GetEnvVars() []string {
	return flag.envVars
}

// Names implements `clihelper.Flag` interface.
func (flag *DeprecatedFlag) Names() []string {
	return flag.names
}

// Evaluate returns an error if the one of the controls is enabled otherwise logs warning messages and returns nil.
func (flag *DeprecatedFlag) Evaluate(ctx context.Context) error {
	return flag.controls.Evaluate(ctx)
}

// SetStrictControls builds the flag-name and env-var strict controls for this
// deprecated flag and wires them into the global deprecated-flags /
// deprecated-env-vars / terragrunt-prefix-* parent controls plus any extra
// parents named in `extraParentNames`. A nil `strictControls` is a no-op.
func (flag *DeprecatedFlag) SetStrictControls(mainFlag *Flag, strictControls strict.Controls, extraParentNames ...string) {
	if strictControls == nil {
		return
	}

	var newValue string

	if flag.newValueFn != nil {
		newValue = flag.newValueFn(nil)
	}

	flagNameControl := controls.NewDeprecatedFlagName(flag, mainFlag, newValue)
	envVarControl := controls.NewDeprecatedEnvVar(flag, mainFlag, newValue)

	flagParents := slices.Concat(extraParentNames, []string{
		controls.TerragruntPrefixFlags,
		controls.DeprecatedFlags,
	})
	strictControls.FilterByNames(flagParents...).AddSubcontrols(flagNameControl)

	envVarParents := slices.Concat(extraParentNames, []string{
		controls.TerragruntPrefixEnvVars,
		controls.DeprecatedEnvVars,
	})
	strictControls.FilterByNames(envVarParents...).AddSubcontrols(envVarControl)

	flag.controls = strict.Controls{flagNameControl, envVarControl}
}

// NewValueFunc represents a function that returns a new value for the current flag if a deprecated flag is called.
// Used when the current flag and the deprecated flag are of different types. For example, the string `log-format` flag
// must be set to `json` when deprecated bool `terragrunt-json-log` flag is used. More examples:
//
// terragrunt-disable-log-formatting  replaced with: log-format=key-value
// terragrunt-json-log                replaced with: log-format=json
// terragrunt-tf-logs-to-json         replaced with: log-format=json
type NewValueFunc func(flagValue clihelper.FlagValue) string

// NewValue returns a callback function that is used to get a new value for the current flag.
func NewValue(val string) NewValueFunc {
	return func(_ clihelper.FlagValue) string {
		return val
	}
}
