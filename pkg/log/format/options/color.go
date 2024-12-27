package options

import (
	"strconv"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mgutz/ansi"
	"github.com/puzpuzpuz/xsync/v3"
	"golang.org/x/exp/maps"
)

// ColorOptionName is the option name.
const ColorOptionName = "color"

const (
	NoneColor ColorValue = iota + 255
	DisableColor
	GradientColor
	PresetColor

	BlackColor
	RedColor
	WhiteColor
	YellowColor
	GreenColor
	BlueColor
	CyanColor
	MagentaColor

	LightBlueColor
	LightBlackColor
	LightRedColor
	LightGreenColor
	LightYellowColor
	LightMagentaColor
	LightCyanColor
	LightWhiteColor
)

var (
	colorList = NewColorList(map[ColorValue]string{ //nolint:gochecknoglobals
		PresetColor:   "preset",
		GradientColor: "gradient",
		DisableColor:  "disable",

		BlackColor:   "black",
		RedColor:     "red",
		WhiteColor:   "white",
		YellowColor:  "yellow",
		GreenColor:   "green",
		CyanColor:    "cyan",
		MagentaColor: "magenta",
		BlueColor:    "blue",

		LightBlueColor:    "light-blue",
		LightBlackColor:   "light-black",
		LightRedColor:     "light-red",
		LightGreenColor:   "light-green",
		LightYellowColor:  "light-yellow",
		LightMagentaColor: "light-magenta",
		LightCyanColor:    "light-cyan",
		LightWhiteColor:   "light-white",
	})

	colorScheme = ColorScheme{ //nolint:gochecknoglobals
		BlackColor:        "black",
		RedColor:          "red",
		WhiteColor:        "white",
		YellowColor:       "yellow",
		GreenColor:        "green",
		CyanColor:         "cyan",
		BlueColor:         "blue",
		MagentaColor:      "magenta",
		LightBlueColor:    "blue+h",
		LightBlackColor:   "black+h",
		LightRedColor:     "red+h",
		LightGreenColor:   "green+h",
		LightYellowColor:  "yellow+h",
		LightMagentaColor: "magenta+h",
		LightCyanColor:    "cyan+h",
		LightWhiteColor:   "white+h",
	}
)

type ColorList struct {
	MapValue[ColorValue]
}

func NewColorList(list map[ColorValue]string) ColorList {
	return ColorList{
		MapValue: NewMapValue(list),
	}
}

func (val *ColorList) Set(v ColorValue) *ColorList {
	return &ColorList{MapValue: *val.MapValue.Set(v)}
}

func (val *ColorList) Parse(str string) error {
	if num, err := strconv.Atoi(str); err == nil && num >= 0 && num <= 255 {
		val.value = ColorValue(byte(num))

		return nil
	}

	if err := val.MapValue.Parse(str); err != nil {
		return errors.Errorf("available values: 0..255,%s", strings.Join(maps.Values(val.list), ","))
	}

	return nil
}

type ColorScheme map[ColorValue]ColorStyle

func (scheme ColorScheme) Compile() compiledColorScheme {
	compiled := make(compiledColorScheme, len(scheme))

	for name, val := range scheme {
		compiled[name] = val.ColorFunc()
	}

	for i := range 255 {
		s := strconv.Itoa(i)

		compiled[ColorValue(i)] = ColorStyle(s).ColorFunc()
	}

	return compiled
}

type ColorStyle string

func (val ColorStyle) ColorFunc() ColorFunc {
	return ansi.ColorFunc(string(val))
}

type ColorFunc func(string) string

type ColorValue int

type compiledColorScheme map[ColorValue]ColorFunc

type ColorOption struct {
	*CommonOption[ColorValue]
	compiledColors compiledColorScheme
	gradientColor  *gradientColor
}

// Format implements `Option` interface.
func (color *ColorOption) Format(data *Data, val any) (any, error) {
	var (
		str   = toString(val)
		value = color.value.Get()
	)

	if value == NoneColor {
		return str, nil
	}

	if value == DisableColor || data.DisableColors {
		return log.RemoveAllASCISeq(str), nil
	}

	if value == PresetColor && data.PresetColorFn != nil {
		value = data.PresetColorFn()
	}

	if value == GradientColor && color.gradientColor != nil {
		value = color.gradientColor.Value(str)
	}

	if colorFn, ok := color.compiledColors[value]; ok {
		str = colorFn(str)
	}

	return str, nil
}

// Color creates the option to change the color of text.
func Color(val ColorValue) Option {
	return &ColorOption{
		CommonOption:   NewCommonOption(ColorOptionName, colorList.Set(val)),
		compiledColors: colorScheme.Compile(),
		gradientColor:  newGradientColor(),
	}
}

var (
	// defaultAutoColorValues contains ANSI color codes that are assigned sequentially to each unique text in a rotating order
	// https://user-images.githubusercontent.com/995050/47952855-ecb12480-df75-11e8-89d4-ac26c50e80b9.png
	// https://www.hackitu.de/termcolor256/
	defaultAutoColorValues = []ColorValue{ //nolint:gochecknoglobals
		66,
		67,
		95,
		96,
		102,
		103,
		108,
		109,
		138,
		139,
		144,
		145,
	}
)

type gradientColor struct {
	// cache stores unique text with their color code.
	// We use [xsync.MapOf](https://github.com/puzpuzpuz/xsync?tab=readme-ov-file#map) instead of standard `sync.Map` since it's faster and has generic types.
	cache  *xsync.MapOf[string, ColorValue]
	values []ColorValue
	mu     sync.Mutex

	// nextStyleIndex is used to get the next style from the `codes` list for a newly discovered text.
	nextStyleIndex int
}

func newGradientColor() *gradientColor {
	return &gradientColor{
		cache:  xsync.NewMapOf[string, ColorValue](),
		values: defaultAutoColorValues,
	}
}

func (color *gradientColor) Value(text string) ColorValue {
	color.mu.Lock()
	defer color.mu.Unlock()

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
