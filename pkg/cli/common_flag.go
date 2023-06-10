package cli

// CommonFlag is a common flag related to parsing flags in cli.
type CommonFlag struct {
	FlagValue FlagValue

	Name        string
	Aliases     []string
	DefaultText string
	Usage       string
	EnvVar      string
}

func (flag *CommonFlag) Value() FlagValue {
	return flag.FlagValue
}

// TakesValue returns true of the flag takes a value, otherwise false.
// Implements `cli.DocGenerationFlag.TakesValue` required to generate help.
func (flag *CommonFlag) TakesValue() bool {
	return !flag.FlagValue.IsBoolFlag()
}

// IsSet `cli.CommonFlag.IsSet` required to generate help.
func (flag *CommonFlag) IsSet() bool {
	return flag.FlagValue.IsSet()
}

// GetValue returns the flags value as string representation and an empty
// string if the flag takes no value at all.
// Implements `cli.DocGenerationFlag.GetValue` required to generate help.
func (flag *CommonFlag) GetValue() string {
	return flag.FlagValue.String()
}

// GetCategory returns the category for the flag.
// Implements `cli.DocGenerationFlag.GetCategory` required to generate help.
func (flag *CommonFlag) GetCategory() string {
	return ""
}
