package options_test

import (
	"bytes"
	"errors"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractErrorMessage_ExcludesCommandFlags(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	stderr.WriteString("flag provided but not defined: -abc")

	err := util.ProcessExecutionError{
		Err:        errors.New("exit status 1"),
		Command:    "tofu",
		Args:       []string{"plan", "-lock-timeout=120m", "-input=false"},
		WorkingDir: "/some/path",
		Output:     util.CmdOutput{Stderr: stderr},
	}

	msg := options.ExportExtractErrorMessage(err)

	// The extracted message should only contain stderr and the underlying error,
	// not the command string with flags.
	assert.NotContains(t, msg, "-lock-timeout")
	assert.NotContains(t, msg, "tofu plan")
	// Should contain the actual error text from stderr and the exit error
	assert.Contains(t, msg, "flag provided but not defined")
	assert.Contains(t, msg, "exit status 1")
}

func TestExtractErrorMessage_DoesNotFalselyMatchTimeout(t *testing.T) {
	t.Parallel()

	// Simulate the exact scenario from issue #5088:
	// Command has -lock-timeout=120m flag, but the actual error is unrelated to timeout.
	var stderr bytes.Buffer
	stderr.WriteString("flag provided but not defined: -abc")

	err := util.ProcessExecutionError{
		Err:        errors.New("exit status 1"),
		Command:    "tofu",
		Args:       []string{"plan", "-lock-timeout=120m", "-input=false", "-fes"},
		WorkingDir: "/some/path",
		Output:     util.CmdOutput{Stderr: stderr},
	}

	timeoutPattern := regexp.MustCompile(`(?s).*timeout.*`)
	patterns := []*options.ErrorsPattern{
		{Pattern: timeoutPattern},
	}

	msg := options.ExportExtractErrorMessage(err)

	// The timeout pattern should NOT match because the extracted message only
	// contains stderr and exit error, not the command flags.
	matched := options.ExportMatchesAnyRegexpPattern(msg, patterns)
	assert.False(t, matched, "timeout pattern should NOT match when 'timeout' only appears in command flags; cleaned message: %s", msg)
}

func TestExtractErrorMessage_StillMatchesRealTimeout(t *testing.T) {
	t.Parallel()

	// When stderr actually contains "timeout", the pattern should match.
	var stderr bytes.Buffer
	stderr.WriteString("Error: timeout waiting for resource to become available")

	err := util.ProcessExecutionError{
		Err:        errors.New("exit status 1"),
		Command:    "tofu",
		Args:       []string{"apply", "-auto-approve"},
		WorkingDir: "/some/path",
		Output:     util.CmdOutput{Stderr: stderr},
	}

	timeoutPattern := regexp.MustCompile(`(?s).*timeout.*`)
	patterns := []*options.ErrorsPattern{
		{Pattern: timeoutPattern},
	}

	msg := options.ExportExtractErrorMessage(err)
	matched := options.ExportMatchesAnyRegexpPattern(msg, patterns)
	assert.True(t, matched, "timeout pattern should match when stderr actually contains 'timeout'; cleaned message: %s", msg)
}

func TestExtractErrorMessage_StillMatchesTimeoutInStderrWithFlags(t *testing.T) {
	t.Parallel()

	// Even when the command has -lock-timeout flags, if stderr also contains "timeout",
	// the pattern should match.
	var stderr bytes.Buffer
	stderr.WriteString("Error: timeout waiting for state lock")

	err := util.ProcessExecutionError{
		Err:        errors.New("exit status 1"),
		Command:    "tofu",
		Args:       []string{"plan", "-lock-timeout=120m", "-input=false"},
		WorkingDir: "/some/path",
		Output:     util.CmdOutput{Stderr: stderr},
	}

	timeoutPattern := regexp.MustCompile(`(?s).*timeout.*`)
	patterns := []*options.ErrorsPattern{
		{Pattern: timeoutPattern},
	}

	msg := options.ExportExtractErrorMessage(err)
	matched := options.ExportMatchesAnyRegexpPattern(msg, patterns)
	assert.True(t, matched, "timeout pattern should match when stderr actually contains 'timeout'; cleaned message: %s", msg)
}

func TestExtractErrorMessage_NonProcessError(t *testing.T) {
	t.Parallel()

	// For non-ProcessExecutionError errors, the full error string is used.
	err := errors.New("some generic error with timeout in it")

	msg := options.ExportExtractErrorMessage(err)
	assert.Contains(t, msg, "timeout")

	timeoutPattern := regexp.MustCompile(`(?s).*timeout.*`)
	patterns := []*options.ErrorsPattern{
		{Pattern: timeoutPattern},
	}

	matched := options.ExportMatchesAnyRegexpPattern(msg, patterns)
	assert.True(t, matched, "timeout pattern should match for non-ProcessExecutionError; cleaned message: %s", msg)
}

func TestErrorCleanPattern_PreservesCharacters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "preserves hyphens in flags",
			input:    "-lock-timeout=120m",
			expected: "-lock-timeout=120m",
		},
		{
			name:     "preserves equals signs",
			input:    "-input=false",
			expected: "-input=false",
		},
		{
			name:     "strips control chars",
			input:    "error\x00here",
			expected: "error here",
		},
		{
			name:     "preserves alphanumeric and standard punctuation",
			input:    `Failed to execute "tofu plan" in /some/path`,
			expected: `Failed to execute "tofu plan" in /some/path`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := options.ExportErrorCleanPattern.ReplaceAllString(tt.input, " ")
			require.Equal(t, tt.expected, result)
		})
	}
}
