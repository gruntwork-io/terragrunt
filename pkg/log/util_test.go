package log_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestRemoveAllASCISeq(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no_ansi",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "single_color_code",
			input:    "\033[31mhello\033[0m",
			expected: "hello",
		},
		{
			name:     "bold",
			input:    "\033[1mbold text\033[0m",
			expected: "bold text",
		},
		{
			name:     "multiple_sequences",
			input:    "\033[31mred\033[0m and \033[32mgreen\033[0m",
			expected: "red and green",
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "embedded_mid_string",
			input:    "before\033[33mmiddle\033[0mafter",
			expected: "beforemiddleafter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, log.RemoveAllASCISeq(tc.input))
		})
	}
}

func TestResetASCISeq(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no_ansi_unchanged",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "with_ansi_appends_reset",
			input:    "\033[31mhello",
			expected: "\033[31mhello\033[0m",
		},
		{
			name:     "empty_unchanged",
			input:    "",
			expected: "",
		},
		{
			name:     "already_has_reset",
			input:    "\033[31mhello\033[0m",
			expected: "\033[31mhello\033[0m\033[0m",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, log.ResetASCISeq(tc.input))
		})
	}
}

func FuzzRemoveAllASCISeq(f *testing.F) {
	f.Add("")
	f.Add("hello")
	f.Add("\033[31mred\033[0m")
	f.Add("\033[1;32mbold green\033[0m")
	f.Add("\033[") // bare/incomplete sequence

	f.Fuzz(func(t *testing.T, input string) {
		// Must never panic
		result := log.RemoveAllASCISeq(input)
		// If input had no ANSI start sequence, output equals input
		if !strings.Contains(input, "\033[") {
			assert.Equal(t, input, result)
		}
	})
}

func FuzzResetASCISeq(f *testing.F) {
	f.Add("")
	f.Add("hello")
	f.Add("\033[31mred")
	f.Add("\033[1;32mbold green\033[0m")

	f.Fuzz(func(t *testing.T, input string) {
		result := log.ResetASCISeq(input)
		// If input contained ANSI escape, output must end with reset sequence
		if strings.Contains(input, "\033[") {
			assert.True(t, strings.HasSuffix(result, "\033[0m"), "output with ANSI sequences should end with reset")
		}
	})
}
