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
	// BareFormatName selects [NewBareFormatPlaceholders] with `--log-format`.
	BareFormatName = "bare"
	// PrettyFormatName selects [NewPrettyFormatPlaceholders] with `--log-format`.
	PrettyFormatName = "pretty"
	// JSONFormatName selects [NewJSONFormatPlaceholders] with `--log-format`.
	JSONFormatName = "json"
	// KeyValueFormatName selects [NewKeyValueFormatPlaceholders] with `--log-format`.
	KeyValueFormatName = "key-value"
)

const (
	// bareLevelWidth is the width of the level column in the bare format.
	bareLevelWidth = 4
	// prettyLevelWidth is the width of the level column in the pretty format.
	prettyLevelWidth = 6
)

// NewBareFormatPlaceholders returns the placeholders of the bare log format: the
// uppercase level, the seconds since start in brackets, the message, and the
// working directory as `prefix=[...]` unless it is the base directory.
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

// NewPrettyFormatPlaceholders returns the placeholders of the default log format:
// a gray clock time with milliseconds, the uppercase level in its preset color,
// the working directory in brackets with a color per directory, the tofu or
// terraform binary name in cyan, the message, and the provider cache server's
// request URL and status when the entry has them.
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

// NewJSONFormatPlaceholders returns the placeholders that render each entry as a
// JSON object with the RFC 3339 time, level, working directory, tofu or terraform
// binary name and arguments, and the message stripped of color.
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

// NewKeyValueFormatPlaceholders returns the placeholders that render each entry
// as `key=value` pairs: the RFC 3339 time, level, working directory, tofu or
// terraform binary name, and the message stripped of color.
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

// ParseFormat returns the placeholders of the log format named str, one of
// [BareFormatName], [PrettyFormatName], [JSONFormatName], or [KeyValueFormatName].
//
// Returns an error listing the format names when str matches none of them.
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
