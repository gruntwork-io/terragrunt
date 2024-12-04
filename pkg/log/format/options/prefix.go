package options

// PrefixOptionName is the option name.
const PrefixOptionName = "prefix"

type PrefixOption struct {
	*CommonOption[string]
}

// Format implements `Option` interface.
func (option *PrefixOption) Format(_ *Data, val any) (any, error) {
	return option.value.Get() + toString(val), nil
}

// Prefix creates the option to add a prefix to the text.
func Prefix(val string) Option {
	return &PrefixOption{
		CommonOption: NewCommonOption(PrefixOptionName, NewStringValue(val)),
	}
}
