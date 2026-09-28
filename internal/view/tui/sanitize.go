package tui

import (
	"strings"
	"unicode"
)

// replacementRune stands in for input a terminal would act on rather than draw.
const replacementRune = '�'

// SanitizeText makes untrusted multi-line text safe to render. Terminals act on
// control characters, and neither the style nor the truncation paths strip
// them, so hostile input could otherwise inject escape sequences that move the
// cursor, set the title, or write the clipboard. Invalid UTF-8 is coerced too,
// since the Markdown and syntax renderers panic on it. Newlines and tabs keep
// their formatting role, and carriage returns are dropped so CRLF text renders
// cleanly.
func SanitizeText(source string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == '\r':
			return -1
		case unicode.IsControl(r):
			return replacementRune
		}

		return r
	}, strings.ToValidUTF8(source, string(replacementRune)))
}

// SanitizeLabel makes an untrusted single-line label safe to render: a file
// name, a path, or a warning message. It is [SanitizeText] with no exemptions,
// because a newline or tab carried by a name would break the row or box it is
// drawn into as surely as an escape sequence would.
func SanitizeLabel(label string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return replacementRune
		}

		return r
	}, strings.ToValidUTF8(label, string(replacementRune)))
}
