package format

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/preset"
)

const (
	ColumnTime    = "#time"
	ColumnLevel   = "#level"
	ColumnPrefix  = "#prefix"
	ColumnMessage = "#message"
)

var (
	DefaultPreset = preset.New(defaultPresetName,
		preset.NewOption(OptionColor, true, nil),
		preset.NewOption(ColumnTime, true,
			preset.NewLayout("%s%s:%s:%s%s",
				preset.NewArg(preset.ArgNameColorBlackH),
				preset.NewArg(preset.ArgNameHour24Zero),
				preset.NewArg(preset.ArgNameMinZero),
				preset.NewArg(preset.ArgNameSecZero),
				preset.NewArg(preset.ArgNameMilliSec)),
		),

		preset.NewOption(ColumnLevel, true,
			preset.NewLayout("%-6s",
				preset.NewArg(preset.ArgNameLevel, preset.ArgOptUpper)),
		),

		preset.NewOption(ColumnLevel, true,
			preset.NewLayout("%s%-6s",
				preset.NewArg(preset.ArgNameColorRed),
				preset.NewArg(preset.ArgNameLevel, preset.ArgOptUpper)),
			log.ErrorLevel, log.StderrLevel,
		),

		preset.NewOption(ColumnLevel, true,
			preset.NewLayout("%s%-6s",
				preset.NewArg(preset.ArgNameColorYellow),
				preset.NewArg(preset.ArgNameLevel, preset.ArgOptUpper)),
			log.WarnLevel,
		),

		preset.NewOption(ColumnLevel, true,
			preset.NewLayout("%s%-6s",
				preset.NewArg(preset.ArgNameColorGreen),
				preset.NewArg(preset.ArgNameLevel, preset.ArgOptUpper)),
			log.InfoLevel,
		),

		preset.NewOption(ColumnLevel, true,
			preset.NewLayout("%s%-6s",
				preset.NewArg(preset.ArgNameColorBlueH),
				preset.NewArg(preset.ArgNameLevel, preset.ArgOptUpper)),
			log.DebugLevel,
		),

		preset.NewOption(ColumnPrefix, true,
			preset.NewLayout("%s[%s]",
				preset.NewArg(preset.ArgNameColorRandom),
				preset.NewArg("rel-prefix", preset.ArgOptRequired))),

		preset.NewOption("prefix", false, nil),
		preset.NewOption("rel-prefix", false, nil),
		preset.NewOption("sub-prefix", false, nil),
		preset.NewOption(ColumnMessage, true, preset.NewLayout("%s", preset.NewArg(preset.ArgNameMessage))),
	)

	TinyPreset = preset.New("tiny",
		preset.NewOption(OptionColor, true, nil),
		preset.NewOption(ColumnTime, true, preset.NewLayout("%s", preset.NewArg(preset.ArgNameSinceStartSec))),
		preset.NewOption(ColumnLevel, true, preset.NewLayout("%s", preset.NewArg(preset.ArgNameLevelShort, preset.ArgOptUpper))),
		preset.NewOption(ColumnMessage, true, preset.NewLayout("%s", preset.NewArg("message"))),
	)
)
