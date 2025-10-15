package flags

import (
	"github.com/gruntwork-io/terragrunt/internal/cli"
)

// Option is used to set options to the `Flag`.
type Option func(*Flag)

// WithDeprecatedFlag returns an `Option` that will register the given `deprecatedFlag` as a deprecated flag.
// `newValueFn` is called to get a value for the new flag when this deprecated flag triggers. For example:
//
//	NewFlag(&cli.GenericFlag[string]{
//	  Name:    "log-format",
//	}, WithDeprecatedFlag(&cli.BoolFlag{
//	  Name:    "terragrunt-json-log",
//	}, flags.NewValue("json"), nil))
func WithDeprecatedFlag(deprecatedFlag cli.Flag, newValueFn NewValueFunc, regControlsFn RegisterStrictControlsFunc) Option {
	return func(newFlag *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:                   deprecatedFlag,
			newValueFn:             newValueFn,
			allowedSubcommandScope: true,
		}
		deprecatedFlag.SetStrictControls(newFlag, regControlsFn)

		newFlag.deprecatedFlags = append(newFlag.deprecatedFlags, deprecatedFlag)
	}
}

// WithDeprecatedPrefix returns an `Option` that will create a deprecated flag with the same name as the new flag,
// but with the specified `prefix` prepended to the names and environment variables.
// Should be used with caution, as changing the name of the new flag will change the name of the deprecated flag.
// For example:
//
//	NewFlag(&cli.GenericFlag[string]{
//	  Name:    "no-color",
//	  Aliases: []string{"disable-color"},
//	  EnvVars: []string{"NO_COLOR","DISABLE_COLOR"},
//	}, WithDeprecatedPrefix(Prefix{"terragrunt"}, nil))
//
// The deprecated flag will have "terragrunt-no-color","terragrunt-disable-color" names and "TERRAGRUNT_NO_COLOR","TERRAGRUNT_DISABLE_COLOR" env vars.
// NOTE: This function is currently unused but retained for future flag deprecation needs.
func WithDeprecatedPrefix(prefix Prefix, regControlsFn RegisterStrictControlsFunc) Option {
	return func(newFlag *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:                   newFlag.Flag,
			names:                  prefix.FlagNames(newFlag.Names()...),
			envVars:                prefix.EnvVars(newFlag.Names()...),
			allowedSubcommandScope: true,
		}
		deprecatedFlag.SetStrictControls(newFlag, regControlsFn)

		newFlag.deprecatedFlags = append(newFlag.deprecatedFlags, deprecatedFlag)
	}
}

// WithDeprecatedNames returns an `Option` that will create a deprecated flag.
// The given `flagNames` names will assign both names (converting to lowercase,dash)
// and env vars (converting to uppercase,underscore). For example:
//
// WithDeprecatedNames([]string{"NO_COLOR", "working-dir"}, nil)
//
// The deprecated flag will have "no-color","working-dir" names and "NO_COLOR","WORKING_DIR" env vars.
func WithDeprecatedNames(flagNames []string, regControlsFn RegisterStrictControlsFunc) Option {
	return func(newFlag *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:                   newFlag.Flag,
			names:                  Prefix{}.FlagNames(flagNames...),
			envVars:                Prefix{}.EnvVars(flagNames...),
			allowedSubcommandScope: true,
		}
		deprecatedFlag.SetStrictControls(newFlag, regControlsFn)

		newFlag.deprecatedFlags = append(newFlag.deprecatedFlags, deprecatedFlag)
	}
}

// WithDeprecatedName does the same as `WithDeprecatedNames`, but with a single name.
func WithDeprecatedName(flagName string, regControlsFn RegisterStrictControlsFunc) Option {
	return func(newFlag *Flag) {
		WithDeprecatedNames([]string{flagName}, regControlsFn)(newFlag)
	}
}

// WithDeprecatedNamesEnvVars returns an `Option` that will create a deprecated flag,
// with the given `flagNames`, `envVars` assigned to the flag names and environment variables as is.
func WithDeprecatedNamesEnvVars(flagNames, envVars []string, regControlsFn RegisterStrictControlsFunc) Option {
	return func(newFlag *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:                   newFlag.Flag,
			names:                  flagNames,
			envVars:                envVars,
			allowedSubcommandScope: true,
		}
		deprecatedFlag.SetStrictControls(newFlag, regControlsFn)

		newFlag.deprecatedFlags = append(newFlag.deprecatedFlags, deprecatedFlag)
	}
}

// WithDeprecatedEnvVars returns an `Option` that will create a flag with the given deprecated env vars.
func WithDeprecatedEnvVars(envVars []string, regControlsFn RegisterStrictControlsFunc) Option {
	return func(newFlag *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:                   newFlag.Flag,
			envVars:                envVars,
			allowedSubcommandScope: true,
		}
		deprecatedFlag.SetStrictControls(newFlag, regControlsFn)

		newFlag.deprecatedFlags = append(newFlag.deprecatedFlags, deprecatedFlag)
	}
}

// WithDeprecatedFlagNames returns an `Option` that will create a flag with the given deprecated flag names.
func WithDeprecatedFlagNames(flagNames []string, regControlsFn RegisterStrictControlsFunc) Option {
	return func(newFlag *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:                   newFlag.Flag,
			names:                  flagNames,
			allowedSubcommandScope: true,
		}
		deprecatedFlag.SetStrictControls(newFlag, regControlsFn)

		newFlag.deprecatedFlags = append(newFlag.deprecatedFlags, deprecatedFlag)
	}
}

// WithDeprecatedFlagName does the same as `WithDeprecatedFlagNames`, but with a single name.
func WithDeprecatedFlagName(flagName string, regControlsFn RegisterStrictControlsFunc) Option {
	return func(newFlag *Flag) {
		WithDeprecatedFlagNames([]string{flagName}, regControlsFn)(newFlag)
	}
}

// WithEvaluateWrapper returns an Option that wraps the strict control `Evaluate(ctx context.Context)` function.
func WithEvaluateWrapper(fn EvaluateWrapperFunc) Option {
	return func(newFlag *Flag) {
		newFlag.evaluateWrapper = fn
	}
}
