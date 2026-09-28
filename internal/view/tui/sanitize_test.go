package tui_test

import (
	"testing"
	"unicode"
	"unicode/utf8"

	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeText(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain text is untouched", in: "inputs = {}", want: "inputs = {}"},
		{name: "newlines and tabs keep their formatting role", in: "a\n\tb", want: "a\n\tb"},
		{name: "carriage returns are dropped", in: "a\r\nb", want: "a\nb"},
		{name: "an OSC title write is defanged", in: "before\x1b]0;pwned\aafter", want: "before�]0;pwned�after"},
		{name: "a C1 control is defanged", in: "a\u009bb", want: "a�b"},
		{name: "invalid UTF-8 is coerced", in: "a\xffb", want: "a�b"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, viewtui.SanitizeText(tt.in))
		})
	}
}

func TestSanitizeLabel(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain names are untouched", in: "main.tf", want: "main.tf"},
		{name: "a clipboard write is defanged", in: "a\x1b]52;c;cHduZWQ=\ab", want: "a�]52;c;cHduZWQ=�b"},
		{name: "a screen clear is defanged", in: "a\x1b[2Jb", want: "a�[2Jb"},
		{name: "a newline cannot break the row", in: "a\nb", want: "a�b"},
		{name: "a tab cannot break the row", in: "a\tb", want: "a�b"},
		{name: "a carriage return cannot rewind the row", in: "a\rb", want: "a�b"},
		{name: "invalid UTF-8 is coerced", in: "a\xff\xfeb", want: "a�b"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, viewtui.SanitizeLabel(tt.in))
		})
	}
}

// FuzzSanitizeLabel asserts the invariant the render paths rely on: whatever a
// filename or log message carries, the result is valid UTF-8 holding nothing a
// terminal would act on, and sanitizing it again changes nothing.
func FuzzSanitizeLabel(f *testing.F) {
	f.Add("main.tf")
	f.Add("a\x1b]52;c;cHduZWQ=\ab")
	f.Add("a\r\nb\tc")
	f.Add("\xff\xfe\x00")

	f.Fuzz(func(t *testing.T, in string) {
		out := viewtui.SanitizeLabel(in)

		assert.True(t, utf8.ValidString(out), "sanitized output must be valid UTF-8")

		for _, r := range out {
			assert.Falsef(t, unicode.IsControl(r), "control rune %U survived sanitizing", r)
		}

		assert.Equal(t, out, viewtui.SanitizeLabel(out), "sanitizing is idempotent")
	})
}

// FuzzSanitizeText asserts the same invariant for multi-line text, less the
// newlines and tabs the preview pane needs to lay a file out.
func FuzzSanitizeText(f *testing.F) {
	f.Add("inputs = {}\n")
	f.Add("# Title\n\n```\nunclosed fence")
	f.Add("before\x1b]0;pwned\aafter\r\nnext")
	f.Add("\x00\x01\xff\xfe")

	f.Fuzz(func(t *testing.T, in string) {
		out := viewtui.SanitizeText(in)

		assert.True(t, utf8.ValidString(out), "sanitized output must be valid UTF-8")

		for _, r := range out {
			if !unicode.IsControl(r) {
				continue
			}

			assert.Containsf(t, "\n\t", string(r), "control rune %U survived sanitizing", r)
		}

		assert.Equal(t, out, viewtui.SanitizeText(out), "sanitizing is idempotent")
	})
}
