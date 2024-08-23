package formatter

var (
	// defeultPrefixStyles contains ANSI color codes that are assigned sequentially to each unique prefix in a rotating order
	// https://user-images.githubusercontent.com/995050/47952855-ecb12480-df75-11e8-89d4-ac26c50e80b9.png
	// https://www.hackitu.de/termcolor256/
	defeultPrefixStyles = []ColorStyle{
		"66", "67", "95", "96", "102", "103", "108", "109", "139", "138", "144", "145",
	}

	// prefixStyle implements PrefixStyle
	_ PrefixStyle = new(prefixStyle)
)

type prefixStyle struct {
	// cache stores prefixes with their color schemes.
	cache map[string]ColorFunc

	styles []ColorStyle

	// nextStyleIndex is used to get the next style from the `defeultPrefixStyles` list for a newly discovered prefix.
	nextStyleIndex int
}

func NewPrefixStyle() *prefixStyle {
	return &prefixStyle{
		cache:  make(map[string]ColorFunc),
		styles: defeultPrefixStyles,
	}
}

func (prefix *prefixStyle) ColorFunc(prefixName string) ColorFunc {
	if colorFunc, ok := prefix.cache[prefixName]; ok {
		return colorFunc
	}

	if prefix.nextStyleIndex >= len(defeultPrefixStyles) {
		prefix.nextStyleIndex = 0
	}

	colorFunc := prefix.styles[prefix.nextStyleIndex].ColorFunc()

	prefix.cache[prefixName] = colorFunc
	prefix.nextStyleIndex++

	return colorFunc
}
