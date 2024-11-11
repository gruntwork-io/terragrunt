package options

const ContentOptionName = "content"

type ContentValue string

func (val ContentValue) Parse(str string) (ContentValue, error) {
	return ContentValue(str), nil
}

type ContentOption struct {
	*CommonOption[ContentValue]
}

func (option *ContentOption) Evaluate(_ *Data, str string) (string, error) {
	return string(option.value), nil
}

func Content(value ContentValue) Option {
	return &ContentOption{
		CommonOption: NewCommonOption(ContentOptionName, value, value),
	}
}
