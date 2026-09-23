package mcp_test

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fuzzAllowCmdMaxWords caps the words on each side of a fuzzed match, since
// the reference matcher backtracks and slows exponentially with more ** words.
const fuzzAllowCmdMaxWords = 8

// FuzzAllowCmdMatches pins that [tgmcp.AllowCmd.Matches] agrees with a
// backtracking matcher that follows the pattern grammar word by word.
func FuzzAllowCmdMatches(f *testing.F) {
	for _, seed := range []struct {
		pattern string
		args    string
	}{
		{pattern: "", args: ""},
		{pattern: "**", args: "a b c"},
		{pattern: "** --profile=*", args: "sts get-caller-identity --profile=dev"},
		{pattern: "a ** a", args: "a"},
		{pattern: "** a ** b **", args: "b a"},
		{pattern: "** a ** b **", args: "a x b"},
		{pattern: "** ** x", args: "x"},
		{pattern: "a* ** *b", args: "ab"},
		{pattern: "[ab] ** {c,d}", args: "a c d"},
	} {
		f.Add(seed.pattern, seed.args)
	}

	f.Fuzz(func(t *testing.T, patternArgs, argLine string) {
		words := strings.Fields(patternArgs)
		args := strings.Fields(argLine)

		if len(words) > fuzzAllowCmdMaxWords || len(args) > fuzzAllowCmdMaxWords {
			t.Skip("the reference matcher backtracks, so large inputs are too slow")
		}

		if !utf8.ValidString(patternArgs) ||
			(runtime.GOOS == "windows" && strings.ContainsRune(patternArgs, '\\')) {
			t.Skip("split.Command does not read such a word back unchanged")
		}

		pattern := &strings.Builder{}
		pattern.WriteString("prog")

		for _, word := range words {
			pattern.WriteString(" " + split.Quote(word))
		}

		cmd, err := tgmcp.ParseAllowCmd(pattern.String())
		if err != nil {
			t.Skip("the pattern has an argument word that is not a valid glob")
		}

		assert.Equal(t, referenceAllowCmdMatch(t, words, args), cmd.Matches("prog", args))
	})
}

// referenceAllowCmdMatch matches args against pattern words by trying every
// number of arguments for each ** word.
func referenceAllowCmdMatch(t *testing.T, words, args []string) bool {
	t.Helper()

	if len(words) == 0 {
		return len(args) == 0
	}

	if words[0] == "**" {
		for i := range len(args) + 1 {
			if referenceAllowCmdMatch(t, words[1:], args[i:]) {
				return true
			}
		}

		return false
	}

	if len(args) == 0 {
		return false
	}

	m, err := glob.Compile(words[0], glob.WithoutSeparator())
	require.NoError(t, err)

	return m.Match(args[0]) && referenceAllowCmdMatch(t, words[1:], args[1:])
}
