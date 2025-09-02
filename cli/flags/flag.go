// Package flags provides tools that are used by all commands to create deprecation flags with strict controls.
package flags

import (
	"context"
	"flag"
	"io"
	"strconv"
	"strings"

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

// TakesValue implements `github.com/urfave/cli.DocGenerationFlag` required to generate help.
// TakesValue returns `true` for all flags except boolean ones that are `false` or `true` inverted.
func (newFlag *Flag) TakesValue() bool {
	if newFlag.Flag.Value() == nil {
		return false
	}

	val, ok := newFlag.Flag.Value().Get().(bool)

	if newFlag.Flag.Value().IsNegativeBoolFlag() {
		val = !val
	}

	return !ok || !val
}

// DeprecatedNames returns all deprecated names for this flag.
func (newFlag *Flag) DeprecatedNames() []string {
	var names []string

	if flag, ok := newFlag.Flag.(interface{ DeprecatedNames() []string }); ok {
		names = flag.DeprecatedNames()
	}

	for _, deprecated := range newFlag.deprecatedFlags {
		names = append(names, deprecated.Names()...)
	}

	return names
}

// Value implements `cli.Flag` interface.
func (newFlag *Flag) Value() cli.FlagValue {
	for _, deprecatedFlag := range newFlag.deprecatedFlags {
		if deprecatedFlag.Flag == newFlag.Flag {
			continue
		}

		if deprecatedFlagValue := deprecatedFlag.Value(); deprecatedFlagValue != nil && deprecatedFlagValue.IsSet() {
			newValue := deprecatedFlagValue.String()

			if newFlag.Flag.Value().IsNegativeBoolFlag() && deprecatedFlagValue.IsBoolFlag() {
				if v, ok := deprecatedFlagValue.Get().(bool); ok {
					newValue = strconv.FormatBool(!v)
				}
			}

			if deprecatedFlag.newValueFn != nil {
				newValue = deprecatedFlag.newValueFn(deprecatedFlagValue)
			}

			newFlag.Flag.Value().Getter(deprecatedFlagValue.GetName()).Set(newValue) //nolint:errcheck
		}
	}

	return newFlag.Flag.Value()
}

// Apply implements `cli.Flag` interface.
func (newFlag *Flag) Apply(set *flag.FlagSet) error {
	if err := newFlag.Flag.Apply(set); err != nil {
		return err
	}

	for _, deprecated := range newFlag.deprecatedFlags {
		if deprecated.Flag == newFlag.Flag {
			if err := cli.ApplyFlag(deprecated, set); err != nil {
				return err
			}

			continue
		}

		if err := deprecated.Apply(set); err != nil {
			return err
		}
	}

	return nil
}

// RunAction implements `cli.Flag` interface.
func (newFlag *Flag) RunAction(ctx *cli.Context) error {
	for _, deprecated := range newFlag.deprecatedFlags {
		if err := newFlag.evaluateWrapper(ctx, deprecated.Evaluate); err != nil {
			return err
		}

		if deprecated.Flag == nil || deprecated.Flag == newFlag.Flag || !deprecated.Value().IsSet() {
			continue
		}

		if err := deprecated.RunAction(ctx); err != nil {
			return err
		}
	}

	if deprecated, ok := newFlag.Flag.(interface {
		Evaluate(ctx context.Context) error
	}); ok {
		if err := newFlag.evaluateWrapper(ctx, deprecated.Evaluate); err != nil {
			return err
		}
	}

	return newFlag.Flag.RunAction(ctx)
}

// Parse parses the given `args` for the flag value and env vars values specified in the flag.
// The value will be assigned to the `Destination` field.
// The value can also be retrieved using `flag.Value().Get()`.
func (newFlag *Flag) Parse(args cli.Args) error {
	flagSet := flag.NewFlagSet("", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	if err := newFlag.Apply(flagSet); err != nil {
		return err
	}

	const maxFlagsParse = 1000 // Maximum flags parse

	for range maxFlagsParse {
		err := flagSet.Parse(args)
		if err == nil {
			break
		}

		if errStr := err.Error(); !strings.HasPrefix(errStr, cli.ErrMsgFlagUndefined) {
			break
		}

		args = flagSet.Args()
	}

	return nil
}
