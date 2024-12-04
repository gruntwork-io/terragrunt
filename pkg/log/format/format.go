// Package format implements a custom format logs
package format

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	. "github.com/gruntwork-io/terragrunt/pkg/log/format/options"      //nolint:stylecheck,revive
	. "github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders" //nolint:stylecheck,revive
	"golang.org/x/exp/maps"
)

const (
	BareFormatName     = "bare"
	PrettyFormatName   = "pretty"
	JSONFormatName     = "json"
	KeyValueFormatName = "key-value"
)

func NewBareFormat() Placeholders {
	return Placeholders{
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
			PathFormat(ShortPath),
			Prefix("\t prefix=["),
			Suffix("] "),
		),
	}
}

func NewPrettyFormat() Placeholders {
	return Placeholders{
		Time(
			TimeFormat(fmt.Sprintf("%s:%s:%s%s", Hour24Zero, MinZero, SecZero, MilliSec)),
			Color(LightBlackColor),
		),
		PlainText(" "),
		Level(
			Width(6), //nolint:mnd
			Case(UpperCase),
			Color(PresetColor),
		),
		PlainText(" "),
		Field(WorkDirKeyName,
			PathFormat(ShortRelativePath),
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
}

func NewJSONFormat() Placeholders {
	return Placeholders{
		PlainText(`{`),
		Time(
			Prefix(`"time":"`),
			Suffix(`"`),
			TimeFormat(RFC3339),
			Escape(JSONEscape),
		),
		Level(
			Prefix(`, "level":"`),
			Suffix(`"`),
			Escape(JSONEscape),
		),
		Field(WorkDirKeyName,
			Prefix(`, "working-dir":"`),
			Suffix(`"`),
			Escape(JSONEscape),
		),
		Field(TFPathKeyName,
			Prefix(`, "tf-path":"`),
			Suffix(`"`),
			PathFormat(FilenamePath),
			Escape(JSONEscape),
		),
		Field(TFCmdArgsKeyName,
			Prefix(`, "tf-command-args":[`),
			Suffix(`]`),
			Escape(JSONEscape),
		),
		Message(
			Prefix(`, "msg":"`),
			Suffix(`"`),
			PathFormat(RelativePath),
			Color(DisableColor),
			Escape(JSONEscape),
		),
		PlainText(`}`),
	}
}

func NewKeyValueFormat() Placeholders {
	return Placeholders{
		Time(
			Prefix("time="),
			TimeFormat(RFC3339),
		),
		Level(
			Prefix(" level="),
		),
		Field(WorkDirKeyName,
			Prefix(" prefix="),
			PathFormat(ShortRelativePath),
		),
		Field(TFPathKeyName,
			Prefix(" tf-path="),
			PathFormat(FilenamePath),
		),
		Message(
			Prefix(" msg="),
			PathFormat(RelativePath),
			Color(DisableColor),
		),
	}
}

func ParseFormat(str string) (Placeholders, error) {
	var presets = map[string]func() Placeholders{
		BareFormatName:     NewBareFormat,
		PrettyFormatName:   NewPrettyFormat,
		JSONFormatName:     NewJSONFormat,
		KeyValueFormatName: NewKeyValueFormat,
	}

	for name, formatFn := range presets {
		if name == str {
			return formatFn(), nil
		}
	}

	return nil, errors.Errorf("available values: %s", strings.Join(maps.Keys(presets), ","))
}
