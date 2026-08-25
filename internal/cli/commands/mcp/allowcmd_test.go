package mcp_test

import (
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllowCmdMatchesWordByWord pins how a pattern matches an invocation: the
// program exactly, each argument word against one pattern word, and ** against
// any number of words.
func TestAllowCmdMatchesWordByWord(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		pattern string
		name    string
		program string
		args    []string
		want    bool
	}{
		{
			name:    "an exact invocation",
			pattern: "git rev-parse --show-toplevel",
			program: "git",
			args:    []string{"rev-parse", "--show-toplevel"},
			want:    true,
		},
		{
			name:    "an exact pattern refuses an extra argument",
			pattern: "git rev-parse --show-toplevel",
			program: "git",
			args:    []string{"rev-parse", "--show-toplevel", "--quiet"},
		},
		{
			name:    "an exact pattern refuses a missing argument",
			pattern: "git rev-parse --show-toplevel",
			program: "git",
			args:    []string{"rev-parse"},
		},
		{
			name:    "another program",
			pattern: "jq **",
			program: "yq",
			args:    []string{"."},
		},
		{
			name:    "* matches one word",
			pattern: "jq *",
			program: "jq",
			args:    []string{"."},
			want:    true,
		},
		{
			name:    "* does not match two words",
			pattern: "jq *",
			program: "jq",
			args:    []string{"-r", ".version"},
		},
		{
			name:    "* does not match no word",
			pattern: "jq *",
			program: "jq",
		},
		{
			name:    "** matches no word",
			pattern: "jq **",
			program: "jq",
			want:    true,
		},
		{
			name:    "** matches many words",
			pattern: "jq **",
			program: "jq",
			args:    []string{"-r", ".version", "version.json"},
			want:    true,
		},
		{
			name:    "** before a fixed word",
			pattern: "aws ** --profile=*",
			program: "aws",
			args:    []string{"sts", "get-caller-identity", "--profile=dev"},
			want:    true,
		},
		{
			name:    "** before a fixed word that is missing",
			pattern: "aws ** --profile=*",
			program: "aws",
			args:    []string{"sts", "get-caller-identity"},
		},
		{
			name:    "** between fixed words",
			pattern: "aws sts ** --profile=*",
			program: "aws",
			args:    []string{"sts", "get-caller-identity", "--output", "json", "--profile=dev"},
			want:    true,
		},
		{
			name:    "fixed words between two ** words in order",
			pattern: "tofu ** -var=* ** -out=*",
			program: "tofu",
			args:    []string{"plan", "-var=region=us-east-1", "-lock=false", "-out=tfplan"},
			want:    true,
		},
		{
			name:    "fixed words between ** words out of order",
			pattern: "tofu ** -lock=false ** -input=false **",
			program: "tofu",
			args:    []string{"plan", "-input=false", "-lock=false"},
		},
		{
			name:    "one argument cannot match both the first and last words",
			pattern: "echo hi ** hi",
			program: "echo",
			args:    []string{"hi"},
		},
		{
			name:    "consecutive ** words",
			pattern: "jq ** ** .",
			program: "jq",
			args:    []string{"."},
			want:    true,
		},
		{
			name:    "* within a word",
			pattern: "aws sts get-caller-identity --profile=*",
			program: "aws",
			args:    []string{"sts", "get-caller-identity", "--profile=dev"},
			want:    true,
		},
		{
			name:    "* spans slashes in a path argument",
			pattern: "jq *",
			program: "jq",
			args:    []string{"./configs/prod/values.json"},
			want:    true,
		},
		{
			name:    "* spans slashes within a word",
			pattern: "tofu -chdir=*",
			program: "tofu",
			args:    []string{"-chdir=envs/prod"},
			want:    true,
		},
		{
			name:    "a quoted word is one argument",
			pattern: `sh -c "echo hello"`,
			program: "sh",
			args:    []string{"-c", "echo hello"},
			want:    true,
		},
		{
			name:    "an argument with spaces is not several words",
			pattern: "jq -r .version",
			program: "jq",
			args:    []string{"-r .version"},
		},
		{
			name:    "an .exe suffix on the program word",
			pattern: "jq.exe **",
			program: "jq",
			args:    []string{"."},
			want:    true,
		},
		{
			name:    "a relative path",
			pattern: "./scripts/version.sh --short",
			program: "./scripts/version.sh",
			args:    []string{"--short"},
			want:    true,
		},
		{
			name:    "a relative path matches once cleaned",
			pattern: "./scripts/version.sh **",
			program: "scripts/../scripts/version.sh",
			want:    true,
		},
		{
			name:    "an absolute path",
			pattern: "/abs/path/jq **",
			program: "/abs/path/jq",
			args:    []string{"."},
			want:    true,
		},
		{
			name:    "an absolute path does not match a relative one",
			pattern: "/abs/path/jq **",
			program: "abs/path/jq",
		},
		{
			name:    "a bare name does not match a path",
			pattern: "git **",
			program: "./git",
		},
		{
			name:    "a path does not match a bare name",
			pattern: "./git **",
			program: "git",
		},
		{
			name:    "an .exe suffix on a path",
			pattern: "./tools/jq.exe **",
			program: "./tools/jq",
			want:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd, err := tgmcp.ParseAllowCmd(tc.pattern)
			require.NoError(t, err)

			assert.Equal(t, tc.want, cmd.Matches(tc.program, tc.args))
		})
	}
}

// TestParseAllowCmdRefusesAPatternItCannotHonor pins that a pattern the matcher
// cannot honor as written fails to parse.
func TestParseAllowCmdRefusesAPatternItCannotHonor(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{
		"",
		"   ",
		`""`,
		"j*q **",
		"./scripts/*.sh **",
		"jq a{",
		"** --version",
		"jq ** ; sh",
		"jq | sh",
		`jq "unterminated`,
	} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			_, err := tgmcp.ParseAllowCmd(pattern)
			require.ErrorIs(t, err, tgmcp.ErrInvalidAllowCmd)
		})
	}
}
