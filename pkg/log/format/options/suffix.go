package options

const SuffixOptionName = "suffix"

type SuffixOption struct {
	*CommonOption[string]
}

func (option *SuffixOption) Evaluate(_ *Data, str string) (string, error) {
	return str + option.value, nil
}

func (option *SuffixOption) ParseValue(str string) error {
	option.value = str

	return nil
}

func Suffix(value string) Option {
	return &SuffixOption{
		CommonOption: NewCommonOption(SuffixOptionName, value, nil),
	}
}
