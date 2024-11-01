package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const LevelPlaceholderName = "level"

var levlAutoColorFunc = func(level log.Level) options.ColorValue {
	switch level {
	case log.TraceLevel:
		return options.WhiteColor
	case log.DebugLevel:
		return options.BlueHColor
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

func (level *level) Evaluate(data *options.Data) string {
	newData := *data
	newData.AutoColorFn = func() options.ColorValue {
		return levlAutoColorFunc(data.Level)
	}

	return level.opts.Evaluate(&newData, data.Level.String())
}

func Level(opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.LevelFormat(options.LevelFormatFull),
	).Merge(opts...)

	return &level{
		CommonPlaceholder: NewCommonPlaceholder(LevelPlaceholderName, opts...),
	}
}

func init() {
	Registered.Add(Level())
}
