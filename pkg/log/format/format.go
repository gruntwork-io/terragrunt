package format

import (
	"fmt"

	. "github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	. "github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

const (
	PrettyFormat = "pretty"
	JSONFormat   = "json"
)

var presets = map[string]Placeholders{
	PrettyFormat: Placeholders{
		Time(
			TimeFormat(fmt.Sprintf("%s:%s:%s%s", Hour24Zero, MinZero, SecZero, MilliSec)),
			Suffix(" "),
			Color(BlackHColor),
		),
		Level(
			Width(6),
			Case(UpperCase),
			Suffix(" "),
			Color(AutoColor),
		),
		Field(WorkDirKeyName,
			PathFormat(ShortPath),
			Prefix("["),
			Suffix("] "),
			Color(RandomColor),
		),
		Field(TFPathKeyName,
			PathFormat(FilenamePath),
			Suffix(": "),
			Color(CyanColor),
		),
		Message(
			PathFormat(RelativePath),
		),
	},
	JSONFormat: Placeholders{
		PlainText(`{"time":"`),
		Time(
			TimeFormat(fmt.Sprintf("%s:%s:%s%s", Hour24Zero, MinZero, SecZero, MilliSec)),
			Escape(JSONEscape),
		),
		PlainText(`", "level":"`),
		Level(
			Escape(JSONEscape),
		),
		PlainText(`", "work-dir":"`),
		Field(WorkDirKeyName,
			PathFormat(ShortPath),
			Escape(JSONEscape),
		),
		PlainText(`", "tfpath":"`),
		Field(TFPathKeyName,
			PathFormat(FilenamePath),
			Escape(JSONEscape),
		),
		PlainText(`", "message":"`),
		Message(
			PathFormat(RelativePath),
			Color(DisableColor),
			Escape(JSONEscape),
		),
		PlainText(`"}`),
	},
}

func ParseFormat(str string) Placeholders {
	for name, format := range presets {
		if name == str {
			return format
		}
	}

	return nil
}
