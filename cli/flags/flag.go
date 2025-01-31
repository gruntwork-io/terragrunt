// Package flags provides tools that are used by all commands to create deprecation flags with strict controls.
package flags

import (
	"flag"

	"github.com/gruntwork-io/terragrunt/internal/cli"
)

var _ = cli.Flag(new(Flag))

// Flag is a wrapper for `cli.Flag` that avoids displaying deprecated flags in help, but registers their flag names and environment variables.
type Flag struct {
	cli.Flag
	envVars         []string
	deprecatedFlags []*DeprecatedFlag
}

func NewFlag(main cli.Flag, opts ...Option) *Flag {
	flag := &Flag{
		Flag: main,
	}

	for _, opt := range opts {
		opt(flag)
	}

	return flag
}

// TakesValue implements `cli.Flag` interface.
func (main *Flag) TakesValue() bool {
	if main.Flag.Value() == nil {
		return false
	}

	val, ok := main.Flag.Value().Get().(bool)

	return !ok || !val
}

// GetEnvVars implements `cli.Flag` interface.
func (main *Flag) GetEnvVars() []string {
	if len(main.envVars) > 0 {
		return main.envVars
	}

	return main.Flag.GetEnvVars()
}

func (main *Flag) Value() cli.FlagValue {
	for _, depreactedFlag := range main.deprecatedFlags {
		if depreactedFlagValue := depreactedFlag.Value(); depreactedFlagValue.IsSet() && depreactedFlag.newValueFn != nil {
			newValue := depreactedFlag.newValueFn(depreactedFlagValue)
			main.Flag.Value().Set(newValue) //nolint:errcheck
		}
	}

	return main.Flag.Value()
}

func (main *Flag) Apply(set *flag.FlagSet) error {
	if err := main.Flag.Apply(set); err != nil {
		return err
	}

	for _, deprecated := range main.deprecatedFlags {
		if deprecated.Flag == main.Flag {
			if err := cli.ApplyFlag(deprecated, set); err != nil {
				return err
			}

			continue
		}

		if err := deprecated.Flag.Apply(set); err != nil {
			return err
		}
	}

	return nil
}

func (main *Flag) RunAction(ctx *cli.Context) error {
	for _, deprecated := range main.deprecatedFlags {
		if err := deprecated.controls.Evaluate(ctx); err != nil {
			return err
		}

		if deprecated.Flag == nil || deprecated.Flag == main.Flag || !deprecated.Value().IsSet() {
			continue
		}

		if err := deprecated.RunAction(ctx); err != nil {
			return err
		}
	}

	return main.Flag.RunAction(ctx)
}

type Option func(*Flag)

func WithDeprecatedFlag(deprecatedFlag cli.Flag, newValueFn NewValueFunc, applyControlsFn ApplyStrictControlsFunc) Option {
	return func(main *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:       deprecatedFlag,
			newValueFn: newValueFn,
		}
		deprecatedFlag.SetStrictControls(main, applyControlsFn)

		main.deprecatedFlags = append(main.deprecatedFlags, deprecatedFlag)
	}
}

func WithDeprecatedPrefix(prefix Prefix, applyControlsFn ApplyStrictControlsFunc) Option {
	return func(main *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:    main.Flag,
			names:   prefix.FlagNames(main.Flag.Names()...),
			envVars: prefix.EnvVars(main.Flag.Names()...),
		}
		deprecatedFlag.SetStrictControls(main, applyControlsFn)

		main.deprecatedFlags = append(main.deprecatedFlags, deprecatedFlag)
	}
}

func WithDeprecatedNames(flagNames []string, applyControlsFn ApplyStrictControlsFunc) Option {
	return func(main *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:    main.Flag,
			names:   Prefix{}.FlagNames(flagNames...),
			envVars: Prefix{}.EnvVars(flagNames...),
		}
		deprecatedFlag.SetStrictControls(main, applyControlsFn)

		main.deprecatedFlags = append(main.deprecatedFlags, deprecatedFlag)
	}
}

func WithDeprecatedName(flagName string, applyControlsFn ApplyStrictControlsFunc) Option {
	return func(main *Flag) {
		WithDeprecatedNames([]string{flagName}, applyControlsFn)(main)
	}
}

func WithDeprecatedNamesEnvVars(flagNames, envVars []string, applyControlsFn ApplyStrictControlsFunc) Option {
	return func(main *Flag) {
		deprecatedFlag := &DeprecatedFlag{
			Flag:    main.Flag,
			names:   flagNames,
			envVars: envVars,
		}
		deprecatedFlag.SetStrictControls(main, applyControlsFn)

		main.deprecatedFlags = append(main.deprecatedFlags, deprecatedFlag)
	}
}
