package flags

import (
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	DeprecatedEnvVarPrefix   = "TERRAGRUNT_"
	DeprecatedFlagNamePrefix = "terragrunt-"
)

// BoolWithDeprecatedFlag adds deprecated names with strict mode control for the given flag.
// If `oldNames` is not specified, names are derived from the given `flag` value with added `terragrunt-/TERRAGRUNT_` prefixes.
func BoolWithDeprecatedFlag(opts *options.TerragruntOptions, flag *cli.BoolFlag, oldNames ...string) *Flag {
	names := flag.Names()
	envVars := flag.GetEnvVars()

	flag.Aliases = append(flag.Aliases, deprecatedFlagNames(flag.Name, oldNames)...)
	flag.EnvVars = append(flag.EnvVars, deprecatedEnvVars(flag.Name, oldNames)...)

	return &Flag{
		opts:    opts,
		Flag:    flag,
		names:   names,
		envVars: envVars,
	}
}

// GenericWithDeprecatedFlag adds deprecated names with strict mode control for the given flag.
// If `oldNames` is not specified, names are derived from the given `flag` value with added `terragrunt-/TERRAGRUNT_` prefixes.
func GenericWithDeprecatedFlag[T cli.GenericType](opts *options.TerragruntOptions, flag *cli.GenericFlag[T], oldNames ...string) *Flag {
	names := flag.Names()
	envVars := flag.GetEnvVars()

	flag.Aliases = append(flag.Aliases, deprecatedFlagNames(flag.Name, oldNames)...)
	flag.EnvVars = append(flag.EnvVars, deprecatedEnvVars(flag.Name, oldNames)...)

	return &Flag{
		opts:    opts,
		Flag:    flag,
		names:   names,
		envVars: envVars,
	}
}

// SliceWithDeprecatedFlag adds deprecated names with strict mode control for the given flag.
// If `oldNames` is not specified, names are derived from the given `flag` value with added `terragrunt-/TERRAGRUNT_` prefixes.
func SliceWithDeprecatedFlag[T cli.SliceFlagType](opts *options.TerragruntOptions, flag *cli.SliceFlag[T], oldNames ...string) *Flag {
	names := flag.Names()
	envVars := flag.GetEnvVars()

	flag.Aliases = append(flag.Aliases, deprecatedFlagNames(flag.Name, oldNames)...)
	flag.EnvVars = append(flag.EnvVars, deprecatedEnvVars(flag.Name, oldNames)...)

	return &Flag{
		opts:    opts,
		Flag:    flag,
		names:   names,
		envVars: envVars,
	}
}

// MapWithDeprecatedFlag adds deprecated names with strict mode control for the given flag.
// If `oldNames` is not specified, names are derived from the given `flag` value with added `terragrunt-/TERRAGRUNT_` prefixes.
func MapWithDeprecatedFlag[K cli.MapFlagKeyType, V cli.MapFlagValueType](opts *options.TerragruntOptions, flag *cli.MapFlag[K, V], oldNames ...string) *Flag {
	names := flag.Names()
	envVars := flag.GetEnvVars()

	flag.Aliases = append(flag.Aliases, deprecatedFlagNames(flag.Name, oldNames)...)
	flag.EnvVars = append(flag.EnvVars, deprecatedEnvVars(flag.Name, oldNames)...)

	return &Flag{
		opts:    opts,
		Flag:    flag,
		names:   names,
		envVars: envVars,
	}
}

// Flag is a wrapper for `cli.Flag` to avoid displaying deprecated names in help.
type Flag struct {
	cli.Flag
	opts    *options.TerragruntOptions
	names   []string
	envVars []string
}

// String returns a readable representation of this value (for usage defaults).
func (flag *Flag) String() string {
	return cli.FlagStringer(flag)
}

// GetEnvVars returns flag envs without deprecated renamedNames to avoid showing them in help.
func (flag *Flag) GetEnvVars() []string {
	return flag.envVars
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
		if _, _, err := control.Evaluate(flag.opts); err != nil {
			strictControl = true
		}
	}

	if flagName := flag.usedDeprecatedFlagName(ctx); flagName != "" && len(flag.names) > 0 {
		if strictControl {
			return errors.Errorf("`--%s` flag is no longer supported, use `--%s` instead", flagName, flag.names[0])
		}

		flag.opts.Logger.Warnf("The `--%s` flag is deprecated and will be removed in a future version. Use `--%s` instead.", flagName, flag.names[0])
	}

	if envVar := flag.usedDeprecatedEnvVar(ctx); envVar != "" && len(flag.envVars) > 0 {
		if strictControl {
			return errors.Errorf("`%s` environment variable is no longer supported, use `%s` instead", envVar, flag.envVars[0])
		}
	}

	return nil
}

// usedDeprecatedFlagName returns the first deprecated flag found if any, otherwise it returns an empty string.
func (flag *Flag) usedDeprecatedFlagName(ctx *cli.Context) string {
	args := util.RemoveSublistFromList(ctx.Parent().Args(), ctx.Args())
	deprecatedNames := util.RemoveSublistFromList(flag.Flag.Names(), flag.names)

	for _, arg := range args {
		substringNumber := 2
		arg = strings.SplitN(arg, "=", substringNumber)[0]

		flagName := strings.TrimLeft(arg, "-")

		if util.ListContainsElement(deprecatedNames, flagName) {
			return flagName
		}
	}

	return ""
}

// usedDeprecatedEnvVar returns the first deprecated env var if any, otherwise an empty string.
func (flag *Flag) usedDeprecatedEnvVar(_ *cli.Context) string {
	deprecatedEnvVars := util.RemoveSublistFromList(flag.Flag.GetEnvVars(), flag.envVars)

	for _, envVar := range deprecatedEnvVars {
		if _, ok := os.LookupEnv(envVar); ok {
			return envVar
		}
	}

	return ""
}

func deprecatedEnvVars(flagName string, oldNames []string) []string {
	if len(oldNames) > 0 {
		return EnvVarsWithPrefix("", oldNames...)
	}

	return EnvVarsWithPrefix(DeprecatedEnvVarPrefix, flagName)
}

func deprecatedFlagNames(flagName string, oldNames []string) []string {
	if len(oldNames) > 0 {
		return oldNames
	}

	return FlagNames(DeprecatedFlagNamePrefix, flagName)
}
