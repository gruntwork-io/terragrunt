package flags

import (
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	EnvVarPrefix = "TG_"

	DeprecatedEnvVarPrefix   = "TERRAGRUNT_"
	DeprecatedFlagNamePrefix = "terragrunt-"
)

// NewBoolFlag wraps the given flag to detect the use of deprecated flag/environment renamedNames and creates an env with the `TG_` prefix.
func NewBoolFlag(opts *options.TerragruntOptions, flag *cli.BoolFlag, renamedNames ...string) cli.Flag {
	names := flag.Names()
	renamedNames = util.RemoveDuplicatesFromList(append(renamedNames, flag.Name))

	envVar := autoEnvVar(flag.Name)
	deprecatedNames, deprecatedEnvVars := deprecatedNames(renamedNames)

	flag.EnvVars = append(flag.EnvVars, append(deprecatedEnvVars, envVar)...)
	flag.Aliases = append(flag.Aliases, deprecatedNames...)

	return &Flag{
		opts:              opts,
		Flag:              flag,
		names:             names,
		envVar:            envVar,
		deprecatedNames:   deprecatedNames,
		deprecatedEnvVars: deprecatedEnvVars,
	}
}

// NewGenericFlag wraps the given flag to detect the use of deprecated flag/environment renamedNames and automatically create an env with the `TG_` prefix.
func NewGenericFlag[T cli.GenericType](opts *options.TerragruntOptions, flag *cli.GenericFlag[T], renamedNames ...string) cli.Flag {
	names := flag.Names()
	renamedNames = util.RemoveDuplicatesFromList(append(renamedNames, flag.Name))

	envVar := autoEnvVar(flag.Name)
	deprecatedNames, deprecatedEnvVars := deprecatedNames(renamedNames)

	flag.EnvVars = append(flag.EnvVars, append(deprecatedEnvVars, envVar)...)
	flag.Aliases = append(flag.Aliases, deprecatedNames...)

	return &Flag{
		opts:              opts,
		Flag:              flag,
		names:             names,
		envVar:            envVar,
		deprecatedNames:   deprecatedNames,
		deprecatedEnvVars: deprecatedEnvVars,
	}
}

// NewSliceFlag wraps the given flag to detect the use of deprecated flag/environment renamedNames and automatically create an env with the `TG_` prefix.
func NewSliceFlag[T cli.SliceFlagType](opts *options.TerragruntOptions, flag *cli.SliceFlag[T], renamedNames ...string) cli.Flag {
	names := flag.Names()
	renamedNames = util.RemoveDuplicatesFromList(append(renamedNames, flag.Name))

	envVar := autoEnvVar(flag.Name)
	deprecatedNames, deprecatedEnvVars := deprecatedNames(renamedNames)

	flag.EnvVars = append(flag.EnvVars, append(deprecatedEnvVars, envVar)...)
	flag.Aliases = append(flag.Aliases, deprecatedNames...)

	return &Flag{
		opts:              opts,
		Flag:              flag,
		names:             names,
		envVar:            envVar,
		deprecatedNames:   deprecatedNames,
		deprecatedEnvVars: deprecatedEnvVars,
	}
}

// NewMapFlag wraps the given flag to detect the use of deprecated flag/environment renamedNames and automatically create an env with the `TG_` prefix.
func NewMapFlag[K cli.MapFlagKeyType, V cli.MapFlagValueType](opts *options.TerragruntOptions, flag *cli.MapFlag[K, V], renamedNames ...string) cli.Flag {
	names := flag.Names()
	renamedNames = util.RemoveDuplicatesFromList(append(renamedNames, flag.Name))

	envVar := autoEnvVar(flag.Name)
	deprecatedNames, deprecatedEnvVars := deprecatedNames(renamedNames)

	flag.EnvVars = append(flag.EnvVars, append(deprecatedEnvVars, envVar)...)
	flag.Aliases = append(flag.Aliases, deprecatedNames...)

	return &Flag{
		opts:              opts,
		Flag:              flag,
		names:             names,
		envVar:            envVar,
		deprecatedNames:   deprecatedNames,
		deprecatedEnvVars: deprecatedEnvVars,
	}
}

// Flag is a flag wrapper that creates an env variable with prefix `TG_` from the flag name
// and checks for deprecated flag renamedNames/envs usage with prefixes `--terragrunt`/`TERRAGRUNT_`.
type Flag struct {
	cli.Flag
	opts              *options.TerragruntOptions
	names             []string
	envVar            string
	deprecatedNames   []string
	deprecatedEnvVars []string
}

// String returns a readable representation of this value (for usage defaults).
func (flag *Flag) String() string {
	return cli.FlagStringer(flag)
}

// GetEnvVars returns flag envs without deprecated renamedNames to avoid showing them in help.
func (flag *Flag) GetEnvVars() []string {
	return []string{flag.envVar}
}

// Names returns flag renamedNames without deprecated renamedNames to avoid showing them in help.
func (flag *Flag) Names() []string {
	return flag.names
}

// RunAction checks for use of deprecated flag renamedNames/envs and runs the inherited `RunAction` function.
func (flag *Flag) RunAction(ctx *cli.Context) error {
	if err := flag.Flag.RunAction(ctx); err != nil {
		return err
	}

	var strictControl bool

	if control, ok := strict.GetStrictControl(strict.RenamedFlag); ok {
		if _, err := control.Evaluate(flag.opts); err != nil {
			strictControl = true
		}
	}

	if flagName := flag.usedDeprecatedFlagName(ctx); flagName != "" {
		if strictControl {
			return errors.Errorf("`--%s` flag is no longer supported, use `--%s` instead.", flagName, flag.names[0])
		}

		flag.opts.Logger.Warnf("The `--%s` flag is deprecated and will be removed in a future version. Use `--%s` instead.", flagName, flag.names[0])
	}

	if envVar := flag.usedDeprecatedEnvVar(ctx); envVar != "" {
		if strictControl {
			return errors.Errorf("`%s` environment variable is no longer supported, use `%s` instead.", envVar, flag.envVar)
		}

		flag.opts.Logger.Warnf("The `%s` environment variable is deprecated and will be removed in a future version. Use `%s` instead.", envVar, flag.envVar)
	}

	return nil
}

// usedDeprecatedFlagName returns the deprecated flag if used, otherwise an empty string.
func (flag *Flag) usedDeprecatedFlagName(ctx *cli.Context) string {
	args := util.RemoveSublistFromList(ctx.Parent().Args(), ctx.Args())

	for _, arg := range args {
		substringNumber := 2
		arg = strings.SplitN(arg, "=", substringNumber)[0]

		flagName := strings.TrimLeft(arg, "-")

		if util.ListContainsElement(flag.deprecatedNames, flagName) {
			return flagName
		}
	}

	return ""
}

// usedDeprecatedEnvVar returns the deprecated env var if used, otherwise an empty string.
func (flag *Flag) usedDeprecatedEnvVar(_ *cli.Context) string {
	for _, envVar := range flag.deprecatedEnvVars {
		if _, ok := os.LookupEnv(envVar); ok {
			return envVar
		}
	}

	return ""
}

// flagNames returns an auto created environment variable and deprecated flag renamedNames/envs.
func autoEnvVar(name string) string {
	envVarSuffix := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return EnvVarPrefix + envVarSuffix
}

func deprecatedNames(names []string) ([]string, []string) {
	var (
		deprecatedNames   = make([]string, len(names))
		deprecatedEnvVars = make([]string, len(names))
	)

	for i := range names {
		deprecatedEnvVars[i] = DeprecatedEnvVarPrefix + strings.ToUpper(strings.ReplaceAll(names[i], "-", "_"))
		deprecatedNames[i] = DeprecatedFlagNamePrefix + names[i]
	}

	return deprecatedNames, deprecatedEnvVars
}
