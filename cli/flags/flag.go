// Package flags provides tools that are used by all commands to create deprecation flags with strict controls.
package flags

import (
	"context"
	"flag"

	"github.com/gruntwork-io/terragrunt/internal/cli"
)

var _ = cli.Flag(new(Flag))

// Flag is a wrapper for `cli.Flag` that avoids displaying deprecated flags in help, but registers their flag names and environment variables.
type Flag struct {
	cli.Flag
	deprecatedFlags DeprecatedFlags
}

// NewFlag returns a new Flag instance.
func NewFlag(new cli.Flag, opts ...Option) *Flag {
	flag := &Flag{
		Flag: new,
	}

	for _, opt := range opts {
		opt(flag)
	}

	return flag
}

func NewMovedFlag(deprecatedFlag cli.Flag, newCommandName string, regControlsFn RegisterStrictControlsFunc) *Flag {
	return NewFlag(nil, WithDeprecatedMovedFlag(deprecatedFlag, newCommandName, regControlsFn))
}

// TakesValue implements `cli.Flag` interface.
func (new *Flag) TakesValue() bool {
	if new.Flag.Value() == nil {
		return false
	}

	val, ok := new.Flag.Value().Get().(bool)

	return !ok || !val
}

// Value implements `cli.Flag` interface.
func (new *Flag) Value() cli.FlagValue {
	for _, deprecatedFlag := range new.deprecatedFlags {
		if deprecatedFlagValue := deprecatedFlag.Value(); deprecatedFlagValue.IsSet() && deprecatedFlag.newValueFn != nil {
			newValue := deprecatedFlag.newValueFn(deprecatedFlagValue)
			new.Flag.Value().Set(newValue) //nolint:errcheck
		}
	}

	return new.Flag.Value()
}

// Apply implements `cli.Flag` interface.
func (new *Flag) Apply(set *flag.FlagSet) error {
	if err := new.Flag.Apply(set); err != nil {
		return err
	}

	for _, deprecated := range new.deprecatedFlags {
		if deprecated.Flag == new.Flag {
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

// RunAction implements `cli.Flag` interface.
func (new *Flag) RunAction(ctx *cli.Context) error {
	for _, deprecated := range new.deprecatedFlags {
		if err := deprecated.Evaluate(ctx); err != nil {
			return err
		}

		if deprecated.Flag == nil || deprecated.Flag == new.Flag || !deprecated.Value().IsSet() {
			continue
		}

		if err := deprecated.RunAction(ctx); err != nil {
			return err
		}
	}

	if deprecated, ok := new.Flag.(interface {
		Evaluate(ctx context.Context) error
	}); ok {
		if err := deprecated.Evaluate(ctx); err != nil {
			return err
		}
	}

	return new.Flag.RunAction(ctx)
}
