package cli

import (
	libflag "flag"

	"github.com/urfave/cli/v2"
)

type BoolFlag struct {
	CommonFlag

	Name        string
	DefaultText string
	Usage       string
	Aliases     []string
	EnvVar      string

	Destination *bool
	Negative    bool
}

// Apply applies Flag settings to the given flag set.
func (flag *BoolFlag) Apply(set *libflag.FlagSet) error {
	var err error
	valType := FlagType[bool](&boolFlagType{negative: flag.Negative})

	if flag.FlagValue, err = newGenericValue(valType, flag.Destination, flag.EnvVar); err != nil {
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

// GetValue returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *BoolFlag) GetDefaultText() string {
	if flag.DefaultText == "" {
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

// -- bool Flag Type
type boolFlagType struct {
	*flagType[bool]
	negative bool
}

func (val *boolFlagType) Clone(dest *bool) FlagType[bool] {
	return &boolFlagType{
		flagType: &flagType[bool]{dest: dest},
		negative: val.negative,
	}
}

func (val *boolFlagType) Set(str string) error {
	if err := val.flagType.Set(str); err != nil {
		return err
	}

	if val.negative {
		*val.flagType.dest = !*val.flagType.dest
	}
	return nil
}
