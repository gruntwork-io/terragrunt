package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWidthOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		expected string
		width    int
	}{
		{
			name:     "zero_width_is_passthrough",
			width:    0,
			value:    "\033[31mhello\033[0m",
			expected: "\033[31mhello\033[0m",
		},
		{
			name:     "pads_shorter_value",
			width:    5,
			value:    "abc",
			expected: "abc  ",
		},
		{
			name:     "pads_by_visible_length_ignoring_ansi",
			width:    7,
			value:    "\033[31mabc\033[0m",
			expected: "\033[31mabc\033[0m    ",
		},
		{
			name:     "truncates_longer_value",
			width:    3,
			value:    "abcdef",
			expected: "abc",
		},
		{
			name:     "truncates_preserving_ansi_sequences",
			width:    3,
			value:    "\033[31mabcdef\033[0m",
			expected: "\033[31mabc",
		},
		{
			name:     "truncates_without_splitting_multibyte_runes",
			width:    2,
			value:    "héllo",
			expected: "hé",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := options.Width(tc.width).Format(nil, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}
