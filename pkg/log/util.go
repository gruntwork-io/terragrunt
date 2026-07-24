package log

import (
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	CurDir              = "."
	CurDirWithSeparator = CurDir + string(os.PathSeparator)

	// startASNISeq is the ANSI start escape sequence
	startASNISeq = "\033["
	// resetANSISeq is the ANSI reset escape sequence
	resetANSISeq = "\033[0m"

	ansiSeq = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
)

var (
	// regexp matches ansi characters getting from a shell output, used for colors etc.
	ansiReg = regexp.MustCompile(ansiSeq)
)

// RemoveAllASCISeq returns a string with all ASCII color characters removed.
func RemoveAllASCISeq(str string) string {
	if strings.Contains(str, startASNISeq) {
		str = ansiReg.ReplaceAllString(str, "")
	}

	return str
}

// ResetASCISeq returns a string with the ASCI color reset to the default one.
func ResetASCISeq(str string) string {
	if strings.Contains(str, startASNISeq) {
		str += resetANSISeq
	}

	return str
}

// VisibleLength returns the number of visible characters in str, counting runes
// and ignoring ANSI escape sequences.
func VisibleLength(str string) int {
	if !hasANSI(str) {
		return utf8.RuneCountInString(str)
	}

	return utf8.RuneCountInString(ansiReg.ReplaceAllString(str, ""))
}

// TruncateVisible returns the prefix of str that holds the first `width` visible
// characters. ANSI escape sequences are copied verbatim and do not count toward
// the width, and runes are never split mid-byte.
func TruncateVisible(str string, width int) string {
	if width <= 0 {
		return ""
	}

	var (
		buf     strings.Builder
		visible int
		pos     int
	)

	appendVisible := func(s string) bool {
		for _, r := range s {
			if visible == width {
				return true
			}

			buf.WriteRune(r)

			visible++
		}

		return false
	}

	// Share VisibleLength's escape-sequence model so both agree on what counts as
	// visible, regardless of where a sequence sits in the string.
	var seqs [][]int
	if hasANSI(str) {
		seqs = ansiReg.FindAllStringIndex(str, -1)
	}

	for _, seq := range seqs {
		if appendVisible(str[pos:seq[0]]) {
			return buf.String()
		}

		buf.WriteString(str[seq[0]:seq[1]])
		pos = seq[1]
	}

	appendVisible(str[pos:])

	return buf.String()
}

// hasANSI reports whether str may contain an ANSI escape sequence, i.e. an ESC
// (U+001B) or CSI (U+009B) introducer that [ansiReg] could match.
func hasANSI(str string) bool {
	return strings.ContainsRune(str, '\u001b') || strings.ContainsRune(str, '\u009b')
}
