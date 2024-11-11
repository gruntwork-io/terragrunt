package options

const PrefixOptionName = "prefix"

type PrefixOption struct {
	*CommonOption[string]
}

func (option *PrefixOption) Format(_ *Data, str string) (string, error) {
	return option.value.Get() + str, nil
}

func Prefix(val string) Option {
	return &PrefixOption{
		CommonOption: NewCommonOption(PrefixOptionName, NewStringValue(val)),
	}
}
