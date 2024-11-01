package options

const PrefixOptionName = "prefix"

type prefix struct {
	*CommonOption[string]
}

func (option *prefix) Evaluate(data *Data, str string) string {
	return option.value + str
}

func (option *prefix) SetValue(str string) error {
	option.value = str

	return nil
}

func Prefix(value string) Option {
	return &prefix{
		CommonOption: NewCommonOption[string](PrefixOptionName, value, nil),
	}
}
