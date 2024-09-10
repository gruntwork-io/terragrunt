package preset

import (
	"github.com/mgutz/ansi"
	"github.com/puzpuzpuz/xsync/v3"
)

var (
	// defaultRandomColorStyles contains ANSI color codes that are assigned sequentially to each unique text in a rotating order
	// https://user-images.githubusercontent.com/995050/47952855-ecb12480-df75-11e8-89d4-ac26c50e80b9.png
	// https://www.hackitu.de/termcolor256/
	defaultRandomColorStyles = ColorStyles{
		"66", "67", "95", "96", "102", "103", "108", "109", "139", "138", "144", "145",
	}
)

type ColorStyles []ColorStyle

func (styles ColorStyles) ColorCodes() []string {
	codes := make([]string, len(styles))

	for i, style := range styles {
		codes[i] = style.ColorCode()
	}

	return codes
}

type ColorStyle string

func (style ColorStyle) ColorCode() string {
	return ansi.ColorCode(string(style))
}

type RandomColor struct {
	// cache stores unique text with their color code.
	// We use [xsync.MapOf](https://github.com/puzpuzpuz/xsync?tab=readme-ov-file#map) instaed of standard `sync.Map` since it's faster and has generic types.
	cache *xsync.MapOf[string, string]

	codes []string

	// nextStyleIndex is used to get the next style from the `codes` list for a newly discovered text.
	nextStyleIndex int
}

func NewRandomColor() *RandomColor {
	return &RandomColor{
		cache: xsync.NewMapOf[string, string](),
		codes: defaultRandomColorStyles.ColorCodes(),
	}
}

func (color *RandomColor) ColorCode(text string) string {
	if colorCode, ok := color.cache.Load(text); ok {
		return colorCode
	}

	if color.nextStyleIndex >= len(color.codes) {
		color.nextStyleIndex = 0
	}

	colorCode := color.codes[color.nextStyleIndex]

	color.cache.Store(text, colorCode)

	color.nextStyleIndex++

	return colorCode
}
