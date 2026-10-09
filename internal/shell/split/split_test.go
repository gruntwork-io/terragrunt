package split_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandSplitsWithShellQuoting pins the grammar callers rely on: quoting
// groups words, and nothing is expanded or substituted.
func TestCommandSplitsWithShellQuoting(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		line      string
		wantWords []string
	}{
		{
			name:      "plain words",
			line:      "auth --region us-east-1",
			wantWords: []string{"auth", "--region", "us-east-1"},
		},
		{
			name:      "a quoted word keeps its spaces",
			line:      `auth --profile "with space"`,
			wantWords: []string{"auth", "--profile", "with space"},
		},
		{
			name:      "a quoted empty word",
			line:      `""`,
			wantWords: []string{""},
		},
		{
			name:      "a single quote inside single quotes",
			line:      `echo 'it'"'"'s'`,
			wantWords: []string{"echo", "it's"},
		},
		{
			name:      "variables are not expanded",
			line:      "echo $HOME",
			wantWords: []string{"echo", "$HOME"},
		},
		{
			name:      "substitutions are not run",
			line:      "echo $(id) `id`",
			wantWords: []string{"echo", "$(id)", "`id`"},
		},
		{
			name:      "a quoted operator is part of a word",
			line:      "tg --filter='a | type=unit'",
			wantWords: []string{"tg", "--filter=a | type=unit"},
		},
		{
			name:      "a path keeps its separators",
			line:      filepath.Join("tools", "jq") + " .",
			wantWords: []string{"tools/jq", "."},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			words, err := split.Command(tc.line)
			require.NoError(t, err)

			assert.Equal(t, tc.wantWords, words)
		})
	}
}

// TestCommandReadsABackslashAsAnEscape pins that, outside Windows, a backslash
// escapes the character after it, and a trailing one has nothing to escape.
func TestCommandReadsABackslashAsAnEscape(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: Command reads a backslash as a forward slash")
	}

	words, err := split.Command(`echo 'it'\''s'`)
	require.NoError(t, err)
	assert.Equal(t, []string{"echo", "it's"}, words)

	_, err = split.Command(`jq \`)
	require.ErrorIs(t, err, split.ErrInvalidCommand)
}

// TestCommandRefusesAShellOperator pins that an unquoted operator fails with
// [split.ErrShellOperator] wherever it appears in the line, and that the
// failure still matches [split.ErrInvalidCommand].
func TestCommandRefusesAShellOperator(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"get-creds | jq .creds",
		"plan; apply",
		"plan && apply",
		"plan > out",
		"2>&1",
		";",
		"|",
	} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()

			words, err := split.Command(line)
			require.ErrorIs(t, err, split.ErrShellOperator)
			require.ErrorIs(t, err, split.ErrInvalidCommand)
			assert.Nil(t, words)
		})
	}
}

// TestCommandRefusesAMalformedLine pins that a line the grammar cannot read
// fails with [split.ErrInvalidCommand] rather than splitting into a guess.
func TestCommandRefusesAMalformedLine(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		`jq "unterminated`,
		`jq 'unterminated`,
		"echo (",
		"echo )",
	} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()

			_, err := split.Command(line)
			require.ErrorIs(t, err, split.ErrInvalidCommand)
		})
	}
}

// TestQuoteRendersOneWord pins that a word is left bare only when no shell
// would read it differently.
func TestQuoteRendersOneWord(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		word string
		want string
	}{
		{word: "./a", want: "./a"},
		{word: "--profile=dev", want: "--profile=dev"},
		{word: "user@host:1.2,3+4%_", want: "user@host:1.2,3+4%_"},
		{word: "", want: "''"},
		{word: "!./b", want: "'!./b'"},
		{word: "a | type=unit", want: "'a | type=unit'"},
		{word: "~/x", want: "'~/x'"},
		{word: "$HOME", want: "'$HOME'"},
		{word: "it's", want: `'it'"'"'s'`},
	} {
		t.Run(tc.word, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, split.Quote(tc.word))
		})
	}
}
