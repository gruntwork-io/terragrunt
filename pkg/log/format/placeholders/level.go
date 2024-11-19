package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

// LevelPlaceholderName is the placeholder name.
const LevelPlaceholderName = "level"

var levlAutoColorFunc = func(level log.Level) options.ColorValue {
	switch level {
	case log.TraceLevel:
		return options.WhiteColor
	case log.DebugLevel:
		return options.LightBlueColor
	case log.InfoLevel:
		return options.GreenColor
	case log.WarnLevel:
		return options.YellowColor
	case log.ErrorLevel:
		return options.RedColor
	case log.StdoutLevel:
		return options.WhiteColor
	case log.StderrLevel:
		return options.RedColor
	default:
		return options.NoneColor
	}
}

type level struct {
	*CommonPlaceholder
}

// Format implements `Placeholder` interface.
func (level *level) Format(data *options.Data) (string, error) {
	newData := *data
	newData.PresetColorFn = func() options.ColorValue {
		return levlAutoColorFunc(data.Level)
	}

	return level.opts.Format(&newData, data.Level.String())
}

// Level creates a placeholder that displays log level name.
func Level(opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.LevelFormat(options.LevelFormatFull),
	).Merge(opts...)

	return &level{
		CommonPlaceholder: NewCommonPlaceholder(LevelPlaceholderName, opts...),
	}
}
