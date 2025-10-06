package cli

import (
	libflag "flag"
	"fmt"

	"github.com/urfave/cli/v2"
)

// BoolFlag implements Flag
var _ Flag = new(BoolFlag)

type BoolFlag struct {
	flag

	// Action is a function that is called when the flag is specified. It is executed only after all command flags have been parsed.
	Action FlagActionFunc[bool]

	// Setter represents the function that is called when the flag is specified.
	// Executed during value parsing, in case of an error the returned error is wrapped with the flag or environment variable name.
	Setter FlagSetterFunc[bool]

	// Destination ia a pointer to which the value of the flag or env var is assigned.
	// It also uses as the default value displayed in the help.
	Destination *bool

	// Name is the name of the flag.
	Name string

	// DefaultText is the default value of the flag to display in the help, if it is empty, the value is taken from `Destination`.
	DefaultText string

	// Usage is a short usage description to display in help.
	Usage string

	// Aliases are usually used for the short flag name, like `-h`.
	Aliases []string

	// EnvVars are the names of the env variables that are parsed and assigned to `Destination` before the flag value.
	EnvVars []string

	// Negative inverts the value of the flag.
	// If set to true, then the assigned flag value will be inverted.
	// Example: With `Negative: true`, `--boolean-flag` sets the value to `false`, and `--boolean-flag=false` sets the value to `true`.
	Negative bool

	// Hidden hides the flag from the help.
	Hidden bool
}

// Apply applies Flag settings to the given flag set.
func (flag *BoolFlag) Apply(set *libflag.FlagSet) error {
	if flag.FlagValue != nil {
		return ApplyFlag(flag, set)
	}

	if flag.Destination == nil {
		flag.Destination = new(bool)
	}

	valueType := newBoolVar(flag.Destination, flag.Negative)
	value := newGenericValue(valueType, flag.Setter)

	flag.FlagValue = &flagValue{
		value:            value,
		initialTextValue: value.String(),
		negative:         flag.Negative,
	}

	return ApplyFlag(flag, set)
}

// GetHidden returns true if the flag should be hidden from the help.
func (flag *BoolFlag) GetHidden() bool {
	return flag.Hidden
}

// GetUsage returns the usage string for the flag.
func (flag *BoolFlag) GetUsage() string {
	return flag.Usage
}

// GetEnvVars implements `cli.Flag` interface.
func (flag *BoolFlag) GetEnvVars() []string {
	return flag.EnvVars
}

// TakesValue returns true of the flag takes a value, otherwise false.
// Implements `cli.DocGenerationFlag.TakesValue` required to generate help.
func (flag *BoolFlag) TakesValue() bool {
	return false
}

// GetDefaultText returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *BoolFlag) GetDefaultText() string {
	if flag.DefaultText == "" && flag.FlagValue != nil {
		return flag.GetInitialTextValue()
	}

	return flag.DefaultText
}

// String returns a readable representation of this value (for usage defaults).
func (flag *BoolFlag) String() string {
	return cli.FlagStringer(flag)
}

// Names returns the names of the flag.
func (flag *BoolFlag) Names() []string {
	if flag.Name == "" {
		return flag.Aliases
	}

	return append([]string{flag.Name}, flag.Aliases...)
}

// RunAction implements ActionableFlag.RunAction
func (flag *BoolFlag) RunAction(ctx *Context) error {
	dest := flag.Destination
	if dest == nil {
		dest = new(bool)
	}

	if flag.Action != nil {
		return flag.Action(ctx, *dest)
	}

	return nil
}

var _ = FlagVariable[bool](new(boolVar))

// -- bool Type
type boolVar struct {
	*genericVar[bool]
	negative bool
}

func newBoolVar(dest *bool, negative bool) *boolVar {
	return &boolVar{
		genericVar: &genericVar[bool]{dest: dest},
		negative:   negative,
	}
}

func (val *boolVar) Clone(dest *bool) FlagVariable[bool] {
	return &boolVar{
		genericVar: &genericVar[bool]{dest: dest},
		negative:   val.negative,
	}
}

func (val *boolVar) Set(str string) error {
	if err := val.genericVar.Set(str); err != nil {
		return err
	}

	if val.negative {
		*val.dest = !*val.dest
	}

	return nil
}

// String returns a readable representation of this value
func (val *boolVar) String() string {
	if val.dest == nil {
		return ""
	}

	format := "%v"
	if _, ok := val.Get().(bool); ok {
		format = "%t"
	}

	return fmt.Sprintf(format, *val.dest)
}
