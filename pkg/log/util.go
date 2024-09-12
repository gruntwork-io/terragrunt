package log

import (
	"os"
	"regexp"
	"strings"
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
