package log_test

import (
	"strings"
	"testing"
	"unicode/utf8"

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

func TestVisibleLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "plain_ascii",
			input:    "hello",
			expected: 5,
		},
		{
			name:     "with_ansi",
			input:    "\033[31mhello\033[0m",
			expected: 5,
		},
		{
			name:     "multibyte_runes",
			input:    "héllo",
			expected: 5,
		},
		{
			name:     "empty",
			input:    "",
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, log.VisibleLength(tc.input))
		})
	}
}

func TestTruncateVisible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
		width    int
	}{
		{
			name:     "plain_ascii",
			input:    "hello world",
			width:    5,
			expected: "hello",
		},
		{
			name:     "no_truncation_when_shorter",
			input:    "hi",
			width:    5,
			expected: "hi",
		},
		{
			name:     "keeps_ansi_sequences",
			input:    "\033[31mhello\033[0m world",
			width:    3,
			expected: "\033[31mhel",
		},
		{
			name:     "counts_only_visible_runes",
			input:    "\033[31mhello\033[0m",
			width:    5,
			expected: "\033[31mhello\033[0m",
		},
		{
			name:     "does_not_split_multibyte_runes",
			input:    "héllo",
			width:    2,
			expected: "hé",
		},
		{
			name:     "zero_width",
			input:    "hello",
			width:    0,
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, log.TruncateVisible(tc.input, tc.width))
		})
	}
}

func FuzzTruncateVisible(f *testing.F) {
	f.Add("hello world", 5)
	f.Add("\033[31mred\033[0m", 2)
	f.Add("héllo", 3)
	f.Add("", 4)

	f.Fuzz(func(t *testing.T, input string, width int) {
		result := log.TruncateVisible(input, width)

		// The result must never contain more visible characters than requested.
		if width > 0 {
			assert.LessOrEqual(t, log.VisibleLength(result), width)
		}

		// The result must always be valid UTF-8, never a split rune.
		assert.True(t, utf8.ValidString(result), "result must be valid UTF-8")
	})
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
			assert.True(
				t,
				strings.HasSuffix(result, "\033[0m"),
				"output with ANSI sequences should end with reset",
			)
		}
	})
}
