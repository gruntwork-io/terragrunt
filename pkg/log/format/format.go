package format

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/config"
)

const (
	ColumnTime    = "#time=#1"
	ColumnLevel   = "#level=#2"
	ColumnPrefix  = "#prefix=#3"
	ColumnMessage = "#message=#4"
)

var (
	DefaultConfig = config.New(defaultConfigName,
		config.NewOption(OptionColor, true, nil),
		config.NewOption(ColumnTime, true,
			config.NewLayout("%s%s:%s:%s%s",
				config.NewVar(config.VarColorBlackH),
				config.NewVar(config.VarHour24Zero),
				config.NewVar(config.VarMinZero),
				config.NewVar(config.VarSecZero),
				config.NewVar(config.VarMilliSec)),
		),

		config.NewOption(ColumnLevel, true,
			config.NewLayout("%-6s",
				config.NewVar(config.VarLevel, config.FilterUpper)),
		),

		config.NewOption(ColumnLevel, true,
			config.NewLayout("%s%-6s",
				config.NewVar(config.VarColorRed),
				config.NewVar(config.VarLevel, config.FilterUpper)),
			log.ErrorLevel, log.StderrLevel,
		),

		config.NewOption(ColumnLevel, true,
			config.NewLayout("%s%-6s",
				config.NewVar(config.VarColorYellow),
				config.NewVar(config.VarLevel, config.FilterUpper)),
			log.WarnLevel,
		),

		config.NewOption(ColumnLevel, true,
			config.NewLayout("%s%-6s",
				config.NewVar(config.VarColorGreen),
				config.NewVar(config.VarLevel, config.FilterUpper)),
			log.InfoLevel,
		),

		config.NewOption(ColumnLevel, true,
			config.NewLayout("%s%-6s",
				config.NewVar(config.VarColorBlueH),
				config.NewVar(config.VarLevel, config.FilterUpper)),
			log.DebugLevel,
		),

		config.NewOption(ColumnPrefix, true,
			config.NewLayout("%s[%s]",
				config.NewVar(config.VarColorRandom),
				config.NewVar("rel-prefix"))),

		config.NewOption("prefix", false, nil),
		config.NewOption("rel-prefix", false, nil),
		config.NewOption("sub-prefix", false, nil),
		config.NewOption(ColumnMessage, true, config.NewLayout("%s", config.NewVar(config.VarMessage))),
	)

	TinyConfig = config.New("tiny",
		config.NewOption(OptionColor, true, nil),
		config.NewOption(ColumnTime, true, config.NewLayout("%s", config.NewVar(config.VarSinceStartSec))),
		config.NewOption(ColumnLevel, true, config.NewLayout("%s", config.NewVar(config.VarLevelShort, config.FilterUpper))),
		config.NewOption(ColumnMessage, true, config.NewLayout("%s", config.NewVar("message"))),
	)

	Configs = config.Configs{DefaultConfig, TinyConfig}
)
