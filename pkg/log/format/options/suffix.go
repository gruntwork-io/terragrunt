package options

// SuffixOptionName is the option name.
const SuffixOptionName = "suffix"

type SuffixOption struct {
	*CommonOption[string]
}

// Format implements `Option` interface.
func (option *SuffixOption) Format(_ *Data, val any) (any, error) {
	return toString(val) + option.value.Get(), nil
}

// Suffix creates the option to add a suffix to the text.
func Suffix(val string) Option {
	return &SuffixOption{
		CommonOption: NewCommonOption(SuffixOptionName, NewStringValue(val)),
	}
}
