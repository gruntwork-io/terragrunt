package cli

import (
	libflag "flag"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/urfave/cli/v2"
)

// BoolFlag implements Flag
var _ Flag = new(BoolFlag)

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
	// The names of the env variables that are parsed and assigned to `Destination` before the flag value.
	EnvVars []string
	// Action is a function that is called when the flag is specified. It is executed only after all command flags have been parsed.
	Action FlagActionFunc[bool]
	// FlagSetterFunc represents function type that is called when the flag is specified.
	// Executed during value parsing, in case of an error the returned error is wrapped with the flag or environment variable name.
	Setter FlagSetterFunc[bool]
	// Destination ia a pointer to which the value of the flag or env var is assigned.
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
		envVar   string
		envValue *string
	)

	valType := newBoolType(flag.Destination, flag.Setter, flag.Negative)

	for _, envVar = range flag.EnvVars {
		if val := flag.LookupEnv(envVar); val != nil && *val != "" {
			envValue = val

			break
		}
	}

	if flag.FlagValue, err = newGenericValue(valType, envValue); err != nil {
		if envValue != nil {
			return errors.Errorf("invalid boolean value %q for env var %s: %w", *envValue, envVar, err)
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
	return flag.EnvVars
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

// -- bool Type
type boolType struct {
	*genericType[bool]
	negative bool
}

func newBoolType(dest *bool, setter FlagSetterFunc[bool], negative bool) *boolType {
	return &boolType{
		genericType: newGenericType(dest, setter),
		negative:    negative,
	}
}

func (val *boolType) Clone(dest *bool) FlagType[bool] {
	return &boolType{
		genericType: &genericType[bool]{dest: dest},
		negative:    val.negative,
	}
}

func (val *boolType) Set(str string) error {
	if err := val.genericType.Set(str); err != nil {
		return err
	}

	if val.negative {
		*val.genericType.dest = !*val.genericType.dest
	}

	return nil
}
