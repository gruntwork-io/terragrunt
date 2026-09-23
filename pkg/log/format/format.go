// Package format implements a custom format logs
package format

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	. "github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	. "github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

const (
	BareFormatName     = "bare"
	PrettyFormatName   = "pretty"
	JSONFormatName     = "json"
	KeyValueFormatName = "key-value"
)

const (
	// bareLevelWidth is the width of the level column in the bare format.
	bareLevelWidth = 4
	// prettyLevelWidth is the width of the level column in the pretty format.
	prettyLevelWidth = 6
)

func NewBareFormatPlaceholders() Placeholders {
	return Placeholders{
		Level(
			Width(bareLevelWidth),
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

func NewPrettyFormatPlaceholders() Placeholders {
	return Placeholders{
		Time(
			TimeFormat(fmt.Sprintf("%s:%s:%s%s", Hour24Zero, MinZero, SecZero, MilliSec)),
			Color(LightBlackColor),
		),
		PlainText(" "),
		Level(
			Width(prettyLevelWidth),
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
		Message(),
		Field(CacheServerURLKeyName,
			Prefix(" "+CacheServerURLKeyName+"="),
		),
		Field(CacheServerStatusKeyName,
			Prefix(" "+CacheServerStatusKeyName+"="),
		),
	}
}

func NewJSONFormatPlaceholders() Placeholders {
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
			Color(DisableColor),
			Escape(JSONEscape),
		),
		PlainText(`}`),
	}
}

func NewKeyValueFormatPlaceholders() Placeholders {
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
			Color(DisableColor),
		),
	}
}

func ParseFormat(str string) (Placeholders, error) {
	var presets = map[string]func() Placeholders{
		BareFormatName:     NewBareFormatPlaceholders,
		PrettyFormatName:   NewPrettyFormatPlaceholders,
		JSONFormatName:     NewJSONFormatPlaceholders,
		KeyValueFormatName: NewKeyValueFormatPlaceholders,
	}

	for name, formatFn := range presets {
		if name == str {
			return formatFn(), nil
		}
	}

	return nil, fmt.Errorf(
		"available values: %s",
		strings.Join(slices.Collect(maps.Keys(presets)), ","),
	)
}
