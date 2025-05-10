// Package flags provides tools that are used by all commands to create deprecation flags with strict controls.
package flags

import (
	"context"
	libflag "flag"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/cli"
)

var _ = cli.Flag(new(Flag))

// EvaluateWrapperFunc represents a function that is used to wrap the `Evaluate(ctx context.Context) error` strict control method.
// Which can be passed as an option `WithEvaluateWrapper` to `NewFlag(...)` to control the behavior of strict control evaluation.
type EvaluateWrapperFunc func(ctx context.Context, evalFn func(ctx context.Context) error) error

// Flag is a wrapper for `cli.Flag` that avoids displaying deprecated flags in help, but registers their flag names and environment variables.
type Flag struct {
	cli.Flag
	evaluateWrapper EvaluateWrapperFunc
	deprecatedFlags DeprecatedFlags
}

// NewFlag returns a new Flag instance.
func NewFlag(new cli.Flag, opts ...Option) *Flag {
	flag := &Flag{
		Flag: new,
		evaluateWrapper: func(ctx context.Context, evalFn func(ctx context.Context) error) error {
			return evalFn(ctx)
		},
	}

	for _, opt := range opts {
		opt(flag)
	}

	return flag
}

func NewMovedFlag(deprecatedFlag cli.Flag, newCommandName string, regControlsFn RegisterStrictControlsFunc, opts ...Option) *Flag {
	return NewFlag(nil, append(opts, WithDeprecatedMovedFlag(deprecatedFlag, newCommandName, regControlsFn))...)
}

// TakesValue implements `github.com/urfave/cli.DocGenerationFlag` required to generate help.
// TakesValue returns `true` for all flags except boolean ones that are `false` or `true` inverted.
func (flag *Flag) TakesValue() bool {
	if flag.Flag.Value() == nil {
		return false
	}

	val, ok := flag.Flag.Value().Get().(bool)

	if flag.Flag.Value().IsNegativeBoolFlag() {
		val = !val
	}

	return !ok || !val
}

// DeprecatedNames returns all deprecated names for this flag.
func (flag *Flag) DeprecatedNames() []string {
	var names []string

	if flag, ok := flag.Flag.(interface{ DeprecatedNames() []string }); ok {
		names = flag.DeprecatedNames()
	}

	for _, deprecated := range flag.deprecatedFlags {
		names = append(names, deprecated.Names()...)
	}

	return names
}

// Value implements `cli.Flag` interface.
func (flag *Flag) Value() cli.FlagValue {
	for _, deprecatedFlag := range flag.deprecatedFlags {
		if deprecatedFlag.Flag == flag.Flag {
			continue
		}

		if deprecatedFlagValue := deprecatedFlag.Value(); deprecatedFlagValue != nil && deprecatedFlagValue.IsSet() {
			newValue := deprecatedFlagValue.String()

			if flag.Flag.Value().IsNegativeBoolFlag() && deprecatedFlagValue.IsBoolFlag() {
				if v, ok := deprecatedFlagValue.Get().(bool); ok {
					newValue = strconv.FormatBool(!v)
				}
			}

			if deprecatedFlag.newValueFn != nil {
				newValue = deprecatedFlag.newValueFn(deprecatedFlagValue)
			}

			flag.Flag.Value().Getter(deprecatedFlagValue.SourceName(), cli.FlagValueSourceArg).Set(newValue) //nolint:errcheck
		}
	}

	return flag.Flag.Value()
}

// Apply implements `cli.Flag` interface.
func (flag *Flag) Apply(set *libflag.FlagSet) error {
	if err := flag.Flag.Apply(set); err != nil {
		return err
	}

	for _, deprecated := range flag.deprecatedFlags {
		if deprecated.Flag == flag.Flag {
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
func (flag *Flag) RunAction(ctx *cli.Context) error {
	for _, deprecated := range flag.deprecatedFlags {
		if err := flag.evaluateWrapper(ctx, deprecated.Evaluate); err != nil {
			return err
		}

		if deprecated.Flag == nil || deprecated.Flag == flag.Flag || !deprecated.Value().IsSet() {
			continue
		}

		if err := deprecated.RunAction(ctx); err != nil {
			return err
		}
	}

	if deprecated, ok := flag.Flag.(interface {
		Evaluate(ctx context.Context) error
	}); ok {
		if err := flag.evaluateWrapper(ctx, deprecated.Evaluate); err != nil {
			return err
		}
	}

	return flag.Flag.RunAction(ctx)
}
