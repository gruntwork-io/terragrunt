package formatter

var (
	// prefixStyles contains ANSI color codes that are assigned sequentially to each unique prefix in a rotating order
	// https://user-images.githubusercontent.com/995050/47952855-ecb12480-df75-11e8-89d4-ac26c50e80b9.png
	// https://www.hackitu.de/termcolor256/
	prefixStyles = []ColorStyle{
		"66", "67", "95", "96", "102", "103", "108", "109", "139", "138", "144", "145",
	}
)

type PrefixStyle struct {
	// cache stores prefixes with their color schemes.
	cache map[string]ColorFunc

	styles []ColorStyle

	// nextStyleIndex is used to get the next style from the `prefixStyles` list for a newly discovered prefix.
	nextStyleIndex int
}

func NewPrefixStyle() *PrefixStyle {
	return &PrefixStyle{
		cache:  make(map[string]ColorFunc),
		styles: prefixStyles,
	}
}

func (prefix *PrefixStyle) ColorFunc(prefixName string) ColorFunc {
	if colorFunc, ok := prefix.cache[prefixName]; ok {
		return colorFunc
	}

	if prefix.nextStyleIndex >= len(prefixStyles) {
		prefix.nextStyleIndex = 0
	}

	colorFunc := prefix.styles[prefix.nextStyleIndex].ColorFunc()

	prefix.cache[prefixName] = colorFunc
	prefix.nextStyleIndex++

	return colorFunc
}
