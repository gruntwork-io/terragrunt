package formats

import (
	"strings"

	"github.com/sirupsen/logrus"
)

type Formatters []Formatter

func (formatters Formatters) Names() []string {
	strs := make([]string, len(formatters))

	for i, formatter := range formatters {
		strs[i] = formatter.Name()
	}

	return strs
}

func (formatters Formatters) String() string {
	return strings.Join(formatters.Names(), ", ")
}

type Formatter interface {
	logrus.Formatter

	Name() string
	SetOptions(presetName string, opts ...*Option) error
}

func AllFormatters() Formatters {
	return []Formatter{
		NewPrettyFormatter(
			NewPreset("",
				NewOption(OptionTime, true, ""),
				NewOption(OptionLevel, true, ""),
				NewOption(OptionPrefix, true, ""),
				NewOption(OptionColor, true, ""),
			),
			NewPreset("tiny",
				NewOption(OptionTime, true, "@mini"),
				NewOption(OptionLevel, true, OptionLevelShort),
				NewOption(OptionColor, true, ""),
			),
		),
		NewKeyValueFormatter(),
		NewJSONFormatter(),
	}
}

// ParseFormat takes a string and returns a Formatter instance with defined options.
func ParseFormat(str string, defaultFormatterName string) (Formatter, error) {
	var (
		allFormatters = AllFormatters()
		formatter     Formatter
		opts          Options
		presetName    string
	)

	formatters := make(map[string]Formatter, len(allFormatters))
	for _, f := range allFormatters {
		formatters[f.Name()] = f
	}

	parts := strings.Split(str, ",")
	for _, str := range parts {
		str = strings.TrimSpace(str)
		str = strings.ToLower(str)

		var (
			name  = str
			value string
		)

		if parts := strings.SplitN(name, ":", 2); len(parts) > 1 {
			name = parts[0]
			value = parts[1]
		}

		if f, ok := formatters[name]; ok {
			formatter = f
			presetName = value
			continue
		}

		opt, err := ParseOption(str)
		if err != nil {
			return nil, err
		}

		opts = append(opts, opt)
	}

	if formatter == nil {
		if f, ok := formatters[defaultFormatterName]; ok {
			formatter = f
		}
	}

	if err := formatter.SetOptions(presetName, opts...); err != nil {
		return nil, err
	}

	return formatter, nil
}
