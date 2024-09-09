package formats

import (
	"strings"

	"github.com/sirupsen/logrus"
)

type Formats []Format

func (formats Formats) Names() []string {
	strs := make([]string, len(formats))

	for i, format := range formats {
		strs[i] = format.Name()
	}

	return strs
}

func (formats Formats) String() string {
	return strings.Join(formats.Names(), ", ")
}

type Format interface {
	logrus.Formatter

	Name() string
	UsePreset(presetName string) error
	SetOptions(opts ...*Option) error
}

var DefaultPrettyFormatPreset *Preset

var TinyPrettyFormatPreset *Preset

func init() {
	DefaultPrettyFormatPreset = NewPreset("",
		NewOption(OptionColor, true, nil),
		NewOption(OptionTime, true, NewLayout("%s:%s:%s%s", NewArg("H"), NewArg("i"), NewArg("s"), NewArg("v"))),
		NewOption(OptionLevel, true, NewLayout("%-6s", NewArg("LEVEL"))),
		NewOption(OptionPrefix, true, NewLayout("[%s]", NewArg("fields.rel-prefix", ArgOptRequireValue))),
		NewOption(OptionMsg, true, NewLayout("%s", NewArg("message"))),
	)

	TinyPrettyFormatPreset = NewPreset("tiny",
		NewOption(OptionColor, true, nil),
		NewOption(OptionTime, true, NewLayout("%s:%s:%s%s", NewArg("H"), NewArg("i"), NewArg("s"), NewArg("v"))),
		NewOption(OptionLevel, true, NewLayout("%s", NewArg("LVL"))),
		NewOption(OptionMsg, true, NewLayout("%s", NewArg("message"))),
	)
}

func AllFormats() Formats {
	return []Format{
		NewPrettyFormat(DefaultPrettyFormatPreset, TinyPrettyFormatPreset),
		NewKeyValueFormat(),
		NewJSONFormat(),
	}
}

// ParseFormat takes a string and returns a Format instance with defined options.

// pretty:tiny@no-color@ident:%s %s %s@time@level@prefix
func ParseFormat(str string, defaultFormatterName string) (Format, error) {
	var (
		allFormatters = AllFormats()
		format        Format
		opts          Options
	)

	formats := make(map[string]Format, len(allFormatters))
	for _, f := range allFormatters {
		formats[f.Name()] = f
	}

	parts := strings.Split(str, ",")
	for _, str := range parts {
		var (
			name       = str
			presetName string
		)

		if parts := strings.SplitN(name, ":", 2); len(parts) > 1 {
			name = parts[0]
			presetName = parts[1]
		}

		if f, ok := formats[name]; ok {
			format = f
			format.UsePreset(presetName)
			continue
		}

		col, err := ParseOption(str)
		if err != nil {
			return nil, err
		}

		opts = append(opts, col)
	}

	if format == nil {
		if f, ok := formats[defaultFormatterName]; ok {
			format = f
		}
	}

	if err := format.SetOptions(opts...); err != nil {
		return nil, err
	}

	return format, nil
}
