package formatter

var (
	// prefixStyles contains ANSI color codes that are assigned sequentially to each unique prefix.
	// https://user-images.githubusercontent.com/995050/47952855-ecb12480-df75-11e8-89d4-ac26c50e80b9.png
	// https://www.hackitu.de/termcolor256/
	prefixStyles = []ColorStyle{
		"66", "67", "95", "96", "102", "103", "108", "109", "139", "138", "144", "145",
	}
)

type PrefixStyle struct {
	// cache stores prefixes with their color schemes.
	cache map[string]ColorFunc

	// nextPrefixStyleIndex is used to get the next style from the `prefixStyles` list for a newly discovered prefix.
	nextPrefixStyleIndex int
}

func NewPrefixStyle() *PrefixStyle {
	return &PrefixStyle{
		cache: make(map[string]ColorFunc),
	}
}

func (prefixStyle *PrefixStyle) ColorFunc(prefix string) ColorFunc {
	if colorFunc, ok := prefixStyle.cache[prefix]; ok {
		return colorFunc
	}

	if prefixStyle.nextPrefixStyleIndex >= len(prefixStyles) {
		prefixStyle.nextPrefixStyleIndex = 0
	}

	colorFunc := prefixStyles[prefixStyle.nextPrefixStyleIndex].ColorFunc()

	prefixStyle.cache[prefix] = colorFunc
	prefixStyle.nextPrefixStyleIndex++

	return colorFunc
}
