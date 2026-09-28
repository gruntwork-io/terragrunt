package log

import (
	"os"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
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

// VisibleLength returns the number of terminal columns str fills, ignoring ANSI
// escape sequences. A character that fills two columns, such as one in a CJK
// script, counts as two, and each byte of invalid UTF-8 counts as one.
func VisibleLength(str string) int {
	if hasANSI(str) {
		str = ansiReg.ReplaceAllString(str, "")
	}

	return runewidth.StringWidth(strings.ToValidUTF8(str, invalidByteReplacement))
}

// TruncateVisible returns the prefix of str that fills at most `width` terminal
// columns. ANSI escape sequences are copied verbatim and do not count toward the
// width. A character is never split, so a two-column character that would cross
// the limit is left out. Each byte of invalid UTF-8 comes back as U+FFFD.
func TruncateVisible(str string, width int) string {
	if width <= 0 {
		return ""
	}

	str = strings.ToValidUTF8(str, invalidByteReplacement)

	var (
		buf       strings.Builder
		remaining = width
		pos       int
	)

	appendVisible := func(s string) bool {
		kept := runewidth.Truncate(s, remaining, "")

		buf.WriteString(kept)

		remaining -= runewidth.StringWidth(kept)

		return len(kept) < len(s)
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

// invalidByteReplacement stands in for each invalid UTF-8 byte before measuring,
// because [runewidth.StringWidth] gives such a byte different widths depending on
// the length of the string around it.
const invalidByteReplacement = "\uFFFD"

// hasANSI reports whether str may contain an ANSI escape sequence, i.e. an ESC
// (U+001B) or CSI (U+009B) introducer that [ansiReg] could match.
func hasANSI(str string) bool {
	return strings.ContainsRune(str, '\u001b') || strings.ContainsRune(str, '\u009b')
}
