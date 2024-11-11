package options

// ContentOptionName is the option name.
const ContentOptionName = "content"

type ContentOption struct {
	*CommonOption[string]
}

// Format implements `Option` interface.
func (option *ContentOption) Format(_ *Data, str string) (string, error) {
	return option.value.Get(), nil
}

// Content creates the option that sets the content.
func Content(val string) Option {
	return &ContentOption{
		CommonOption: NewCommonOption(ContentOptionName, NewStringValue(val)),
	}
}
