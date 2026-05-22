package util

import (
	"strings"
	"unicode/utf8"
)

// WrapWords greedily wraps s at space boundaries so no output line exceeds
// width runes. Words longer than width go on their own line rather than being
// split mid-word. Existing newlines in s are preserved as hard breaks. A width
// of zero or less returns s unchanged.
func WrapWords(s string, width int) string {
	if width <= 0 {
		return s
	}

	var (
		out     strings.Builder
		lineLen int
		first   = true
	)

	for paragraph := range strings.SplitSeq(s, "\n") {
		if !first {
			out.WriteByte('\n')

			lineLen = 0
		}

		first = false

		for word := range strings.FieldsSeq(paragraph) {
			wordLen := utf8.RuneCountInString(word)

			if lineLen == 0 {
				out.WriteString(word)

				lineLen = wordLen

				continue
			}

			if lineLen+1+wordLen > width {
				out.WriteByte('\n')
				out.WriteString(word)

				lineLen = wordLen

				continue
			}

			out.WriteByte(' ')
			out.WriteString(word)

			lineLen += 1 + wordLen
		}
	}

	return out.String()
}
