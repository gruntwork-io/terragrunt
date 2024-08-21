package formatter

import (
	"github.com/mgutz/ansi"
	"github.com/sirupsen/logrus"
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

func (scheme ColorScheme) Complite() compiledColorScheme {
	compiled := make(compiledColorScheme, len(scheme))

	for name, style := range scheme {
		compiled[name] = style.ColorFunc()
	}
	return compiled
}

type compiledColorScheme map[ColorStyleName]ColorFunc

func (scheme compiledColorScheme) LevelColorFunc(level logrus.Level) ColorFunc {
	switch level {
	case logrus.InfoLevel:
		return scheme.ColorFunc(InfoLevelStyle)
	case logrus.WarnLevel:
		return scheme.ColorFunc(WarnLevelStyle)
	case logrus.ErrorLevel:
		return scheme.ColorFunc(ErrorLevelStyle)
	case logrus.FatalLevel:
		return scheme.ColorFunc(FatalLevelStyle)
	case logrus.PanicLevel:
		return scheme.ColorFunc(PanicLevelStyle)
	case logrus.DebugLevel:
		return scheme.ColorFunc(DebugLevelStyle)
	case logrus.TraceLevel:
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
