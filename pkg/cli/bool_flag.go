package cli

import (
	libflag "flag"

	"github.com/urfave/cli/v2"
)

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
	Action ActionFunc
	// The pointer to which the value of the flag or env var is assigned.
	// It also uses as the default value displayed in the help.
	Destination *bool
	// If set to true, then the assigned flag value will be inverted
	Negative bool
}

// Apply applies Flag settings to the given flag set.
func (flag *BoolFlag) Apply(set *libflag.FlagSet) error {
	var err error
	valType := FlagType[bool](&boolFlagType{negative: flag.Negative})

	if flag.FlagValue, err = newGenericValue(valType, flag.LookupEnv(flag.EnvVar), flag.Destination); err != nil {
		return err
	}

	for _, name := range flag.Names() {
		set.Var(flag.FlagValue, name, flag.Usage)
	}
	return nil
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
		return flag.Action(ctx)
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
