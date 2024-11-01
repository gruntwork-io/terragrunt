package options

const SuffixOptionName = "suffix"

type suffix struct {
	*CommonOption[string]
}

func (option *suffix) Evaluate(data *Data, str string) string {
	return str + option.value
}

func (option *suffix) SetValue(str string) error {
	option.value = str

	return nil
}

func Suffix(value string) Option {
	return &suffix{
		CommonOption: NewCommonOption[string](SuffixOptionName, value, nil),
	}
}
