package formatter

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mgutz/ansi"
)

var (
	defaultColorScheme = &ColorScheme{
		InfoLevelStyle:  "green",
		WarnLevelStyle:  "yellow",
		ErrorLevelStyle: "red",
		FatalLevelStyle: "red",
		PanicLevelStyle: "red",
		DebugLevelStyle: "blue+h",
		TraceLevelStyle: "white",
		TFBinaryStyle:   "cyan",
		TimestampStyle:  "black+h",
	}
)

const (
	None ColorStyleName = iota
	InfoLevelStyle
	WarnLevelStyle
	ErrorLevelStyle
	FatalLevelStyle
	PanicLevelStyle
	DebugLevelStyle
	TraceLevelStyle
	TFBinaryStyle
	TimestampStyle
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
	case log.DebugLevel:
		return scheme.ColorFunc(DebugLevelStyle)
	case log.InfoLevel:
		return scheme.ColorFunc(InfoLevelStyle)
	case log.WarnLevel:
		return scheme.ColorFunc(WarnLevelStyle)
	case log.ErrorLevel:
		return scheme.ColorFunc(ErrorLevelStyle)
	case log.StdoutLevel:
		return scheme.ColorFunc(TraceLevelStyle)
	case log.StderrLevel:
		return scheme.ColorFunc(ErrorLevelStyle)
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
