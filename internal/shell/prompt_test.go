package shell_test

import (
	"io"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPromptUserForInputSequential pins that a run prompting more than once
// reads every answer. The venv buffers stdin once, so the read-ahead from the
// first prompt stays available to the second.
func TestPromptUserForInputSequential(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithStdin(strings.NewReader("first\nsecond\n"))

	first, err := shell.PromptUserForInput(t.Context(), l, v, "one: ", false)
	require.NoError(t, err)
	assert.Equal(t, "first", first)

	second, err := shell.PromptUserForInput(t.Context(), l, v, "two: ", false)
	require.NoError(t, err)
	assert.Equal(t, "second", second)
}

// TestPromptUserForInputUnterminatedFinalLine pins that an answer ending at EOF
// without a trailing newline is still read, so `printf yes | terragrunt ...`
// answers a prompt.
func TestPromptUserForInputUnterminatedFinalLine(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithStdin(strings.NewReader("first\nsecond"))

	first, err := shell.PromptUserForInput(t.Context(), l, v, "one: ", false)
	require.NoError(t, err)
	assert.Equal(t, "first", first)

	second, err := shell.PromptUserForInput(t.Context(), l, v, "two: ", false)
	require.NoError(t, err)
	assert.Equal(t, "second", second)

	_, err = shell.PromptUserForInput(t.Context(), l, v, "three: ", false)
	require.ErrorIs(t, err, io.EOF)
}

// TestPromptUserForInputEmptyStdin pins that stdin closed without any answer is
// an error rather than an empty answer, which yes/no prompts would read as "no".
func TestPromptUserForInputEmptyStdin(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithStdin(strings.NewReader(""))

	_, err := shell.PromptUserForInput(t.Context(), l, v, "one: ", false)
	require.ErrorIs(t, err, io.EOF)
}

func TestPromptUserForYesNo(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "y", input: "y\n", expected: true},
		{name: "yes", input: "yes\n", expected: true},
		{name: "uppercase yes", input: "YES\n", expected: true},
		{name: "n", input: "n\n", expected: false},
		{name: "anything else", input: "maybe\n", expected: false},
		{name: "yes without trailing newline", input: "yes", expected: true},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			v := venvtest.New().WithStdin(strings.NewReader(tt.input))

			got, err := shell.PromptUserForYesNo(t.Context(), l, v, "proceed?", false)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestPromptUserForYesNoNonInteractive pins that stdin is never consulted when
// the non-interactive flag is set: the answer is "yes" despite the "no" input.
func TestPromptUserForYesNoNonInteractive(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithStdin(strings.NewReader("no\n"))

	got, err := shell.PromptUserForYesNo(t.Context(), l, v, "proceed?", true)
	require.NoError(t, err)
	assert.True(t, got)
}

func TestPromptUserForInputPanicsOnUnsetReader(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	v.Stdin = nil

	assert.PanicsWithError(t, venv.ErrVenvStdinUnset.Error(), func() {
		_, _ = shell.PromptUserForInput(t.Context(), logger.CreateLogger(), v, "one: ", false)
	})
}
