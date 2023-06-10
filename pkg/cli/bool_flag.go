package cli

import (
	libflag "flag"
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
	flag.normalize()

	var err error
	valType := FlagType[bool](new(flagType[bool]))

	if flag.FlagValue, err = newGenreicValue(valType, flag.Destination, flag.EnvVar, flag.Negative); err != nil {
		return err
	}
	return flag.CommonFlag.Apply(set)
}

func (flag *BoolFlag) normalize() {
	flag.CommonFlag.Name = flag.Name
	flag.CommonFlag.DefaultText = flag.DefaultText
	flag.CommonFlag.Usage = flag.Usage
	flag.CommonFlag.Aliases = flag.Aliases
	flag.CommonFlag.EnvVar = flag.EnvVar
}
