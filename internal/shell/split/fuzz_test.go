package split_test

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzCommand pins that no command line, however malformed, panics the
// splitter. Command lines come from configuration and flags a user or an agent
// wrote.
func FuzzCommand(f *testing.F) {
	for _, seed := range []string{
		"",
		"auth --profile dev",
		`auth --profile "with space"`,
		`echo 'it'\''s'`,
		"jq . | sh",
		"echo $(id) `id`",
		`jq "unterminated`,
		`C:\tools\jq.exe .`,
		"echo (",
		"2>&1",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, line string) {
		words, err := split.Command(line)
		if err != nil {
			require.ErrorIs(t, err, split.ErrInvalidCommand)
			assert.Nil(t, words)
		}
	})
}

// FuzzQuote pins that a quoted word splits back into exactly that word, so an
// agent-written value shown in a command line cannot end its word early or
// start another.
func FuzzQuote(f *testing.F) {
	for _, seed := range []string{
		"",
		"plain",
		"with space",
		"it's",
		"'",
		"!./b",
		"a | type=unit",
		"$(id)",
		"`id`",
		"#note",
		"tab\there",
		"new\nline",
		`back\slash`,
		"ünïcödé",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, word string) {
		if !utf8.ValidString(word) {
			t.Skip("Command reads invalid UTF-8 as U+FFFD")
		}

		if runtime.GOOS == "windows" && strings.ContainsRune(word, '\\') {
			t.Skip("Command reads a backslash as a forward slash on Windows")
		}

		words, err := split.Command(split.Quote(word))
		require.NoError(t, err)

		assert.Equal(t, []string{word}, words)
	})
}
