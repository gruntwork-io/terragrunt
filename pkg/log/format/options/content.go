package options

// ContentOptionName is the option name.
const ContentOptionName = "content"

type ContentOption struct {
	*CommonOption[string]
}

// Format implements `Option` interface.
func (option *ContentOption) Format(_ *Data, val any) (any, error) {
	if val := option.value.Get(); val != "" {
		return val, nil
	}

	return val, nil
}

// Content creates the option that sets the content.
func Content(val string) Option {
	return &ContentOption{
		CommonOption: NewCommonOption(ContentOptionName, NewStringValue(val)),
	}
}
