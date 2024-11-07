package options

const PrefixOptionName = "prefix"

type PrefixOption struct {
	*CommonOption[string]
}

func (option *PrefixOption) Evaluate(data *Data, str string) string {
	return option.value + str
}

func (option *PrefixOption) ParseValue(str string) error {
	option.value = str

	return nil
}

func Prefix(value string) Option {
	return &PrefixOption{
		CommonOption: NewCommonOption[string](PrefixOptionName, value, nil),
	}
}
