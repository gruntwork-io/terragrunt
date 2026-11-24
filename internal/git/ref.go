package git

import "strings"

// SanitizeRef returns ref with every character other than an ASCII letter,
// digit, '-', or '_' replaced by '_', so it is safe as one element of a path
// on every platform.
func SanitizeRef(ref string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-',
			r == '_':
			return r
		}

		return '_'
	}, ref)
}
