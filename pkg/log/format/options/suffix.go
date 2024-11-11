package options

const SuffixOptionName = "suffix"

type SuffixOption struct {
	*CommonOption[string]
}

func (option *SuffixOption) Format(_ *Data, str string) (string, error) {
	return str + option.value.Get(), nil
}

func Suffix(val string) Option {
	return &SuffixOption{
		CommonOption: NewCommonOption(SuffixOptionName, NewStringValue(val)),
	}
}
