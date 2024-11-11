package options

const PrefixOptionName = "prefix"

type PrefixOption struct {
	*CommonOption[string]
}

func (option *PrefixOption) Evaluate(_ *Data, str string) (string, error) {
	return option.value + str, nil
}

func (option *PrefixOption) ParseValue(str string) error {
	option.value = str

	return nil
}

func Prefix(value string) Option {
	return &PrefixOption{
		CommonOption: NewCommonOption(PrefixOptionName, value, nil),
	}
}
