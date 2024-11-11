package options

// PrefixOptionName is the option name.
const PrefixOptionName = "prefix"

type PrefixOption struct {
	*CommonOption[string]
}

// Format implements `Option` interface.
func (option *PrefixOption) Format(_ *Data, str string) (string, error) {
	return option.value.Get() + str, nil
}

// Prefix creates the option to add a prefix to the text.
func Prefix(val string) Option {
	return &PrefixOption{
		CommonOption: NewCommonOption(PrefixOptionName, NewStringValue(val)),
	}
}
