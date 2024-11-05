package options

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mgutz/ansi"
	"github.com/puzpuzpuz/xsync/v3"
)

const ColorOptionName = "color"

const (
	NoneColor ColorValue = iota
	DisableColor
	RedColor
	WhiteColor
	YellowColor
	GreenColor
	CyanColor
	BlueHColor
	BlackHColor
	AutoColor
	RandomColor

	Color66
	Color67
	Color95
	Color96
	Color102
	Color103
	Color108
	Color109
	Color138
	Color139
	Color144
	Color145
)

var colorValues = CommonMapValues[ColorValue]{
	RedColor:     "red",
	WhiteColor:   "white",
	YellowColor:  "yellow",
	GreenColor:   "green",
	CyanColor:    "cyan",
	BlueHColor:   "light-blue",
	BlackHColor:  "light-black",
	AutoColor:    "auto",
	RandomColor:  "random",
	DisableColor: "disable",
}

var (
	colorScheme = ColorScheme{
		RedColor:    "red",
		WhiteColor:  "white",
		YellowColor: "yellow",
		GreenColor:  "green",
		CyanColor:   "cyan",
		BlueHColor:  "blue+h",
		BlackHColor: "black+h",

		Color66:  "66",
		Color67:  "67",
		Color95:  "95",
		Color96:  "96",
		Color102: "102",
		Color103: "103",
		Color108: "108",
		Color109: "109",
		Color138: "138",
		Color139: "139",
		Color144: "144",
		Color145: "145",
	}
)

type ColorScheme map[ColorValue]ColorStyle

func (scheme ColorScheme) Compile() compiledColorScheme {
	compiled := make(compiledColorScheme, len(scheme))

	for name, val := range scheme {
		compiled[name] = val.ColorFunc()
	}

	return compiled
}

type ColorStyle string

func (val ColorStyle) ColorFunc() ColorFunc {
	return ansi.ColorFunc(string(val))
}

type ColorFunc func(string) string

type ColorValue byte

type compiledColorScheme map[ColorValue]ColorFunc

type ColorOption struct {
	*CommonOption[ColorValue]
	compiledColors compiledColorScheme
	randomColor    *randomColor
}

func (color *ColorOption) Evaluate(data *Data, str string) string {
	value := color.value

	if value == DisableColor || data.DisableColors {
		return log.RemoveAllASCISeq(str)
	}

	if value == AutoColor && data.AutoColorFn != nil {
		value = data.AutoColorFn()
	}

	if value == RandomColor && color.randomColor != nil {
		value = color.randomColor.Value(str)
	}

	if colorFn, ok := color.compiledColors[value]; ok {
		str = colorFn(str)
	}

	return str
}

func (color *ColorOption) SetValue(str string) error {
	val, err := colorValues.Parse(str)
	if err != nil {
		return err
	}

	color.value = val

	return nil
}

func Color(val ColorValue) Option {
	return &ColorOption{
		CommonOption:   NewCommonOption[ColorValue](ColorOptionName, val, colorValues),
		compiledColors: colorScheme.Compile(),
		randomColor:    newRandomColor(),
	}
}

var (
	// defaultAutoColorValues contains ANSI color codes that are assigned sequentially to each unique text in a rotating order
	// https://user-images.githubusercontent.com/995050/47952855-ecb12480-df75-11e8-89d4-ac26c50e80b9.png
	// https://www.hackitu.de/termcolor256/
	defaultAutoColorValues = []ColorValue{
		Color66,
		Color67,
		Color95,
		Color96,
		Color102,
		Color103,
		Color108,
		Color109,
		Color138,
		Color139,
		Color144,
		Color145,
	}
)

type randomColor struct {
	// cache stores unique text with their color code.
	// We use [xsync.MapOf](https://github.com/puzpuzpuz/xsync?tab=readme-ov-file#map) instaed of standard `sync.Map` since it's faster and has generic types.
	cache  *xsync.MapOf[string, ColorValue]
	values []ColorValue

	// nextStyleIndex is used to get the next style from the `codes` list for a newly discovered text.
	nextStyleIndex int
}

func newRandomColor() *randomColor {
	return &randomColor{
		cache:  xsync.NewMapOf[string, ColorValue](),
		values: defaultAutoColorValues,
	}
}

func (color *randomColor) Value(text string) ColorValue {
	if colorCode, ok := color.cache.Load(text); ok {
		return colorCode
	}

	if color.nextStyleIndex >= len(color.values) {
		color.nextStyleIndex = 0
	}

	colorCode := color.values[color.nextStyleIndex]

	color.cache.Store(text, colorCode)

	color.nextStyleIndex++

	return colorCode
}
