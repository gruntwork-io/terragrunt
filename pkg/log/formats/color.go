package formats

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mgutz/ansi"
)

var (
	defaultColorScheme = &ColorScheme{
		StderrLevelStyle: "red",
		StdoutLevelStyle: "white",
		ErrorLevelStyle:  "red",
		WarnLevelStyle:   "yellow",
		InfoLevelStyle:   "green",
		DebugLevelStyle:  "blue+h",
		TraceLevelStyle:  "white",
		SubPrefixStyle:   "cyan",
		TimestampStyle:   "black+h",
	}
)

const (
	None ColorStyleName = iota
	StderrLevelStyle
	StdoutLevelStyle
	ErrorLevelStyle
	WarnLevelStyle
	InfoLevelStyle
	DebugLevelStyle
	TraceLevelStyle
	TimestampStyle
	SubPrefixStyle
)

type ColorStyleName byte

type ColorFunc func(string) string

type ColorStyle string

func (style ColorStyle) ColorFunc() ColorFunc {
	return ansi.ColorFunc(string(style))
}

type ColorScheme map[ColorStyleName]ColorStyle

func (scheme ColorScheme) Compile() compiledColorScheme {
	compiled := make(compiledColorScheme, len(scheme))

	for name, style := range scheme {
		compiled[name] = style.ColorFunc()
	}

	return compiled
}

type compiledColorScheme map[ColorStyleName]ColorFunc

func (scheme compiledColorScheme) LevelColorFunc(level log.Level) ColorFunc {
	switch level {
	case log.StdoutLevel:
		return scheme.ColorFunc(StdoutLevelStyle)
	case log.StderrLevel:
		return scheme.ColorFunc(StderrLevelStyle)
	case log.ErrorLevel:
		return scheme.ColorFunc(ErrorLevelStyle)
	case log.WarnLevel:
		return scheme.ColorFunc(WarnLevelStyle)
	case log.InfoLevel:
		return scheme.ColorFunc(InfoLevelStyle)
	case log.DebugLevel:
		return scheme.ColorFunc(DebugLevelStyle)
	case log.TraceLevel:
		return scheme.ColorFunc(TraceLevelStyle)
	default:
		return scheme.ColorFunc(None)
	}
}

func (scheme compiledColorScheme) ColorFunc(name ColorStyleName) ColorFunc {
	if colorFunc, ok := scheme[name]; ok {
		return colorFunc
	}

	return func(s string) string { return s }
}
