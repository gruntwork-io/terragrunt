package cli

import (
	libflag "flag"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/urfave/cli/v2"
)

// BoolFlag implements Flag
var _ Flag = new(BoolFlag)

// BoolActionFunc is the action to execute when the flag has been set either via a flag or via an environment variable.
type BoolActionFunc[T bool] func(ctx *Context, value T) error

type BoolFlag struct {
	flag

	// The name of the flag.
	Name string
	// The default value of the flag to display in the help, if it is empty, the value is taken from `Destination`.
	DefaultText string
	// A short usage description to display in help.
	Usage string
	// Aliases are usually used for the short flag name, like `-h`.
	Aliases []string
	// The name of the env variable that is parsed and assigned to `Destination` before the flag value.
	EnvVar string
	// The action to execute when flag is specified
	Action BoolActionFunc[bool]
	// The pointer to which the value of the flag or env var is assigned.
	// It also uses as the default value displayed in the help.
	Destination *bool
	// If set to true, then the assigned flag value will be inverted
	Negative bool
	// Hidden hides the flag from the help, if set to true.
	Hidden bool
}

// Apply applies Flag settings to the given flag set.
func (flag *BoolFlag) Apply(set *libflag.FlagSet) error {
	if flag.Destination == nil {
		flag.Destination = new(bool)
	}

	var (
		err      error
		envValue *string
	)

	valType := FlagType[bool](&boolFlagType{negative: flag.Negative})

	if val := flag.LookupEnv(flag.EnvVar); val != nil && *val != "" {
		envValue = val
	}

	if flag.FlagValue, err = newGenericValue(valType, envValue, flag.Destination); err != nil {
		if envValue != nil {
			return errors.Errorf("invalid boolean value %q for %s: %w", *envValue, flag.EnvVar, err)
		}

		return err
	}

	for _, name := range flag.Names() {
		set.Var(flag.FlagValue, name, flag.Usage)
	}

	return nil
}

// GetHidden returns true if the flag should be hidden from the help.
func (flag *BoolFlag) GetHidden() bool {
	return flag.Hidden
}

// GetUsage returns the usage string for the flag.
func (flag *BoolFlag) GetUsage() string {
	return flag.Usage
}

// GetEnvVars returns the env vars for this flag.
func (flag *BoolFlag) GetEnvVars() []string {
	if flag.EnvVar == "" {
		return nil
	}

	return []string{flag.EnvVar}
}

// GetDefaultText returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *BoolFlag) GetDefaultText() string {
	if flag.DefaultText == "" && flag.FlagValue != nil {
		return flag.FlagValue.GetDefaultText()
	}

	return flag.DefaultText
}

// String returns a readable representation of this value (for usage defaults).
func (flag *BoolFlag) String() string {
	return cli.FlagStringer(flag)
}

// Names returns the names of the flag.
func (flag *BoolFlag) Names() []string {
	return append([]string{flag.Name}, flag.Aliases...)
}

// RunAction implements ActionableFlag.RunAction
func (flag *BoolFlag) RunAction(ctx *Context) error {
	if flag.Action != nil {
		return flag.Action(ctx, *flag.Destination)
	}

	return nil
}

// -- bool Flag Type
type boolFlagType struct {
	*genericType[bool]
	negative bool
}

func (val *boolFlagType) Clone(dest *bool) FlagType[bool] {
	return &boolFlagType{
		genericType: &genericType[bool]{dest: dest},
		negative:    val.negative,
	}
}

func (val *boolFlagType) Set(str string) error {
	if err := val.genericType.Set(str); err != nil {
		return err
	}

	if val.negative {
		*val.genericType.dest = !*val.genericType.dest
	}

	return nil
}
