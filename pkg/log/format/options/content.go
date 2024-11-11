package options

const ContentOptionName = "content"

type ContentOption struct {
	*CommonOption[string]
}

func (option *ContentOption) Format(_ *Data, str string) (string, error) {
	return option.value.Get(), nil
}

func Content(val string) Option {
	return &ContentOption{
		CommonOption: NewCommonOption(ContentOptionName, NewStringValue(val)),
	}
}
