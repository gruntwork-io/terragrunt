package options

// ContentOptionName is the option name.
const ContentOptionName = "content"

type ContentOption struct {
	*CommonOption[string]
}

// Format implements `Option` interface.
func (option *ContentOption) Format(_ *Data, str string) (string, error) {
	if val := option.value.Get(); val != "" {
		return val, nil
	}

	return str, nil
}

// Content creates the option that sets the content.
func Content(val string) Option {
	return &ContentOption{
		CommonOption: NewCommonOption(ContentOptionName, NewStringValue(val)),
	}
}
