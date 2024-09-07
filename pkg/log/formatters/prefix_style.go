package formatters

import (
	"github.com/puzpuzpuz/xsync/v3"
)

var (
	// defaultPrefixStyles contains ANSI color codes that are assigned sequentially to each unique prefix in a rotating order
	// https://user-images.githubusercontent.com/995050/47952855-ecb12480-df75-11e8-89d4-ac26c50e80b9.png
	// https://www.hackitu.de/termcolor256/
	defaultPrefixStyles = []ColorStyle{
		"66", "67", "95", "96", "102", "103", "108", "109", "139", "138", "144", "145",
	}

	// prefixStyle implements PrefixStyle
	_ PrefixStyle = new(prefixStyle)
)

type PrefixStyle interface {
	// ColorFunc creates a closure to avoid computation ANSI color code.
	ColorFunc(prefixName string) ColorFunc
}

type prefixStyle struct {
	// cache stores prefixes with their color schemes.
	// We use [xsync.MapOf](https://github.com/puzpuzpuz/xsync?tab=readme-ov-file#map) instaed of standard `sync.Map` since it's faster and has generic types.
	cache *xsync.MapOf[string, ColorFunc]

	availableStyles []ColorStyle

	// nextStyleIndex is used to get the next style from the `defaultPrefixStyles` list for a newly discovered prefix.
	nextStyleIndex int
}

func NewPrefixStyle() *prefixStyle {
	return &prefixStyle{
		cache:           xsync.NewMapOf[string, ColorFunc](),
		availableStyles: defaultPrefixStyles,
	}
}

func (prefix *prefixStyle) ColorFunc(prefixName string) ColorFunc {
	if colorFunc, ok := prefix.cache.Load(prefixName); ok {
		return colorFunc
	}

	if prefix.nextStyleIndex >= len(prefix.availableStyles) {
		prefix.nextStyleIndex = 0
	}

	colorFunc := prefix.availableStyles[prefix.nextStyleIndex].ColorFunc()

	prefix.cache.Store(prefixName, colorFunc)

	prefix.nextStyleIndex++

	return colorFunc
}
