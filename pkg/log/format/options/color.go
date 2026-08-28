package options

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/puzpuzpuz/xsync/v4"
)

//go:generate go run ./colorgen

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
		return fmt.Errorf(
			"available values: 0..255,%s",
			strings.Join(slices.Collect(maps.Values(val.list)), ","),
		)
	}

	return nil
}

type ColorScheme map[ColorValue]ColorStyle

type ColorStyle string

// ColorFunc parses the style spec and returns a function that wraps text in
// the corresponding ANSI escape codes. The spec is either a decimal index in
// `0..255` (256-color palette) or a named color (`black`, `red`, `green`,
// `yellow`, `blue`, `magenta`, `cyan`, `white`, `gray`) optionally followed
// by `+<attrs>` where attrs is any subset of `b` (bold), `h` (bright; bumps
// the base palette index by 8 when 0..7), or `d` (faint). An empty or
// unrecognised spec yields an identity colorizer.
func (val ColorStyle) ColorFunc() ColorFunc {
	style := parseColorStyle(string(val))

	return func(s string) string {
		return style.Render(s)
	}
}

// ansiBrightOffset converts a 0..7 standard ANSI palette index to its 8..15
// bright counterpart.
const ansiBrightOffset = 8

// ANSI palette indices for the historical color names accepted by this package.
const (
	ansiBlack = iota
	ansiRed
	ansiGreen
	ansiYellow
	ansiBlue
	ansiMagenta
	ansiCyan
	ansiWhite
	ansiGray // bright black
)

// ansiBaseColors maps the historical color names accepted by this package to
// their corresponding ANSI palette index.
var ansiBaseColors = map[string]int{ //nolint:gochecknoglobals
	"black":   ansiBlack,
	"red":     ansiRed,
	"green":   ansiGreen,
	"yellow":  ansiYellow,
	"blue":    ansiBlue,
	"magenta": ansiMagenta,
	"cyan":    ansiCyan,
	"white":   ansiWhite,
	"gray":    ansiGray,
}

func parseColorStyle(spec string) lipgloss.Style {
	style := lipgloss.NewStyle()
	if spec == "" {
		return style
	}

	if idx, err := strconv.Atoi(spec); err == nil && idx >= 0 && idx <= 255 {
		return style.Foreground(lipgloss.Color(strconv.Itoa(idx)))
	}

	name, attrs, _ := strings.Cut(spec, "+")

	base, ok := ansiBaseColors[name]
	if !ok {
		return style
	}

	if strings.ContainsRune(attrs, 'h') && base < ansiBrightOffset {
		base += ansiBrightOffset
	}

	if strings.ContainsRune(attrs, 'b') {
		style = style.Bold(true)
	}

	if strings.ContainsRune(attrs, 'd') {
		style = style.Faint(true)
	}

	return style.Foreground(lipgloss.Color(strconv.Itoa(base)))
}

type ColorFunc func(string) string

type ColorValue int

// ColorReset ends a styled run. One SGR reset clears every attribute, so lipgloss
// closes with it however many the prefix set, and colorgen refuses to write a table
// whose entries close with anything else.
const ColorReset = "\x1b[m"

// ColorSeq is the pair of escape sequences that wrap text in one color.
type ColorSeq struct {
	Prefix string
	Suffix string
}

// BuildColorTable renders every [ColorValue] through lipgloss and returns the
// sequences it produces. [colorPrefixes] is generated from it, and
// TestColorTableIsCurrent compares the two. Nothing on the logging path calls it.
func BuildColorTable() []ColorSeq {
	table := make([]ColorSeq, int(LightWhiteColor)+1)

	for value := range table {
		table[value] = renderSeq(styleFor(ColorValue(value)))
	}

	return table
}

// seqSentinel is text no style spec produces, so splitting a rendered sample on
// it recovers what lipgloss added around it.
const seqSentinel = "\x00\x00terragrunt\x00\x00"

func renderSeq(style ColorStyle) ColorSeq {
	prefix, suffix, found := strings.Cut(parseColorStyle(string(style)).Render(seqSentinel), seqSentinel)
	if !found {
		panic(fmt.Sprintf("options.renderSeq: lipgloss rewrote the sentinel for style %q", style))
	}

	return ColorSeq{Prefix: prefix, Suffix: suffix}
}

type ColorOption struct {
	*CommonOption[ColorValue]
	gradientColor *gradientColor
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

	if value == DisableColor || data.DisabledColors {
		return log.RemoveAllASCISeq(str), nil
	}

	if value == PresetColor && data.PresetColorFn != nil {
		value = data.PresetColorFn()
	}

	if value == GradientColor && color.gradientColor != nil {
		value = color.gradientColor.Value(str)
	}

	return colorize(value, str), nil
}

const lipglossFormatting = "\n\t"

// colorize wraps str in value's escape sequences. A value with no sequences in
// the table leaves str unstyled.
func colorize(value ColorValue, str string) string {
	if value < 0 || int(value) >= len(colorPrefixes) {
		return str
	}

	prefix := colorPrefixes[value]
	if prefix == "" {
		return str
	}

	// lipgloss lays this text out rather than wrapping it. It re-styles each line
	// of a multi-line value, pads those lines to a common width, and expands tabs.
	// A prefix and suffix reproduce none of that, so lipgloss renders it itself.
	if strings.ContainsAny(str, lipglossFormatting) {
		return styleFor(value).ColorFunc()(str)
	}

	return prefix + str + ColorReset
}

// styleFor returns the style spec a [ColorValue] renders with. Values below
// [NoneColor] are palette indices a user named directly. The rest are scheme
// entries.
func styleFor(value ColorValue) ColorStyle {
	if value < NoneColor {
		return ColorStyle(strconv.Itoa(int(value)))
	}

	return colorScheme[value]
}

// Color creates the option to change the color of text.
func Color(val ColorValue) Option {
	return &ColorOption{
		CommonOption:  NewCommonOption(ColorOptionName, colorList.Set(val)),
		gradientColor: newGradientColor(),
	}
}

var (
	// defaultAutoColorValues contains ANSI color codes that are assigned
	// sequentially to each unique text in a rotating order
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
	// We use [xsync.Map](https://github.com/puzpuzpuz/xsync?tab=readme-ov-file#map)
	// instead of standard `sync.Map` since it's faster and has generic types.
	cache  *xsync.Map[string, ColorValue]
	values []ColorValue
	mu     sync.Mutex

	// nextStyleIndex is used to get the next style from the `codes` list for a newly discovered text.
	nextStyleIndex int
}

func newGradientColor() *gradientColor {
	return &gradientColor{
		cache:  xsync.NewMap[string, ColorValue](),
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
