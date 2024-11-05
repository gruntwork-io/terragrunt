// Package format implements a custom format logs
package format

import (
	"fmt"

	. "github.com/gruntwork-io/terragrunt/pkg/log/format/options"      //nolint:stylecheck
	. "github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders" //nolint:stylecheck
)

var (
	BareFormat = Placeholders{
		Level(
			Width(4), //nolint:mnd
			Case(UpperCase),
		),
		Interval(
			Prefix("["),
			Suffix("]"),
		),
		PlainText(" "),
		Message(),
		Field(WorkDirKeyName,
			PathFormat(ModulePath),
			Prefix("\t prefix=["),
			Suffix("] "),
		),
	}

	PrettyFormat = Placeholders{
		Time(
			TimeFormat(fmt.Sprintf("%s:%s:%s%s", Hour24Zero, MinZero, SecZero, MilliSec)),
			Color(BlackHColor),
		),
		PlainText(" "),
		Level(
			Width(6), //nolint:mnd
			Case(UpperCase),
			Color(AutoColor),
		),
		PlainText(" "),
		Field(WorkDirKeyName,
			PathFormat(RelativeModulePath),
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
	}

	JSONFormat = Placeholders{
		PlainText(`{"time":"`),
		Time(
			TimeFormat(RFC3339),
			Escape(JSONEscape),
		),
		PlainText(`", "level":"`),
		Level(
			Escape(JSONEscape),
		),
		PlainText(`", "work-dir":"`),
		Field(WorkDirKeyName,
			PathFormat(ModulePath),
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
	}

	KeyValueFormat = Placeholders{
		Time(
			Prefix("time="),
			TimeFormat(RFC3339),
		),
		Level(
			Prefix(" level="),
			Escape(JSONEscape),
		),
		Field(WorkDirKeyName,
			Prefix(" work-dir="),
			PathFormat(RelativeModulePath),
			Escape(JSONEscape),
		),
		Field(TFPathKeyName,
			Prefix(" tfpath="),
			PathFormat(FilenamePath),
			Escape(JSONEscape),
		),
		Message(
			Prefix(" message="),
			PathFormat(RelativePath),
			Color(DisableColor),
			Escape(JSONEscape),
		),
	}
)

var presets = map[string]Placeholders{
	"bare":      BareFormat,
	"pretty":    PrettyFormat,
	"json":      JSONFormat,
	"key-value": KeyValueFormat,
}

func ParseFormat(str string) Placeholders {
	for name, format := range presets {
		if name == str {
			return format
		}
	}

	return nil
}
