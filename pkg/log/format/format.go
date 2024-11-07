// Package format implements a custom format logs
package format

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	. "github.com/gruntwork-io/terragrunt/pkg/log/format/options"      //nolint:stylecheck
	. "github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders" //nolint:stylecheck
	"golang.org/x/exp/maps"
)

const (
	BareFormatName     = "bare"
	PrettyFormatName   = "pretty"
	JSONFormatName     = "json"
	KeyValueFormatName = "key-value"
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
			Color(GradientColor),
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
		PlainText(`", "prefix":"`),
		Field(WorkDirKeyName,
			PathFormat(ModulePath),
			Escape(JSONEscape),
		),
		PlainText(`", "tfpath":"`),
		Field(TFPathKeyName,
			PathFormat(FilenamePath),
			Escape(JSONEscape),
		),
		PlainText(`", "msg":"`),
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
		),
		Field(WorkDirKeyName,
			Prefix(" prefix="),
			PathFormat(RelativeModulePath),
		),
		Field(TFPathKeyName,
			Prefix(" tfpath="),
			PathFormat(FilenamePath),
		),
		Message(
			Prefix(" msg="),
			PathFormat(RelativePath),
			Color(DisableColor),
		),
	}
)

var presets = map[string]Placeholders{
	BareFormatName:     BareFormat,
	PrettyFormatName:   PrettyFormat,
	JSONFormatName:     JSONFormat,
	KeyValueFormatName: KeyValueFormat,
}

func ParseFormat(str string) (Placeholders, error) {
	for name, format := range presets {
		if name == str {
			return format, nil
		}
	}

	return nil, errors.Errorf("available values: %s", strings.Join(maps.Keys(presets), ","))
}
